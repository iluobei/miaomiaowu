package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 外部节点探测结果的内存存储。
//
// 刻意不落数据库:这是分钟级时序。姊妹项目妙妙屋X 在这类写入上栽过 —— 分钟级的
// 探测指标把 SQLite 的 WAL 顶到几十 GB(主库才十几 MB),因为 WAL 只有在「全部帧
// 已回写且当下零 reader」时才 rewind,而高频短读几乎铺满时间轴,那个瞬间永远等不到。
//
// 改用「内存 ring + 定期落盘 checkpoint」:热路径纯内存,checkpoint 只是重启后的
// 恢复点,不是可查询的数据库。

const (
	// nodeProbeRingCap 每个节点保留多少个采样点。
	// 默认 5 分钟一轮 × 288 = 24 小时,足够画一天的趋势;
	// 每点约 40 字节,1000 个节点也就 ~11MB。
	nodeProbeRingCap = 288
	// nodeProbeCheckpointInterval 落盘间隔。探测本身 5 分钟一轮,落盘不必更密。
	nodeProbeCheckpointInterval = 5 * time.Minute
)

// NodeProbeSample 一次探测的结果。
type NodeProbeSample struct {
	At        int64 `json:"at"`         // Unix 秒
	LatencyMs int64 `json:"latency_ms"` // 失败时为 0
	OK        bool  `json:"ok"`
}

// NodeProbeState 一个节点的探测状态 + 历史。
type NodeProbeState struct {
	NodeID int64 `json:"node_id"`
	// FailStreak 连续失败次数。用于「连续 K 次才判不可用」的去抖 ——
	// 单次失败很可能只是这一刻的网络抖动,不该立刻告警。
	FailStreak int               `json:"fail_streak"`
	Announced  bool              `json:"announced"` // 已就"不可用"发过公告,用于状态翻转去抖
	Samples    []NodeProbeSample `json:"samples"`
	// Source 最近一次探测的执行端,便于排查"为什么这个延迟不对"
	// (主控在海外机房,测出来的延迟和国内测速端能差很多)。
	Source string `json:"source"`
}

// NodeProbeStore 节点探测结果的内存存储。
type NodeProbeStore struct {
	mu   sync.RWMutex
	data map[int64]*NodeProbeState
	cap  int
}

func NewNodeProbeStore(capN int) *NodeProbeStore {
	if capN <= 0 {
		capN = nodeProbeRingCap
	}
	return &NodeProbeStore{data: make(map[int64]*NodeProbeState), cap: capN}
}

// Record 记录一次探测结果,返回记录后的状态快照(供调用方判断是否要告警)。
func (s *NodeProbeStore) Record(nodeID int64, latencyMs int64, ok bool, source string) NodeProbeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.data[nodeID]
	if st == nil {
		st = &NodeProbeState{NodeID: nodeID}
		s.data[nodeID] = st
	}
	st.Source = source
	if ok {
		st.FailStreak = 0
	} else {
		st.FailStreak++
	}
	st.Samples = append(st.Samples, NodeProbeSample{
		At:        time.Now().Unix(),
		LatencyMs: latencyMs,
		OK:        ok,
	})
	if len(st.Samples) > s.cap {
		// 丢最旧的。用 copy 而不是切片再切片 —— 后者会让底层数组一直不释放。
		drop := len(st.Samples) - s.cap
		copy(st.Samples, st.Samples[drop:])
		st.Samples = st.Samples[:s.cap]
	}
	snapshot := *st
	snapshot.Samples = append([]NodeProbeSample(nil), st.Samples...)
	return snapshot
}

// MarkAnnounced 记下"已就当前不可用状态发过公告",避免每轮都刷屏。
func (s *NodeProbeStore) MarkAnnounced(nodeID int64, announced bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.data[nodeID]; st != nil {
		st.Announced = announced
	}
}

// Snapshot 取一个节点的状态;不存在返回 false。
func (s *NodeProbeStore) Snapshot(nodeID int64) (NodeProbeState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.data[nodeID]
	if !ok {
		return NodeProbeState{}, false
	}
	out := *st
	out.Samples = append([]NodeProbeSample(nil), st.Samples...)
	return out, true
}

// All 取全部节点状态,供面板列表用。
func (s *NodeProbeStore) All() map[int64]NodeProbeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[int64]NodeProbeState, len(s.data))
	for id, st := range s.data {
		c := *st
		c.Samples = append([]NodeProbeSample(nil), st.Samples...)
		out[id] = c
	}
	return out
}

// Forget 节点被删除或关闭探测时清掉它的历史,否则 map 只增不减。
func (s *NodeProbeStore) Forget(nodeID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, nodeID)
}

// RetainOnly 只保留给定的节点 —— 每轮探测后调用,自动清掉已删除/已关探测的节点。
func (s *NodeProbeStore) RetainOnly(ids map[int64]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.data {
		if _, keep := ids[id]; !keep {
			delete(s.data, id)
		}
	}
}

// ---- checkpoint:重启恢复,不是数据库 ----

func (s *NodeProbeStore) saveCheckpoint(path string) error {
	s.mu.RLock()
	blob, err := json.Marshal(s.data)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 先写临时文件再 rename:进程在写一半时被杀不会留下半个 JSON,
	// 否则下次启动 Unmarshal 失败、历史全丢。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *NodeProbeStore) loadCheckpoint(path string) error {
	blob, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data map[int64]*NodeProbeState
	if err := json.Unmarshal(blob, &data); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
	if s.data == nil {
		s.data = make(map[int64]*NodeProbeState)
	}
	return nil
}
