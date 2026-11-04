package notify

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Notifier manages sending notifications based on configuration.
type Notifier struct {
	mu  sync.RWMutex
	cfg Config
	// extra 额外添加的渠道(notify_channels 表);内置 Telegram 渠道由 cfg 的 BotToken / ChatID 构成。
	extra []ChannelConfig
	// workers 每个渠道一个串行发送队列(见 dispatch.go),key = 渠道 ID(内置 Telegram 为 0)。
	// 同一渠道内按 Send 先后顺序发 —— 多个 goroutine 同时 go n.Send(...) 时 HTTP 请求不会乱序
	// (典型现象:节点探测「恢复」比「离线」先收到);渠道之间互不阻塞。
	workers map[int64]*channelWorker
	sink    func(id int64, st ChannelStatus)
}

// New creates a Notifier with the given config.
func New(cfg Config) *Notifier {
	n := &Notifier{cfg: cfg, workers: map[int64]*channelWorker{}}
	n.syncWorkersLocked()
	return n
}

// UpdateConfig hot-reloads the notification configuration.
func (n *Notifier) UpdateConfig(cfg Config) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cfg = cfg
	n.syncWorkersLocked()
}

// GetConfig returns a copy of the current configuration.
func (n *Notifier) GetConfig() Config {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.cfg
}

// Close 停掉所有渠道的发送 goroutine(替换全局 Notifier 时调用)。
func (n *Notifier) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for id, w := range n.workers {
		w.close()
		delete(n.workers, id)
	}
}

// SetChannels 整份替换额外渠道(ID 必须 > 0)。已有渠道的发送队列保留,只换配置。
func (n *Notifier) SetChannels(channels []ChannelConfig) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.extra = nil
	for _, c := range channels {
		if c.ID > BuiltinTelegramID {
			n.extra = append(n.extra, c)
		}
	}
	n.syncWorkersLocked()
}

// SetStatusSink 设置发送结果的落库回调(在发送 goroutine 里同步调用,应尽快返回)。
func (n *Notifier) SetStatusSink(fn func(id int64, st ChannelStatus)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sink = fn
}

func (n *Notifier) persistStatus(id int64, st ChannelStatus) {
	n.mu.RLock()
	fn := n.sink
	n.mu.RUnlock()
	if fn != nil {
		fn(id, st)
	}
}

// ChannelStatuses 内存里各渠道最近一次发送结果(本进程启动以来)。
func (n *Notifier) ChannelStatuses() map[int64]ChannelStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make(map[int64]ChannelStatus, len(n.workers))
	for id, w := range n.workers {
		w.mu.Lock()
		out[id] = w.status
		w.mu.Unlock()
	}
	return out
}

// BuiltinTelegramChannel 由 system_config 的 token / chat_id 构成的内置 Telegram 渠道。
// 没填 token 或 chat_id 时视为未启用 —— 与多渠道之前「没配就不发」一致。
func (c Config) BuiltinTelegramChannel() ChannelConfig {
	return ChannelConfig{
		ID:      BuiltinTelegramID,
		Type:    ChannelTelegram,
		Name:    "Telegram",
		Enabled: c.BotToken != "" && c.ChatID != "" && !c.TelegramChannelDisabled,
		Params:  map[string]string{"bot_token": c.BotToken, "chat_id": c.ChatID},
		Events:  c.TelegramEvents,
	}
}

func (n *Notifier) syncWorkersLocked() {
	want := map[int64]ChannelConfig{BuiltinTelegramID: n.cfg.BuiltinTelegramChannel()}
	for _, c := range n.extra {
		want[c.ID] = c
	}
	for id, w := range n.workers {
		if _, ok := want[id]; !ok {
			w.close()
			delete(n.workers, id)
		}
	}
	for id, c := range want {
		if w, ok := n.workers[id]; ok {
			w.update(c)
		} else {
			n.workers[id] = newChannelWorker(n, c)
		}
	}
}

// targetsLocked 要接收该事件的渠道,按 ID 排序(内置 Telegram 在前)。
func (n *Notifier) targetsLocked(t EventType) []*channelWorker {
	ids := make([]int64, 0, len(n.workers))
	for id := range n.workers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out []*channelWorker
	for _, id := range ids {
		if w := n.workers[id]; w.eligible(t) {
			out = append(out, w)
		}
	}
	return out
}

// SkipReason 取值:enabled / global_disabled / bot_token_empty / chat_id_empty / event_disabled / no_channel,
// 排查"为什么有时不发"用。
type SkipReason string

const (
	ReasonEnabled        SkipReason = ""
	ReasonGlobalDisabled SkipReason = "global_disabled"
	ReasonBotTokenEmpty  SkipReason = "bot_token_empty"
	ReasonChatIDEmpty    SkipReason = "chat_id_empty"
	ReasonEventDisabled  SkipReason = "event_disabled"
	// ReasonNoChannel 事件开着,但没有启用的渠道订阅了它(或额外渠道都没配好)。
	ReasonNoChannel SkipReason = "no_channel"
)

// IsEnabled checks whether the given event type should trigger a notification.
func (n *Notifier) IsEnabled(eventType EventType) bool {
	ok, _ := n.CheckEnabled(eventType)
	return ok
}

// CheckEnabled 返回 (是否启用, 不启用原因)。
//   - 全局开 + 该事件开关打开 + 至少一个启用的渠道订阅了它:(true, ReasonEnabled)
//   - 全局关 / 该事件开关 off / 没有渠道接收:(false, 对应 reason)
//   - 只有内置 Telegram 一个渠道时,缺 token / chat_id 仍报 bot_token_empty / chat_id_empty(排障口径不变)
func (n *Notifier) CheckEnabled(eventType EventType) (bool, SkipReason) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if !n.cfg.Enabled {
		return false, ReasonGlobalDisabled
	}
	if !n.hasExtraEnabledLocked() {
		if n.cfg.BotToken == "" {
			return false, ReasonBotTokenEmpty
		}
		if n.cfg.ChatID == "" {
			return false, ReasonChatIDEmpty
		}
	}

	var on bool
	switch eventType {
	case EventSubscribeFetch:
		on = n.cfg.NotifySubscribeFetch
	case EventLogin:
		on = n.cfg.NotifyLogin
	case EventIPBan:
		on = n.cfg.NotifyIPBan
	case EventSilentMode:
		on = n.cfg.NotifySilentMode
	case EventDailyTraffic:
		on = n.cfg.NotifyDailyTraffic
	case EventExpiry:
		on = n.cfg.NotifyExpiry
	case EventNodeProbeOffline:
		on = n.cfg.NotifyNodeProbeOffline
	case EventNodeProbeOnline:
		on = n.cfg.NotifyNodeProbeOnline
	default:
		on = false
	}
	if !on {
		return false, ReasonEventDisabled
	}
	if len(n.targetsLocked(eventType)) == 0 {
		return false, ReasonNoChannel
	}
	return true, ReasonEnabled
}

func (n *Notifier) hasExtraEnabledLocked() bool {
	for _, c := range n.extra {
		if c.Enabled {
			return true
		}
	}
	return false
}

// Send 把事件交给每个订阅了它的渠道的发送队列,并等各渠道发完。
// 任一渠道发送成功即返回 nil(告警已送达;失败的渠道各自记 last_error 并打日志),
// 全部失败才返回错误。
func (n *Notifier) Send(ctx context.Context, event Event) error {
	if ok, _ := n.CheckEnabled(event.Type); !ok {
		return nil
	}
	n.mu.RLock()
	targets := n.targetsLocked(event.Type)
	n.mu.RUnlock()
	msg := Msg{Title: event.Title, Body: event.Message, Event: event.Type}

	dones := make([]chan error, len(targets))
	var errs []error
	for i, w := range targets {
		dones[i] = make(chan error, 1)
		if err := w.enqueue(sendJob{ctx: ctx, msg: msg, done: dones[i]}); err != nil {
			dones[i] <- err
		}
	}
	ok := false
	for i, done := range dones {
		select {
		case err := <-done:
			if err == nil {
				ok = true
			} else {
				errs = append(errs, fmt.Errorf("%s: %w", targets[i].name(), err))
			}
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("%s: %w", targets[i].name(), ctx.Err()))
		}
	}
	if ok || len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

const testTitle = "测试通知"
const testBody = "喵喵喵喵屋通知配置成功 ✓"

// SendTest sends a test message regardless of event type toggles.
func (n *Notifier) SendTest(ctx context.Context) error {
	cfg := n.GetConfig()
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return fmt.Errorf("bot token or chat ID is empty")
	}
	return sendTelegram(ctx, cfg.BotToken, cfg.ChatID, composeText(testTitle, testBody))
}

// TestChannel 用给定(可能尚未保存)的配置直接发一条测试消息,不经发送队列、不看启用状态。
func TestChannel(ctx context.Context, c ChannelConfig) error {
	ch, err := NewChannel(c)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	name := c.Name
	if name == "" {
		name = string(c.Type)
	}
	return ch.Send(ctx, Msg{Title: testTitle, Body: testBody + "\n渠道: " + name})
}
