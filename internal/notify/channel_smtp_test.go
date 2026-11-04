package notify

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP 最小 SMTP 服务端:EHLO / STARTTLS / AUTH PLAIN|LOGIN / MAIL / RCPT / DATA / QUIT。
type fakeSMTP struct {
	ln        net.Listener
	tlsCfg    *tls.Config
	startTLS  bool   // EHLO 里是否宣告 STARTTLS
	authMechs string // EHLO 里宣告的 AUTH 方式,空 = 不宣告
	authReply string // AUTH 的回应,默认 235
	rcptReply string // RCPT 的回应,默认 250

	mu         sync.Mutex
	upgraded   bool
	authMech   string
	authUser   string
	authPass   string
	mailFrom   string
	rcpts      []string
	data       string
	sessionsOK int
}

// localTLS 借 httptest 的自签证书(对 127.0.0.1 有效),并让客户端信任它。
func localTLS(t *testing.T) *tls.Config {
	t.Helper()
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	old := smtpTLSRoots
	smtpTLSRoots = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	t.Cleanup(func() { smtpTLSRoots = old })
	return &tls.Config{Certificates: srv.TLS.Certificates}
}

func startFakeSMTP(t *testing.T, f *fakeSMTP, directTLS bool) (host string, port string) {
	t.Helper()
	var err error
	if directTLS {
		f.ln, err = tls.Listen("tcp", "127.0.0.1:0", f.tlsCfg)
	} else {
		f.ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.ln.Close() })
	go func() {
		for {
			c, err := f.ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	h, p, _ := net.SplitHostPort(f.ln.Addr().String())
	return h, p
}

func (f *fakeSMTP) serve(raw net.Conn) {
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
	_, isTLS := raw.(*tls.Conn)
	tp := textproto.NewConn(raw)
	_ = tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			lines := []string{"fake"}
			if f.startTLS && !isTLS {
				lines = append(lines, "STARTTLS")
			}
			if f.authMechs != "" {
				lines = append(lines, "AUTH "+f.authMechs)
			}
			lines = append(lines, "8BITMIME")
			for i, l := range lines {
				sep := "-"
				if i == len(lines)-1 {
					sep = " "
				}
				_ = tp.PrintfLine("250%s%s", sep, l)
			}
		case "STARTTLS":
			_ = tp.PrintfLine("220 go ahead")
			tc := tls.Server(raw, f.tlsCfg)
			if err := tc.Handshake(); err != nil {
				return
			}
			raw, isTLS = tc, true
			tp = textproto.NewConn(tc)
			f.mu.Lock()
			f.upgraded = true
			f.mu.Unlock()
		case "AUTH":
			mech, initial, _ := strings.Cut(arg, " ")
			var user, pass string
			switch strings.ToUpper(mech) {
			case "PLAIN":
				b, _ := base64.StdEncoding.DecodeString(initial)
				parts := strings.Split(string(b), "\x00")
				if len(parts) == 3 {
					user, pass = parts[1], parts[2]
				}
			case "LOGIN":
				_ = tp.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
				l, _ := tp.ReadLine()
				b, _ := base64.StdEncoding.DecodeString(l)
				user = string(b)
				_ = tp.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
				l, _ = tp.ReadLine()
				b, _ = base64.StdEncoding.DecodeString(l)
				pass = string(b)
			}
			f.mu.Lock()
			f.authMech, f.authUser, f.authPass = strings.ToUpper(mech), user, pass
			f.mu.Unlock()
			reply := f.authReply
			if reply == "" {
				reply = "235 ok"
			}
			_ = tp.PrintfLine("%s", reply)
		case "MAIL":
			f.mu.Lock()
			f.mailFrom = arg
			f.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case "RCPT":
			f.mu.Lock()
			f.rcpts = append(f.rcpts, arg)
			f.mu.Unlock()
			reply := f.rcptReply
			if reply == "" {
				reply = "250 ok"
			}
			_ = tp.PrintfLine("%s", reply)
		case "DATA":
			_ = tp.PrintfLine("354 go")
			lines, err := tp.ReadDotLines()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.data = strings.Join(lines, "\r\n")
			f.sessionsOK++
			f.mu.Unlock()
			_ = tp.PrintfLine("250 queued")
		case "RSET", "NOOP":
			_ = tp.PrintfLine("250 ok")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
	}
}

// parsedMail 解出主题与正文。
func parsedMail(t *testing.T, data string) (*mail.Message, string, string) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatalf("邮件解析失败: %v\n%s", err, data)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatal(err)
	}
	return msg, subject, string(body)
}

func TestSMTPChannelPlainAuthAndContent(t *testing.T) {
	f := &fakeSMTP{authMechs: "PLAIN LOGIN"}
	host, port := startFakeSMTP(t, f, false)
	ch, err := NewChannel(ChannelConfig{Type: ChannelSMTP, Params: map[string]string{
		"host": host, "port": port, "security": "none",
		"username": "bot@example.com", "password": "p@ss word",
		"from_name": "妙妙屋 告警",
		"to":        "a@example.com; 运维 <b@example.com>\nA@example.com",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Send(context.Background(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.authMech != "PLAIN" || f.authUser != "bot@example.com" || f.authPass != "p@ss word" {
		t.Fatalf("AUTH 不对: %s %q %q", f.authMech, f.authUser, f.authPass)
	}
	if !strings.HasPrefix(f.mailFrom, "FROM:<bot@example.com>") {
		t.Fatalf("发件人(from 为空时用用户名)不对: %q", f.mailFrom)
	}
	// 大小写不同的同一地址只发一次
	if len(f.rcpts) != 2 || f.rcpts[0] != "TO:<a@example.com>" || f.rcpts[1] != "TO:<b@example.com>" {
		t.Fatalf("收件人不对: %v", f.rcpts)
	}
	msg, subject, body := parsedMail(t, f.data)
	if subject != "节点不可用" {
		t.Fatalf("主题不对: %q", subject)
	}
	from, _ := msg.Header.AddressList("From")
	if len(from) != 1 || from[0].Name != "妙妙屋 告警" || from[0].Address != "bot@example.com" {
		t.Fatalf("From 头不对: %v", from)
	}
	if to, _ := msg.Header.AddressList("To"); len(to) != 2 || to[1].Name != "运维" {
		t.Fatalf("To 头不对: %v", to)
	}
	if msg.Header.Get("X-Miaomiaowu-Event") != string(EventNodeProbeOffline) || msg.Header.Get("Message-ID") == "" || msg.Header.Get("Date") == "" {
		t.Fatalf("缺头: %v", msg.Header)
	}
	// Markdown 标记与转义都要去掉
	if !strings.Contains(body, "节点 hk_1(vmess)连续 3 次探测失败\r\nIP: 1.2.3.4") || strings.ContainsAny(body, "`\\*") {
		t.Fatalf("正文不对: %q", body)
	}
}

func TestSMTPChannelLoginAuthOverDirectTLS(t *testing.T) {
	f := &fakeSMTP{authMechs: "LOGIN", tlsCfg: localTLS(t)}
	host, port := startFakeSMTP(t, f, true)
	ch, err := NewChannel(ChannelConfig{Type: ChannelSMTP, Params: map[string]string{
		"host": host, "port": port, "security": "ssl",
		"username": "u1", "password": "secret", "from": "noreply@example.com", "to": "a@example.com",
	}})
	if err != nil {
		t.Fatal(err)
	}
	// 无标题消息取正文首行当主题
	if err := ch.Send(context.Background(), Msg{Body: "*📊 每日流量*\n今日 1.2 GB"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.authMech != "LOGIN" || f.authUser != "u1" || f.authPass != "secret" {
		t.Fatalf("LOGIN 不对: %s %q %q", f.authMech, f.authUser, f.authPass)
	}
	_, subject, body := parsedMail(t, f.data)
	if subject != "📊 每日流量" || strings.TrimSpace(body) != "今日 1.2 GB" {
		t.Fatalf("主题/正文不对: %q %q", subject, body)
	}
}

func TestSMTPChannelStartTLS(t *testing.T) {
	cfg := localTLS(t)
	// 服务器支持 STARTTLS:先升级再登录(PlainAuth 对非本机明文会拒绝,这里本机也照样走 TLS)
	f := &fakeSMTP{startTLS: true, authMechs: "PLAIN", tlsCfg: cfg}
	host, port := startFakeSMTP(t, f, false)
	params := map[string]string{"host": host, "port": port, "username": "u", "password": "p", "from": "x@example.com", "to": "a@example.com"}
	ch, err := NewChannel(ChannelConfig{Type: ChannelSMTP, Params: params}) // security 默认 starttls
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Send(context.Background(), sampleMsg); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	if !f.upgraded || f.authUser != "u" || f.sessionsOK != 1 {
		t.Fatalf("STARTTLS 没生效: upgraded=%v auth=%q", f.upgraded, f.authUser)
	}
	f.mu.Unlock()

	// 服务器不宣告 STARTTLS:不能降级成明文,直接报错
	f2 := &fakeSMTP{authMechs: "PLAIN", tlsCfg: cfg}
	host2, port2 := startFakeSMTP(t, f2, false)
	params["host"], params["port"] = host2, port2
	ch2, _ := NewChannel(ChannelConfig{Type: ChannelSMTP, Params: params})
	err = ch2.Send(context.Background(), sampleMsg)
	if err == nil || !strings.Contains(err.Error(), "不支持 STARTTLS") {
		t.Fatalf("应报不支持 STARTTLS: %v", err)
	}
	f2.mu.Lock()
	defer f2.mu.Unlock()
	if f2.authUser != "" || f2.sessionsOK != 0 {
		t.Fatal("没升级成功不该继续登录 / 发信")
	}
}

func TestSMTPChannelErrors(t *testing.T) {
	f := &fakeSMTP{authMechs: "PLAIN", authReply: "535 5.7.8 auth failed"}
	host, port := startFakeSMTP(t, f, false)
	ch, _ := NewChannel(ChannelConfig{Type: ChannelSMTP, Params: map[string]string{
		"host": host, "port": port, "security": "none", "username": "u", "password": "wrong", "from": "x@example.com", "to": "a@example.com",
	}})
	err := ch.Send(context.Background(), sampleMsg)
	if err == nil || !strings.Contains(err.Error(), "登录") || !strings.Contains(err.Error(), "授权码") || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("登录失败提示不对: %v", err)
	}

	// 421 视为限流,交给 dispatch 退避
	f2 := &fakeSMTP{rcptReply: "421 4.7.0 too many messages"}
	host2, port2 := startFakeSMTP(t, f2, false)
	ch2, _ := NewChannel(ChannelConfig{Type: ChannelSMTP, Params: map[string]string{
		"host": host2, "port": port2, "security": "none", "from": "x@example.com", "to": "a@example.com",
	}})
	var rl *rateLimitError
	if err := ch2.Send(context.Background(), sampleMsg); !errors.As(err, &rl) {
		t.Fatalf("421 应为限流错误: %v", err)
	}
}

// 服务器连上了却不说话(典型:465 端口选了 STARTTLS):必须按 ctx 超时返回,不能卡住发送协程。
func TestSMTPChannelTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(io.Discard, bufio.NewReader(c)) }()
		}
	}()
	_, port := strconvPort(ln)
	ch, _ := NewChannel(ChannelConfig{Type: ChannelSMTP, Params: map[string]string{
		"host": "127.0.0.1", "port": port, "security": "starttls", "from": "x@example.com", "to": "a@example.com",
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = ch.Send(ctx, sampleMsg)
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("应报超时: %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("超时没生效,等了 %s", d)
	}
}

func strconvPort(ln net.Listener) (string, string) {
	a := ln.Addr().(*net.TCPAddr)
	return a.IP.String(), strconv.Itoa(a.Port)
}

func TestSMTPChannelValidate(t *testing.T) {
	ok := map[string]string{"host": "smtp.example.com", "username": "me@example.com", "password": "x", "to": "a@example.com"}
	cases := []struct {
		name  string
		patch map[string]string
		want  string
	}{
		{"缺主机", map[string]string{"host": ""}, "服务器地址"},
		{"主机带协议", map[string]string{"host": "smtp://smtp.example.com"}, "只填主机名"},
		{"主机带端口", map[string]string{"host": "smtp.example.com:465"}, "端口单独填"},
		{"端口越界", map[string]string{"port": "70000"}, "端口"},
		{"加密方式未知", map[string]string{"security": "ssl3"}, "加密方式"},
		{"明文发密码到外网", map[string]string{"security": "none"}, "不加密"},
		{"有用户名没密码", map[string]string{"password": ""}, "授权码"},
		{"缺收件人", map[string]string{"to": " ; "}, "缺少收件人"},
		{"收件人格式", map[string]string{"to": "a@example.com, not-an-email"}, "not-an-email"},
		{"发件人格式", map[string]string{"from": "bad"}, "发件人"},
		{"无登录也无发件人", map[string]string{"username": "", "password": ""}, "发件人"},
	}
	for _, c := range cases {
		p := map[string]string{}
		for k, v := range ok {
			p[k] = v
		}
		for k, v := range c.patch {
			p[k] = v
		}
		err := ValidateChannel(ChannelConfig{Type: ChannelSMTP, Enabled: true, Params: p})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: 期望含 %q,得到 %v", c.name, c.want, err)
		}
	}
	if err := ValidateChannel(ChannelConfig{Type: ChannelSMTP, Enabled: true, Params: ok}); err != nil {
		t.Fatalf("合法配置被拒: %v", err)
	}
	// 默认端口跟加密方式走
	for sec, want := range map[string]int{"": 587, "starttls": 587, "tls": 465, "none": 25} {
		p := map[string]string{"host": "127.0.0.1", "security": sec, "from": "x@example.com", "to": "a@example.com"}
		ch, err := newSMTPChannel(p)
		if err != nil || ch.(*smtpChannel).port != want {
			t.Errorf("security=%q 默认端口应为 %d: %v", sec, want, err)
		}
	}
	// 本机中继允许不加密登录
	if _, err := newSMTPChannel(map[string]string{"host": "localhost", "security": "none", "username": "u", "password": "p", "from": "x@example.com", "to": "a@example.com"}); err != nil {
		t.Fatalf("本机明文登录应允许: %v", err)
	}
}

func TestSMTPPasswordMaskAndMerge(t *testing.T) {
	saved := map[string]string{"host": "smtp.qq.com", "username": "me@qq.com", "password": "abcdefghijkl"}
	masked := MaskParams(ChannelSMTP, saved)
	if masked["password"] != passwordMask || masked["host"] != "smtp.qq.com" || masked["username"] != "me@qq.com" {
		t.Fatalf("掩码不对: %+v", masked)
	}
	// 回传掩码、留空 → 沿用;填了新值 → 替换(不去空白)
	for _, v := range []string{passwordMask, ""} {
		if m := MergeParams(ChannelSMTP, map[string]string{"host": " smtp.qq.com ", "password": v}, saved); m["password"] != saved["password"] || m["host"] != "smtp.qq.com" {
			t.Fatalf("password=%q 应沿用旧值: %+v", v, m)
		}
	}
	if m := MergeParams(ChannelSMTP, map[string]string{"password": " new pass "}, saved); m["password"] != " new pass " {
		t.Fatalf("新密码不对: %+v", m)
	}
	// 新建时(没有旧值)留空就是没有
	if m := MergeParams(ChannelSMTP, map[string]string{"password": ""}, nil); m["password"] != "" {
		t.Fatalf("新建留空不该有密码: %+v", m)
	}
	if MaskParams(ChannelSMTP, map[string]string{"password": ""})["password"] != "" {
		t.Fatal("没设密码不该显示掩码")
	}
}

// 订阅到期提醒(expiry 分组)经真实分发队列投到邮件渠道。
func TestSMTPChannelReceivesExpiryNotice(t *testing.T) {
	f := &fakeSMTP{}
	host, port := startFakeSMTP(t, f, false)
	n := New(Config{Enabled: true, NotifyExpiry: true})
	defer n.Close()
	n.SetChannels([]ChannelConfig{{ID: 1, Type: ChannelSMTP, Name: "邮件", Enabled: true, Events: []string{"expiry"},
		Params: map[string]string{"host": host, "port": port, "security": "none", "from": "x@example.com", "to": "a@example.com"}}})
	if ok, reason := n.CheckEnabled(EventExpiry); !ok {
		t.Fatalf("只配了邮件渠道时到期提醒应可发: %s", reason)
	}
	err := n.Send(context.Background(), Event{
		Type: EventExpiry, Title: "⏰ 订阅即将到期",
		Message: "用户: `alice\\_01`\n到期日: 2026-11-05\n剩余: 7 天",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, subject, body := parsedMail(t, f.data)
	if subject != "⏰ 订阅即将到期" || !strings.Contains(body, "用户: alice_01") || !strings.Contains(body, "剩余: 7 天") {
		t.Fatalf("到期提醒邮件不对: %q %q", subject, body)
	}
}
