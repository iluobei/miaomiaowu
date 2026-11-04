package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// 签名向量用 openssl 独立算出:
//
//	printf '' | openssl dgst -sha256 -mac HMAC -macopt key:"$(printf '1599360473\nSECa1b2c3')" -binary | base64
//	printf '1577262236757\nSEC123' | openssl dgst -sha256 -hmac 'SEC123' -binary | base64
func TestFeishuSignVector(t *testing.T) {
	if got := feishuSign(1599360473, "SECa1b2c3"); got != "+M7bHNlP/cc33PZa3oY22S47kovLdMAqsL4gnFsY5AQ=" {
		t.Fatalf("飞书签名不对: %s", got)
	}
}

func TestDingTalkSignVector(t *testing.T) {
	if got := dingTalkSign(1577262236757, "SEC123"); got != "Z/IOagKYTkrnYtxAsTKneRe0bzmlPCH3ZDJPTD2h9QA=" {
		t.Fatalf("钉钉签名不对: %s", got)
	}
}

// capture 记下 httptest 收到的请求。
type capture struct {
	mu    sync.Mutex
	reqs  []*http.Request
	bodys []string
}

func (c *capture) handler(status int, resp string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.reqs = append(c.reqs, r)
		c.bodys = append(c.bodys, string(b))
		c.mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}
}

func (c *capture) last(t *testing.T) (*http.Request, string) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reqs) == 0 {
		t.Fatal("没有收到请求")
	}
	return c.reqs[len(c.reqs)-1], c.bodys[len(c.bodys)-1]
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

var sampleMsg = Msg{
	Title: "节点不可用",
	Body:  "节点 `hk\\_1`(vmess)连续 3 次探测失败\nIP: 1.2.3.4",
	Event: EventNodeProbeOffline,
}

func TestFeishuChannel(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler(200, `{"code":0,"msg":"success","data":{}}`))
	defer srv.Close()
	feishuNow = func() time.Time { return time.Unix(1599360473, 0) }
	defer func() { feishuNow = time.Now }()

	ch, err := NewChannel(ChannelConfig{Type: ChannelFeishu, Params: map[string]string{"webhook": srv.URL + "/open-apis/bot/v2/hook/abc", "secret": "SECa1b2c3"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Send(context.Background(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	r, body := cap.last(t)
	if r.URL.Path != "/open-apis/bot/v2/hook/abc" {
		t.Fatalf("path %s", r.URL.Path)
	}
	var p struct {
		MsgType   string `json:"msg_type"`
		Timestamp string `json:"timestamp"`
		Sign      string `json:"sign"`
		Card      struct {
			Header struct {
				Title    struct{ Content string } `json:"title"`
				Template string                   `json:"template"`
			} `json:"header"`
			Elements []struct {
				Tag     string `json:"tag"`
				Content string `json:"content"`
			} `json:"elements"`
		} `json:"card"`
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if p.MsgType != "interactive" || p.Timestamp != "1599360473" || p.Sign != "+M7bHNlP/cc33PZa3oY22S47kovLdMAqsL4gnFsY5AQ=" {
		t.Fatalf("签名字段不对: %+v", p)
	}
	if p.Card.Header.Title.Content != "节点不可用" || p.Card.Header.Template != "red" {
		t.Fatalf("卡片标题不对: %+v", p.Card.Header)
	}
	want := "节点 `hk_1`(vmess)连续 3 次探测失败\nIP: 1.2.3.4"
	if len(p.Card.Elements) != 1 || p.Card.Elements[0].Tag != "markdown" || p.Card.Elements[0].Content != want {
		t.Fatalf("卡片正文不对: %+v", p.Card.Elements)
	}
}

func TestFeishuChannelErrors(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler(200, `{"code":19021,"msg":"sign match fail"}`))
	defer srv.Close()
	ch, _ := NewChannel(ChannelConfig{Type: ChannelFeishu, Params: map[string]string{"webhook": srv.URL + "/hook/abc"}})
	err := ch.Send(context.Background(), sampleMsg)
	if err == nil || !strings.Contains(err.Error(), "19021") {
		t.Fatalf("应报签名错误: %v", err)
	}

	srv2 := httptest.NewServer(cap.handler(200, `{"code":11232,"msg":"frequency limited"}`))
	defer srv2.Close()
	ch2, _ := NewChannel(ChannelConfig{Type: ChannelFeishu, Params: map[string]string{"webhook": srv2.URL + "/hook/abc"}})
	var rl *rateLimitError
	if err := ch2.Send(context.Background(), sampleMsg); !errors.As(err, &rl) {
		t.Fatalf("应识别为限流: %v", err)
	}
}

func TestDingTalkChannel(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler(200, `{"errcode":0,"errmsg":"ok"}`))
	defer srv.Close()
	dingTalkNow = func() time.Time { return time.UnixMilli(1577262236757) }
	defer func() { dingTalkNow = time.Now }()

	ch, err := NewChannel(ChannelConfig{Type: ChannelDingTalk, Params: map[string]string{
		"webhook": srv.URL + "/robot/send?access_token=tok123", "secret": "SEC123"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Send(context.Background(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	r, body := cap.last(t)
	q := r.URL.Query()
	if q.Get("access_token") != "tok123" || q.Get("timestamp") != "1577262236757" || q.Get("sign") != "Z/IOagKYTkrnYtxAsTKneRe0bzmlPCH3ZDJPTD2h9QA=" {
		t.Fatalf("加签参数不对: %s", r.URL.RawQuery)
	}
	// sign 必须 URL 编码(+ / = 原样放进 query 会被解成空格等)
	if !strings.Contains(r.URL.RawQuery, "sign="+url.QueryEscape("Z/IOagKYTkrnYtxAsTKneRe0bzmlPCH3ZDJPTD2h9QA=")) {
		t.Fatalf("sign 没有 URL 编码: %s", r.URL.RawQuery)
	}
	var p struct {
		MsgType  string            `json:"msgtype"`
		Markdown map[string]string `json:"markdown"`
	}
	_ = json.Unmarshal([]byte(body), &p)
	want := "**节点不可用**  \n节点 `hk_1`(vmess)连续 3 次探测失败  \nIP: 1.2.3.4"
	if p.MsgType != "markdown" || p.Markdown["title"] != "节点不可用" || p.Markdown["text"] != want {
		t.Fatalf("消息不对: %+v\n%q", p, p.Markdown["text"])
	}

	// 只填 access_token 也行
	ch2, err := NewChannel(ChannelConfig{Type: ChannelDingTalk, Params: map[string]string{"webhook": "tok456"}})
	if err != nil || ch2.(*dingTalkChannel).webhook != dingTalkSendPrefix+"tok456" {
		t.Fatalf("裸 token 应拼成完整地址: %v", err)
	}
}

func TestDingTalkRateLimited(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler(200, `{"errcode":130101,"errmsg":"send too fast"}`))
	defer srv.Close()
	ch, _ := NewChannel(ChannelConfig{Type: ChannelDingTalk, Params: map[string]string{"webhook": srv.URL + "/robot/send?access_token=x"}})
	var rl *rateLimitError
	if err := ch.Send(context.Background(), sampleMsg); !errors.As(err, &rl) || rl.retryAfter != time.Minute {
		t.Fatalf("应识别为限流并等 1 分钟: %v", err)
	}
}

func TestServerChanChannel(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler(200, `{"code":0,"message":"","data":{"pushid":"1"}}`))
	defer srv.Close()
	orig := serverChanEndpoint
	defer func() { serverChanEndpoint = orig }()
	var gotURL string
	serverChanEndpoint = func(key string) string {
		gotURL = orig(key)
		return srv.URL + "/" + key + ".send"
	}

	ch, err := NewChannel(ChannelConfig{Type: ChannelServerChan, Params: map[string]string{"sendkey": "SCT123abc"}})
	if err != nil {
		t.Fatal(err)
	}
	daily := Msg{Body: "*📊 每日流量统计*\n• a\\_b: 1GB\n• c: 2GB", Event: EventDailyTraffic}
	if err := ch.Send(context.Background(), daily); err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://sctapi.ftqq.com/SCT123abc.send" {
		t.Fatalf("Turbo 地址不对: %s", gotURL)
	}
	_, body := cap.last(t)
	var p map[string]string
	_ = json.Unmarshal([]byte(body), &p)
	if p["title"] != "📊 每日流量统计" || p["desp"] != "• a_b: 1GB  \n• c: 2GB" {
		t.Fatalf("消息不对: %+v", p)
	}

	// Server酱³ 的 SendKey 走 {uid}.push.ft07.com
	if got := orig("sctp12345tABCdef"); got != "https://12345.push.ft07.com/send/sctp12345tABCdef.send" {
		t.Fatalf("Server酱³ 地址不对: %s", got)
	}

	srvErr := httptest.NewServer(cap.handler(200, `{"code":40001,"message":"bad pushtoken"}`))
	defer srvErr.Close()
	serverChanEndpoint = func(key string) string { return srvErr.URL + "/x.send" }
	if err := ch.Send(context.Background(), sampleMsg); err == nil || !strings.Contains(err.Error(), "bad pushtoken") {
		t.Fatalf("应报服务端错误: %v", err)
	}
	if _, err := NewChannel(ChannelConfig{Type: ChannelServerChan, Params: map[string]string{"sendkey": "https://sctapi.ftqq.com/SCT1.send"}}); err == nil {
		t.Fatal("整条链接当 SendKey 应拒绝")
	}
}

func TestBarkChannel(t *testing.T) {
	cap := &capture{}
	srv := httptest.NewServer(cap.handler(200, `{"code":200,"message":"success"}`))
	defer srv.Close()

	// 直接粘贴 App 里复制的整条链接:server 与 key 都从里面取
	ch, err := NewChannel(ChannelConfig{Type: ChannelBark, Params: map[string]string{"device_key": srv.URL + "/KEY123/", "level": "timeSensitive"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Send(context.Background(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	r, body := cap.last(t)
	if r.URL.Path != "/push" {
		t.Fatalf("path %s", r.URL.Path)
	}
	var p map[string]string
	_ = json.Unmarshal([]byte(body), &p)
	if p["device_key"] != "KEY123" || p["title"] != "节点不可用" || p["group"] != "妙妙屋" ||
		p["level"] != "timeSensitive" || p["body"] != "节点 hk_1(vmess)连续 3 次探测失败\nIP: 1.2.3.4" {
		t.Fatalf("消息不对: %+v", p)
	}
	if _, err := NewChannel(ChannelConfig{Type: ChannelBark, Params: map[string]string{"device_key": "k", "level": "loud"}}); err == nil {
		t.Fatal("未知级别应拒绝")
	}
	if c, _ := NewChannel(ChannelConfig{Type: ChannelBark, Params: map[string]string{"device_key": "k"}}); c.(*barkChannel).server != barkDefaultServer {
		t.Fatal("默认服务器应为 api.day.app")
	}
}

func TestMaskAndMergeParams(t *testing.T) {
	saved := map[string]string{"webhook": "https://open.feishu.cn/open-apis/bot/v2/hook/abcdef123456", "secret": "SECabcdefgh"}
	masked := MaskParams(ChannelFeishu, saved)
	if masked["webhook"] != "https://open.feishu.cn/****3456" || masked["secret"] != "****efgh" {
		t.Fatalf("掩码不对: %+v", masked)
	}
	merged := MergeParams(ChannelFeishu, map[string]string{"webhook": masked["webhook"], "secret": " NEWSEC "}, saved)
	if merged["webhook"] != saved["webhook"] || merged["secret"] != "NEWSEC" {
		t.Fatalf("合并不对: %+v", merged)
	}
	// Bark 的 server / group 不是密钥,原样回显
	if m := MaskParams(ChannelBark, map[string]string{"server": "https://bark.example", "device_key": "KEY1234567890"}); m["server"] != "https://bark.example" || m["device_key"] != "****7890" {
		t.Fatalf("Bark 掩码不对: %+v", m)
	}
}

func TestValidateChannel(t *testing.T) {
	if err := ValidateChannel(ChannelConfig{Type: ChannelFeishu, Enabled: true, Params: map[string]string{}}); err == nil {
		t.Fatal("缺 webhook 应拒绝")
	}
	if err := ValidateChannel(ChannelConfig{Type: "wechat"}); err == nil {
		t.Fatal("未知类型应拒绝")
	}
}
