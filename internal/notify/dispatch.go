package notify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// 每个渠道一个串行队列(worker goroutine):
//   - 同一渠道内严格按 Send 的先后顺序发(节点探测的「恢复」不会比「离线」先到)
//   - 渠道之间互不阻塞:钉钉被限流退避时,Telegram 照常发
//   - 按服务商的限频做令牌桶,收到「发太快」再退避一段时间

const (
	channelQueueSize = 200
	// 排队等令牌 / 等退避结束最多这么久,再久就丢弃并记错误 —— 告警晚到十几分钟已经没有意义。
	maxQueueWait = 2 * time.Minute
	sendTimeout  = 15 * time.Second
	// 成功状态最多这么久落一次库;错误变化时立即落库。
	statusPersistInterval = 10 * time.Minute
)

// channelLimiter 各服务商的限频。Telegram / Bark 不限(保持与多渠道之前一致)。
func channelLimiter(t ChannelType) *rate.Limiter {
	switch t {
	case ChannelDingTalk:
		return rate.NewLimiter(rate.Every(3*time.Second), 20) // 20 条/分钟
	case ChannelFeishu:
		return rate.NewLimiter(rate.Limit(5), 5) // 5 条/秒
	case ChannelServerChan:
		return rate.NewLimiter(rate.Every(time.Second), 5)
	case ChannelSMTP:
		// 邮箱服务商对短时间大量发信很敏感(QQ / 163 会临时封发信),一阵告警别一口气全发出去
		return rate.NewLimiter(rate.Every(2*time.Second), 10)
	}
	return nil
}

type sendJob struct {
	ctx  context.Context
	msg  Msg
	done chan error
}

type channelWorker struct {
	n *Notifier

	mu      sync.Mutex
	cfg     ChannelConfig
	ch      Channel // nil = 配置不可用(缺参数等),不参与分发
	chErr   error
	limiter *rate.Limiter

	// 退避:收到 rateLimitError 后暂停到 pausedUntil;backoff 每次翻倍,成功后清零。
	pausedUntil time.Time
	backoff     time.Duration

	status        ChannelStatus
	lastPersistOK int64

	queue chan sendJob
	stop  chan struct{}
}

func newChannelWorker(n *Notifier, cfg ChannelConfig) *channelWorker {
	w := &channelWorker{n: n, queue: make(chan sendJob, channelQueueSize), stop: make(chan struct{})}
	w.update(cfg)
	go w.run()
	return w
}

// update 换配置。限频器只在类型变了时重建,免得改个名字就把桶清空。
func (w *channelWorker) update(cfg ChannelConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.limiter == nil || w.cfg.Type != cfg.Type {
		w.limiter = channelLimiter(cfg.Type)
	}
	w.cfg = cfg
	w.ch, w.chErr = nil, nil
	if cfg.Enabled {
		w.ch, w.chErr = NewChannel(cfg)
	}
}

// eligible 该渠道是否要接这个事件。
func (w *channelWorker) eligible(t EventType) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cfg.Enabled && w.ch != nil && w.cfg.Subscribed(t)
}

func (w *channelWorker) name() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cfg.Name != "" {
		return w.cfg.Name
	}
	return string(w.cfg.Type)
}

func (w *channelWorker) enqueue(job sendJob) error {
	select {
	case <-w.stop:
		return errors.New("渠道已移除")
	default:
	}
	select {
	case w.queue <- job:
		return nil
	default:
		err := fmt.Errorf("发送队列已满(%d 条),丢弃", channelQueueSize)
		w.record(err)
		return err
	}
}

func (w *channelWorker) run() {
	for {
		select {
		case <-w.stop:
			// 还在排队的直接回错误,别让等结果的调用方卡住
			for {
				select {
				case job := <-w.queue:
					job.done <- errors.New("渠道已移除")
				default:
					return
				}
			}
		case job := <-w.queue:
			job.done <- w.process(job)
		}
	}
}

func (w *channelWorker) process(job sendJob) error {
	w.mu.Lock()
	ch, limiter, paused := w.ch, w.limiter, w.pausedUntil
	w.mu.Unlock()
	if ch == nil {
		return errors.New("渠道已停用")
	}
	if wait := time.Until(paused); wait > 0 {
		if wait > maxQueueWait {
			err := fmt.Errorf("服务商限流,暂停发送到 %s,本条丢弃", paused.Format("15:04:05"))
			w.record(err)
			return err
		}
		select {
		case <-time.After(wait):
		case <-job.ctx.Done():
			return job.ctx.Err()
		case <-w.stop:
			return errors.New("渠道已移除")
		}
	}
	if limiter != nil {
		wctx, cancel := context.WithTimeout(job.ctx, maxQueueWait)
		err := limiter.Wait(wctx)
		cancel()
		if err != nil {
			err = fmt.Errorf("超出发送频率限制,本条丢弃: %w", err)
			w.record(err)
			return err
		}
	}
	sctx, cancel := context.WithTimeout(job.ctx, sendTimeout)
	err := ch.Send(sctx, job.msg)
	cancel()

	var rl *rateLimitError
	w.mu.Lock()
	if errors.As(err, &rl) {
		w.backoff = nextBackoff(w.backoff, rl.retryAfter)
		w.pausedUntil = time.Now().Add(w.backoff)
	} else if err == nil {
		w.backoff, w.pausedUntil = 0, time.Time{}
	}
	w.mu.Unlock()
	w.record(err)
	if err != nil {
		log.Printf("[Notify] 渠道 %s 发送失败 event=%s: %v", w.name(), job.msg.Event, err)
	}
	return err
}

// nextBackoff 服务商给了等待时间就用它,否则从 30 秒起每次翻倍,封顶 10 分钟。
func nextBackoff(prev, hint time.Duration) time.Duration {
	next := 30 * time.Second
	if prev > 0 {
		next = prev * 2
	}
	if hint > 0 {
		next = hint
	}
	if next > 10*time.Minute {
		next = 10 * time.Minute
	}
	return next
}

// record 记最近一次结果;错误内容变了、或由错转好、或距上次落库超过 statusPersistInterval 才回调落库,
// 订阅获取这类高频通知不会每条都写一次库。
func (w *channelWorker) record(err error) {
	now := time.Now().Unix()
	w.mu.Lock()
	id := w.cfg.ID
	persist := false
	if err != nil {
		msg := err.Error()
		persist = msg != w.status.LastError
		w.status.LastError, w.status.LastErrorAt = msg, now
	} else {
		persist = w.status.LastError != "" || now-w.lastPersistOK >= int64(statusPersistInterval/time.Second)
		w.status.LastError, w.status.LastOKAt = "", now
	}
	if persist && err == nil {
		w.lastPersistOK = now
	}
	st := w.status
	w.mu.Unlock()
	if persist {
		w.n.persistStatus(id, st)
	}
}

func (w *channelWorker) close() {
	close(w.stop)
}
