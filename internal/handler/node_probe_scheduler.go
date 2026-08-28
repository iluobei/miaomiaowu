package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/speedtest"
	"miaomiaowu/internal/storage"
)

// 外部节点连通性探测:周期性用 mihomo 真连一次每个「已勾选探测」的外部节点,
// 记录延迟与成败。
//
// 与 TCPing 的区别:那个只拨 server:port,只能看出端口通不通,握手失败、证书不对、
// 密码错、被 QoS 限速一概看不出来。这里是**完整握手 + 真实请求**,拨的是节点本身
// 能不能用。代价是每个节点要起一个 mihomo 进程,所以只跑用户勾选的那些,并且限流。

const (
	// nodeProbeInterval 探测周期。
	nodeProbeInterval = 5 * time.Minute
	// nodeProbeConcurrency 只能是 1。
	//
	// speedtest.RunNodeTest 用固定的 mixedPort(17900)起 mihomo,因此内部有一把全局
	// 串行锁 —— 真实并发能力就是 1。把这里开成 3 不但没用,还会**制造超时**:
	// 三个协程同时进 probeOne 各自开始计 20 秒,然后在锁上排队;头一个若是拨不通的
	// 节点要占满 20 秒,后两个还没轮到就已经超时,被记成"不通"。
	// 真机上就是这么被坑的:两个节点(一好一坏)一起探测 → 通 0 / 不通 2;
	// 单独探好的那个 → 1.5 秒通过。
	//
	// 代价:整轮是串行的,单节点上限 20 秒,所以一轮最多消化 ~15 个挂掉的节点
	// (5 分钟周期)。节点更多时下一次 tick 会被丢掉,相当于自动降频,不会堆积。
	nodeProbeConcurrency = 1
	// nodeProbePerNodeTimeout 单个节点的探测上限。连不上的节点不该拖住整轮。
	nodeProbePerNodeTimeout = 20 * time.Second
	// nodeProbeFailK 连续几次失败才判定不可用并告警。单次失败多半是这一刻的抖动。
	nodeProbeFailK = 2

	// settingNodeProbeEnabled 功能总开关(默认关 —— 这是会起进程的后台任务,不该默认开)。
	settingNodeProbeEnabled = "node_probe_enabled"
	// settingNodeProbeTesterID 探测源测速端 ID。空/0 = 用主控本机。
	settingNodeProbeTesterID = "node_probe_tester_id"
	// settingNodeProbeResyncMinutes 节点掉线满 N 分钟后自动重新同步其所属用户的外部订阅。
	// 空/0 = 关闭。机场换服务器时订阅里的节点信息会变,重新拉一次往往就自愈了。
	settingNodeProbeResyncMinutes = "node_probe_resync_minutes"
	// nodeProbeResyncMinInterval 同一用户两次自动重同步的最小间隔。
	// 重同步要对外发 HTTP 拉订阅,节点永久挂掉时不能每轮都去敲机场的接口。
	nodeProbeResyncMinInterval = 15 * time.Minute
)

// NodeProbeScheduler 外部节点探测调度器。
type NodeProbeScheduler struct {
	repo   *storage.TrafficRepository
	store  *NodeProbeStore
	tester *SpeedTesterWSHandler
	// checkpointPath 内存 ring 的落盘路径,重启后恢复历史。
	checkpointPath string
	// subscribeDir 外部订阅同步需要的订阅文件目录。
	subscribeDir string
	// lastResync 按用户记的上次自动重同步时间,配合 nodeProbeResyncMinInterval 节流。
	resyncMu   sync.Mutex
	lastResync map[string]time.Time
}

func NewNodeProbeScheduler(repo *storage.TrafficRepository, store *NodeProbeStore,
	tester *SpeedTesterWSHandler, checkpointPath, subscribeDir string) *NodeProbeScheduler {
	return &NodeProbeScheduler{
		repo: repo, store: store, tester: tester,
		checkpointPath: checkpointPath, subscribeDir: subscribeDir,
		lastResync: make(map[string]time.Time),
	}
}

// Start 启动后台探测循环。功能默认关闭,每轮开始时读开关 —— 这样用户在面板上
// 开/关不需要重启主控。
func (s *NodeProbeScheduler) Start(ctx context.Context) {
	if s == nil || s.repo == nil || s.store == nil {
		return
	}
	if s.checkpointPath != "" {
		if err := s.store.loadCheckpoint(s.checkpointPath); err != nil && !strings.Contains(err.Error(), "no such file") {
			log.Printf("[NodeProbe] 恢复 checkpoint 失败(不影响探测,只是丢历史): %v", err)
		}
		go s.runCheckpointLoop(ctx)
	}
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(45 * time.Second): // 错开启动风暴:此时 agent 正在重连、限速配置正在推
		}
		s.runCycle(ctx)
		ticker := time.NewTicker(nodeProbeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runCycle(ctx)
			}
		}
	}()
}

func (s *NodeProbeScheduler) runCheckpointLoop(ctx context.Context) {
	ticker := time.NewTicker(nodeProbeCheckpointInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// 退出前存一次,免得最后一段历史白丢
			if err := s.store.saveCheckpoint(s.checkpointPath); err != nil {
				log.Printf("[NodeProbe] 退出前保存 checkpoint 失败: %v", err)
			}
			return
		case <-ticker.C:
			if err := s.store.saveCheckpoint(s.checkpointPath); err != nil {
				log.Printf("[NodeProbe] 保存 checkpoint 失败: %v", err)
			}
		}
	}
}

// runCycle 跑一轮:取待探测节点 → 限流并发探测 → 记录 → 按需告警。
func (s *NodeProbeScheduler) runCycle(ctx context.Context) {
	if v, _ := s.repo.GetSystemSetting(ctx, settingNodeProbeEnabled); v != "1" {
		return // 总开关关闭
	}
	targets, err := s.repo.ListProbeEnabledNodes(ctx)
	if err != nil {
		log.Printf("[NodeProbe] 取待探测节点失败: %v", err)
		return
	}
	if len(targets) == 0 {
		return
	}

	testerID := s.resolveTesterID(ctx)
	resyncMinutes := s.resolveResyncMinutes(ctx)
	started := time.Now()

	// 探测完成后把已删除/已取消勾选的节点从内存里清掉,否则 map 只增不减。
	alive := make(map[int64]struct{}, len(targets))
	for _, t := range targets {
		alive[t.ID] = struct{}{}
	}
	defer s.store.RetainOnly(alive)

	sem := make(chan struct{}, nodeProbeConcurrency)
	var wg sync.WaitGroup
	var okCount, failCount int
	var mu sync.Mutex

	for _, target := range targets {
		select {
		case <-ctx.Done():
			return
		default:
		}
		wg.Add(1)
		go func(t storage.ProbeTargetNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			latency, ok, source := s.probeOne(ctx, t, testerID)
			state := s.store.Record(t.ID, latency, ok, source)

			mu.Lock()
			if ok {
				okCount++
			} else {
				failCount++
			}
			mu.Unlock()

			s.reconcileNodeState(ctx, t, state, resyncMinutes)
		}(target)
	}
	wg.Wait()

	log.Printf("[NodeProbe] 一轮完成:%d 个节点(通 %d / 不通 %d),来源=%s,耗时 %s",
		len(targets), okCount, failCount, testerSourceLabel(testerID), time.Since(started).Round(time.Millisecond))
}

// probeOne 探测单个节点,返回 (延迟毫秒, 是否成功, 执行端标签)。
//
// 优先派发给配置的测速端 —— 家用宽带测出来的延迟才代表用户的真实体验,
// 主控通常在海外机房,数字好看但没参考价值。测速端不可用时回退主控本机:
// 宁可拿到一个"仅供参考"的数字,也好过整个功能因为测速端离线而静默停摆。
func (s *NodeProbeScheduler) probeOne(ctx context.Context, t storage.ProbeTargetNode, testerID int64) (int64, bool, string) {
	probeCtx, cancel := context.WithTimeout(ctx, nodeProbePerNodeTimeout)
	defer cancel()

	if testerID > 0 && s.tester != nil {
		// latencyOnly=true:定时任务只测连通性和延迟,不跑吞吐 ——
		// 每个节点每 5 分钟下几十 MB,用户的流量和节点的带宽都遭不住。
		res, err := s.tester.Dispatch(probeCtx, testerID, t.ClashConfig, 0, "", 1, true)
		if err == nil {
			return res.LatencyMs, res.LatencyMs > 0, "tester#" + strconv.FormatInt(testerID, 10)
		}
		log.Printf("[NodeProbe] 测速端 %d 探测节点 %q 失败,回退主控本机: %v", testerID, t.NodeName, err)
	}

	bin, err := speedtest.EnsureMihomo(probeCtx)
	if err != nil {
		log.Printf("[NodeProbe] 主控本机没有可用的 mihomo,跳过节点 %q: %v", t.NodeName, err)
		return 0, false, "master(mihomo 缺失)"
	}
	res, err := speedtest.RunNodeTest(probeCtx, bin, t.ClashConfig, speedtest.Options{LatencyOnly: true})
	if err != nil {
		return 0, false, "master"
	}
	return res.LatencyMs, res.LatencyMs > 0, "master"
}

// reconcileNodeState 按「连续 K 次失败 → 判不可用;恢复 → 判恢复」处理状态翻转,
// 只在翻转时动作,不每轮刷屏。
//
// 两件事各自独立开关,互不牵连:Telegram 通知(通知设置)、掉线自动重同步外部订阅
// (订阅设置)。
func (s *NodeProbeScheduler) reconcileNodeState(ctx context.Context, t storage.ProbeTargetNode,
	state NodeProbeState, resyncMinutes int) {
	down := state.FailStreak >= nodeProbeFailK && !state.Announced
	up := state.FailStreak == 0 && state.Announced

	if down || up {
		s.notifyStateChange(ctx, t, state, down)
		s.store.MarkAnnounced(t.ID, down)
		if down {
			log.Printf("[NodeProbe] 节点 %q 连续 %d 次探测失败,判定不可用", t.NodeName, state.FailStreak)
		} else {
			log.Printf("[NodeProbe] 节点 %q 已恢复", t.NodeName)
		}
	}

	// 重同步与「是否刚翻转」无关:节点可能已经挂了很久,阈值也可能是用户后改大的。
	if resyncMinutes > 0 && state.FailStreak >= nodeProbeFailK {
		s.maybeResyncSubscriptions(ctx, t, state, resyncMinutes)
	}
}

// notifyStateChange 发 Telegram 通知。开关在通知设置里,由 notify 层的 CheckEnabled 判定。
func (s *NodeProbeScheduler) notifyStateChange(ctx context.Context, t storage.ProbeTargetNode,
	state NodeProbeState, down bool) {
	n := GetNotifier()
	if n == nil {
		return
	}
	if down {
		go n.Send(ctx, notify.Event{
			Type:  notify.EventNodeProbeOffline,
			Title: "节点不可用",
			Message: fmt.Sprintf("节点 `%s`(%s)连续 %d 次探测失败,已判定不可用。",
				t.NodeName, t.Protocol, state.FailStreak),
		})
		return
	}
	go n.Send(ctx, notify.Event{
		Type:  notify.EventNodeProbeOnline,
		Title: "节点已恢复",
		Message: fmt.Sprintf("节点 `%s`(%s)探测已恢复,当前延迟 %d ms。",
			t.NodeName, t.Protocol, lastLatency(state)),
	})
}

// maybeResyncSubscriptions 节点掉线满 N 分钟后,重新拉一次该用户的外部订阅。
//
// 机场换机器时订阅里的 server/port 会变,而本地节点还指着旧地址 —— 重新同步一次
// 往往就自愈了。按用户维度同步(syncExternalSubscriptions 的粒度就是用户),
// 同一用户有多个节点同时掉线也只会触发一次。
func (s *NodeProbeScheduler) maybeResyncSubscriptions(ctx context.Context, t storage.ProbeTargetNode,
	state NodeProbeState, resyncMinutes int) {
	if t.Username == "" || s.subscribeDir == "" {
		return
	}
	downFor := downDuration(state)
	if downFor < time.Duration(resyncMinutes)*time.Minute {
		return
	}

	// 冷却:重同步要对外发 HTTP 拉订阅。节点永久挂掉时不能每轮都去敲机场接口 ——
	// 取「用户设的阈值」与「硬下限」中较大的那个。
	cooldown := time.Duration(resyncMinutes) * time.Minute
	if cooldown < nodeProbeResyncMinInterval {
		cooldown = nodeProbeResyncMinInterval
	}
	if !s.resyncAllowed(t.Username, cooldown) {
		return
	}

	log.Printf("[NodeProbe] 节点 %q 已掉线 %s(阈值 %d 分钟),自动重新同步用户 %q 的外部订阅",
		t.NodeName, downFor.Round(time.Minute), resyncMinutes, t.Username)
	if err := syncExternalSubscriptions(ctx, s.repo, s.subscribeDir, t.Username); err != nil {
		log.Printf("[NodeProbe] 自动重同步外部订阅失败(用户 %q): %v", t.Username, err)
	}
}

// resyncAllowed 判定该用户此刻是否允许重同步,允许则同时记下时间(判定与记录必须在
// 同一把锁里,否则同一用户的多个节点并发探测时会同时通过判定、把订阅拉好几次)。
func (s *NodeProbeScheduler) resyncAllowed(username string, cooldown time.Duration) bool {
	s.resyncMu.Lock()
	defer s.resyncMu.Unlock()
	if last, ok := s.lastResync[username]; ok && time.Since(last) < cooldown {
		return false
	}
	if s.lastResync == nil {
		s.lastResync = make(map[string]time.Time)
	}
	s.lastResync[username] = time.Now()
	return true
}

// downDuration 从采样历史反推「已经连续挂了多久」。
//
// 不额外存 DownSince 字段:那样重启后(checkpoint 只存采样)会丢失,
// 而采样里本来就有每次探测的时间戳,倒着数到最后一次成功即可。
func downDuration(state NodeProbeState) time.Duration {
	var firstFailAt int64
	for i := len(state.Samples) - 1; i >= 0; i-- {
		if state.Samples[i].OK {
			break
		}
		firstFailAt = state.Samples[i].At
	}
	if firstFailAt == 0 {
		return 0
	}
	return time.Since(time.Unix(firstFailAt, 0))
}

// lastLatency 取最近一次采样的延迟,用于「已恢复」通知里报个数字。
func lastLatency(state NodeProbeState) int64 {
	if n := len(state.Samples); n > 0 {
		return state.Samples[n-1].LatencyMs
	}
	return 0
}

// resolveResyncMinutes 读「掉线满 N 分钟自动重同步」阈值;未设/非法 = 0(关闭)。
func (s *NodeProbeScheduler) resolveResyncMinutes(ctx context.Context) int {
	raw, _ := s.repo.GetSystemSetting(ctx, settingNodeProbeResyncMinutes)
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// resolveTesterID 读探测源配置;读不到或非法则返回 0(用主控本机)。
func (s *NodeProbeScheduler) resolveTesterID(ctx context.Context) int64 {
	raw, _ := s.repo.GetSystemSetting(ctx, settingNodeProbeTesterID)
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func testerSourceLabel(testerID int64) string {
	if testerID > 0 {
		return "tester#" + strconv.FormatInt(testerID, 10)
	}
	return "master"
}
