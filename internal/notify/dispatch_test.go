package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTelegram 起一个假 Bot API,记录收到的 text。
func fakeTelegram(t *testing.T) (*capture, func()) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(cap.handler(200, `{"ok":true}`))
	old := telegramAPIBase
	telegramAPIBase = srv.URL + "/bot"
	return cap, func() { srv.Close(); telegramAPIBase = old }
}

// 老配置(只有 token / chat_id,没有任何额外渠道)零改动照旧:同样的开关判定、同样的正文、按顺序发。
func TestLegacyTelegramUnchanged(t *testing.T) {
	cap, done := fakeTelegram(t)
	defer done()
	n := New(Config{Enabled: true, BotToken: "1:T", ChatID: "9", NotifyNodeProbeOffline: true, NotifyNodeProbeOnline: true})
	defer n.Close()

	if ok, reason := n.CheckEnabled(EventLogin); ok || reason != ReasonEventDisabled {
		t.Fatalf("登录事件没开: %v %s", ok, reason)
	}
	for i := 0; i < 5; i++ {
		typ, title := EventNodeProbeOffline, "节点不可用"
		if i%2 == 1 {
			typ, title = EventNodeProbeOnline, "节点已恢复"
		}
		if err := n.Send(context.Background(), Event{Type: typ, Title: title, Message: "节点 `a\\_b`"}); err != nil {
			t.Fatal(err)
		}
	}
	if cap.count() != 5 {
		t.Fatalf("应发 5 条,实际 %d", cap.count())
	}
	for i, r := range cap.reqs {
		if r.URL.Path != "/bot1:T/sendMessage" || r.URL.Query().Get("parse_mode") != "Markdown" || r.URL.Query().Get("chat_id") != "9" {
			t.Fatalf("第 %d 条请求变了: %s", i, r.URL.String())
		}
		want := "*节点不可用*\n节点 `a\\_b`"
		if i%2 == 1 {
			want = "*节点已恢复*\n节点 `a\\_b`"
		}
		if got := r.URL.Query().Get("text"); got != want {
			t.Fatalf("第 %d 条正文变了: %q", i, got)
		}
	}

	// 缺 token / chat_id 的排障口径不变
	if ok, reason := New(Config{Enabled: true, ChatID: "9", NotifyLogin: true}).CheckEnabled(EventLogin); ok || reason != ReasonBotTokenEmpty {
		t.Fatalf("got %v %s", ok, reason)
	}
	if ok, reason := New(Config{Enabled: true, BotToken: "t", NotifyLogin: true}).CheckEnabled(EventLogin); ok || reason != ReasonChatIDEmpty {
		t.Fatalf("got %v %s", ok, reason)
	}
	if n := New(Config{BotToken: "t", ChatID: "9", NotifyLogin: true}); n.IsEnabled(EventLogin) {
		t.Fatal("总开关关着不该发")
	}
}

// recordingChannel 测试用渠道:记录收到的消息,可设延迟与错误。
type recordingChannel struct {
	mu    sync.Mutex
	got   []Msg
	delay time.Duration
	err   error
}

func (c *recordingChannel) Send(ctx context.Context, m Msg) error {
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, m)
	return c.err
}

func (c *recordingChannel) titles() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, m := range c.got {
		out = append(out, m.Title)
	}
	return out
}

// injectChannel 把 worker 的实现换成测试渠道。
func injectChannel(t *testing.T, n *Notifier, id int64, ch Channel) {
	t.Helper()
	n.mu.RLock()
	w := n.workers[id]
	n.mu.RUnlock()
	if w == nil {
		t.Fatalf("渠道 %d 不存在", id)
	}
	w.mu.Lock()
	w.ch = ch
	w.limiter = nil
	w.mu.Unlock()
}

func barkCfg(id int64, events ...string) ChannelConfig {
	return ChannelConfig{ID: id, Type: ChannelBark, Name: "bark", Enabled: true, Params: map[string]string{"device_key": "k"}, Events: events}
}

// 事件过滤:全局开关决定「发不发」,渠道的 events 决定「发到哪」。
func TestDispatchFiltering(t *testing.T) {
	n := New(Config{Enabled: true, NotifyLogin: true, NotifyNodeProbeOffline: true, NotifyExpiry: true})
	defer n.Close()
	n.SetChannels([]ChannelConfig{
		barkCfg(1),                       // 全部
		barkCfg(2, "node_probe_offline"), // 只要节点离线
		barkCfg(3, "expiry"),             // 只要到期
		{ID: 4, Type: ChannelBark, Enabled: false, Params: map[string]string{"device_key": "k"}}, // 停用
	})
	all, offline, expiry, disabled := &recordingChannel{}, &recordingChannel{}, &recordingChannel{}, &recordingChannel{}
	injectChannel(t, n, 1, all)
	injectChannel(t, n, 2, offline)
	injectChannel(t, n, 3, expiry)
	n.mu.RLock()
	n.workers[4].ch = disabled // 停用的 worker 本来就没有实现;塞一个也不该收到
	n.mu.RUnlock()

	ctx := context.Background()
	for _, ev := range []Event{
		{Type: EventLogin, Title: "login"},
		{Type: EventNodeProbeOffline, Title: "offline"},
		{Type: EventExpiry, Title: "expiry"},
		{Type: EventSubscribeFetch, Title: "sub"}, // 全局开关关着,谁都不发
	} {
		if err := n.Send(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(all.titles(), ","); got != "login,offline,expiry" {
		t.Fatalf("全部订阅: %s", got)
	}
	if got := strings.Join(offline.titles(), ","); got != "offline" {
		t.Fatalf("只订阅离线: %s", got)
	}
	if got := strings.Join(expiry.titles(), ","); got != "expiry" {
		t.Fatalf("只订阅到期: %s", got)
	}
	if len(disabled.titles()) != 0 {
		t.Fatal("停用的渠道不该收到")
	}
	// 没有 token 的内置 Telegram + 只订阅离线的渠道:登录事件就没有渠道接收
	n.SetChannels([]ChannelConfig{barkCfg(2, "node_probe_offline")})
	if ok, reason := n.CheckEnabled(EventLogin); ok || reason != ReasonNoChannel {
		t.Fatalf("got %v %s", ok, reason)
	}
}

// 渠道之间互不阻塞:一个渠道卡住时,另一个照常按顺序发完。
func TestDispatchChannelsIndependent(t *testing.T) {
	n := New(Config{Enabled: true, NotifyLogin: true})
	defer n.Close()
	n.SetChannels([]ChannelConfig{barkCfg(1), barkCfg(2)})
	slow, fast := &recordingChannel{delay: 300 * time.Millisecond}, &recordingChannel{}
	injectChannel(t, n, 1, slow)
	injectChannel(t, n, 2, fast)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		title := string(rune('a' + i))
		go func() { defer wg.Done(); _ = n.Send(context.Background(), Event{Type: EventLogin, Title: title}) }()
		time.Sleep(20 * time.Millisecond) // 固定入队顺序
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for len(fast.titles()) < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := strings.Join(fast.titles(), ""); got != "abc" {
		t.Fatalf("快渠道应在慢渠道发完前按序发完: %q", got)
	}
	wg.Wait()
	if got := strings.Join(slow.titles(), ""); got != "abc" {
		t.Fatalf("慢渠道也要按序: %q", got)
	}
}

// 部分成功算成功;全部失败才报错;状态回调记下错误。
func TestDispatchErrorsAndStatus(t *testing.T) {
	n := New(Config{Enabled: true, NotifyLogin: true})
	defer n.Close()
	var mu sync.Mutex
	persisted := map[int64]ChannelStatus{}
	n.SetStatusSink(func(id int64, st ChannelStatus) { mu.Lock(); persisted[id] = st; mu.Unlock() })
	n.SetChannels([]ChannelConfig{barkCfg(1), barkCfg(2)})
	bad := &recordingChannel{err: errTest("boom")}
	injectChannel(t, n, 1, bad)
	injectChannel(t, n, 2, &recordingChannel{})

	if err := n.Send(context.Background(), Event{Type: EventLogin, Title: "x"}); err != nil {
		t.Fatalf("一个渠道成功就算送达: %v", err)
	}
	st := n.ChannelStatuses()
	if st[1].LastError != "boom" || st[1].LastErrorAt == 0 || st[2].LastOKAt == 0 || st[2].LastError != "" {
		t.Fatalf("状态不对: %+v", st)
	}
	mu.Lock()
	if persisted[1].LastError != "boom" || persisted[2].LastOKAt == 0 {
		t.Fatalf("状态应落库: %+v", persisted)
	}
	mu.Unlock()

	injectChannel(t, n, 2, &recordingChannel{err: errTest("down")})
	err := n.Send(context.Background(), Event{Type: EventLogin, Title: "y"})
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "down") {
		t.Fatalf("全部失败应返回合并的错误: %v", err)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// 服务商限流 → 退避:暂停期间的消息快速失败(不卡住调用方),并记错误。
func TestDispatchBackoffOnRateLimit(t *testing.T) {
	n := New(Config{Enabled: true, NotifyLogin: true})
	defer n.Close()
	n.SetChannels([]ChannelConfig{barkCfg(1)})
	limited := &recordingChannel{err: &rateLimitError{retryAfter: 5 * time.Minute, err: errTest("too fast")}}
	injectChannel(t, n, 1, limited)

	_ = n.Send(context.Background(), Event{Type: EventLogin, Title: "1"})
	start := time.Now()
	err := n.Send(context.Background(), Event{Type: EventLogin, Title: "2"})
	if err == nil || !strings.Contains(err.Error(), "限流") || time.Since(start) > time.Second {
		t.Fatalf("退避期间应立即丢弃: %v (%s)", err, time.Since(start))
	}
	if len(limited.titles()) != 1 {
		t.Fatalf("退避期间不该再请求服务商: %v", limited.titles())
	}
	if nextBackoff(0, 0) != 30*time.Second || nextBackoff(30*time.Second, 0) != time.Minute || nextBackoff(8*time.Minute, 0) != 10*time.Minute {
		t.Fatal("退避翻倍 / 封顶不对")
	}
}

// 真实 HTTP:Telegram 与飞书同时收到,且飞书拿到的是转换过的 Markdown。
func TestDispatchEndToEndHTTP(t *testing.T) {
	tg, done := fakeTelegram(t)
	defer done()
	fs := &capture{}
	srv := httptest.NewServer(fs.handler(200, `{"code":0}`))
	defer srv.Close()

	n := New(Config{Enabled: true, BotToken: "1:T", ChatID: "9", NotifyIPBan: true, TelegramEvents: []string{"login"}})
	defer n.Close()
	n.SetChannels([]ChannelConfig{{ID: 7, Type: ChannelFeishu, Name: "飞书群", Enabled: true, Params: map[string]string{"webhook": srv.URL + "/hook/x"}}})

	if err := n.Send(context.Background(), Event{Type: EventIPBan, Title: "IP 封禁", Message: "IP `1.2.3.4` 已被封禁"}); err != nil {
		t.Fatal(err)
	}
	if tg.count() != 0 {
		t.Fatal("内置 Telegram 只订阅了登录,不该收到 IP 封禁")
	}
	_, body := fs.last(t)
	var p struct {
		Card struct {
			Elements []struct{ Content string } `json:"elements"`
		} `json:"card"`
	}
	_ = json.Unmarshal([]byte(body), &p)
	if len(p.Card.Elements) != 1 || p.Card.Elements[0].Content != "IP `1.2.3.4` 已被封禁" {
		t.Fatalf("飞书正文不对: %s", body)
	}

	// 停用内置 Telegram:有 token 也不发
	n.UpdateConfig(Config{Enabled: true, BotToken: "1:T", ChatID: "9", NotifyIPBan: true, TelegramChannelDisabled: true})
	n.SetChannels(nil)
	if ok, reason := n.CheckEnabled(EventIPBan); ok || reason != ReasonNoChannel {
		t.Fatalf("got %v %s", ok, reason)
	}
}

func TestTestChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer srv.Close()
	if err := TestChannel(context.Background(), ChannelConfig{Type: ChannelBark, Name: "my_bark", Params: map[string]string{"server": srv.URL, "device_key": "k"}}); err != nil {
		t.Fatal(err)
	}
}
