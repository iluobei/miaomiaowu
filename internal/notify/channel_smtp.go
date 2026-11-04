package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"
)

// ===== 邮件(SMTP)=====
//
// 参数:host / port / security(starttls 默认、tls = SSL/TLS 直连、none)/ username / password
// / from / from_name / to(多个用逗号、分号或换行分隔)。
//   - username 为空 = 不登录(内网中继);填了 username 就必须有 password
//   - 不加密时只允许对本机(localhost / 127.0.0.1 / ::1)登录,不把密码明文发到网上
//   - from 为空时用 username(多数邮箱要求发件人与登录账号一致)
//
// 正文是纯文本(quoted-printable):通知文本是 Telegram Markdown,转成 ToPlainText。
// 超时:连接 10 秒;整个会话的读写截止取 ctx 的 deadline(dispatch 给每条 15 秒),
// ctx 取消时直接关连接,不会卡住渠道的发送协程。

const (
	smtpSecurityStartTLS = "starttls"
	smtpSecurityTLS      = "tls"
	smtpSecurityNone     = "none"

	smtpDialTimeout    = 10 * time.Second
	smtpSessionTimeout = 30 * time.Second // ctx 没带 deadline 时的兜底
	smtpMaxRecipients  = 20
)

// smtpTLSRoots 测试里换成 httptest 的自签证书;nil = 系统根证书。
var smtpTLSRoots *x509.CertPool

type smtpChannel struct {
	host     string
	port     int
	security string
	username string
	password string
	from     mail.Address
	to       []*mail.Address
}

func newSMTPChannel(p map[string]string) (Channel, error) {
	host := strings.TrimSpace(p["host"])
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if host == "" {
		return nil, errors.New("缺少 SMTP 服务器地址")
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/ \t\r\n") {
		return nil, errors.New("SMTP 服务器只填主机名,如 smtp.qq.com")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return nil, errors.New("SMTP 服务器地址里不要带端口,端口单独填")
	}

	security := strings.ToLower(strings.TrimSpace(p["security"]))
	switch security {
	case "":
		security = smtpSecurityStartTLS
	case "ssl":
		security = smtpSecurityTLS
	case smtpSecurityStartTLS, smtpSecurityTLS, smtpSecurityNone:
	default:
		return nil, fmt.Errorf("不支持的加密方式 %q", security)
	}

	port := 0
	if s := strings.TrimSpace(p["port"]); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("SMTP 端口要在 1-65535 之间")
		}
		port = n
	} else {
		port = map[string]int{smtpSecurityStartTLS: 587, smtpSecurityTLS: 465, smtpSecurityNone: 25}[security]
	}

	username := strings.TrimSpace(p["username"])
	password := p["password"]
	if username != "" && password == "" {
		return nil, errors.New("填了用户名就要填密码(多数邮箱是「授权码」,不是登录密码)")
	}
	if username != "" && security == smtpSecurityNone && !isLocalSMTPHost(host) {
		return nil, errors.New("不加密的连接不能发送密码,请选择 STARTTLS 或 SSL/TLS")
	}

	fromAddr := strings.TrimSpace(p["from"])
	if fromAddr == "" {
		fromAddr = username
	}
	if fromAddr == "" {
		return nil, errors.New("缺少发件人地址")
	}
	from, err := mail.ParseAddress(fromAddr)
	if err != nil {
		return nil, fmt.Errorf("发件人地址 %q 格式不对", fromAddr)
	}
	if name := strings.TrimSpace(p["from_name"]); name != "" {
		from.Name = name
	} else if from.Name == "" {
		from.Name = "妙妙屋"
	}

	to, err := parseRecipients(p["to"])
	if err != nil {
		return nil, err
	}
	return &smtpChannel{
		host: host, port: port, security: security,
		username: username, password: password,
		from: *from, to: to,
	}, nil
}

// parseRecipients 收件人:逗号 / 分号 / 换行分隔,可写成「名字 <a@b.com>」。
func parseRecipients(raw string) ([]*mail.Address, error) {
	raw = strings.NewReplacer(";", ",", "\n", ",", "\r", ",", "，", ",", "；", ",").Replace(raw)
	var out []*mail.Address
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		a, err := mail.ParseAddress(part)
		if err != nil {
			return nil, fmt.Errorf("收件人 %q 格式不对", part)
		}
		key := strings.ToLower(a.Address)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, errors.New("缺少收件人")
	}
	if len(out) > smtpMaxRecipients {
		return nil, fmt.Errorf("收件人最多 %d 个", smtpMaxRecipients)
	}
	return out, nil
}

func isLocalSMTPHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *smtpChannel) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: c.host, MinVersion: tls.VersionTLS12, RootCAs: smtpTLSRoots}
}

func (c *smtpChannel) Send(ctx context.Context, m Msg) error {
	raw, err := c.buildMessage(m, time.Now())
	if err != nil {
		return fmt.Errorf("邮件: %w", err)
	}
	if err := c.deliver(ctx, raw); err != nil {
		return c.wrapErr(ctx, err)
	}
	return nil
}

// smtpStageError 标出失败在哪一步,方便管理员对症排查。
type smtpStageError struct {
	stage string
	err   error
}

func (e *smtpStageError) Error() string { return e.stage + ": " + e.err.Error() }
func (e *smtpStageError) Unwrap() error { return e.err }

func smtpStage(name string, err error) error {
	if err == nil {
		return nil
	}
	return &smtpStageError{stage: name, err: err}
}

func (c *smtpChannel) deliver(ctx context.Context, raw []byte) error {
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	var conn net.Conn
	var err error
	if c.security == smtpSecurityTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: c.tlsConfig()}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return smtpStage("连接服务器", err)
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(smtpSessionTimeout)
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	cl, err := smtp.NewClient(conn, c.host)
	if err != nil {
		return smtpStage("读取服务器欢迎信息", err)
	}
	defer cl.Close()
	if err := cl.Hello("localhost"); err != nil {
		return smtpStage("EHLO", err)
	}
	if c.security == smtpSecurityStartTLS {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return smtpStage("STARTTLS", errors.New("服务器不支持 STARTTLS(465 端口一般要选 SSL/TLS)"))
		}
		if err := cl.StartTLS(c.tlsConfig()); err != nil {
			return smtpStage("STARTTLS", err)
		}
	}
	if c.username != "" {
		auth, err := c.pickAuth(cl)
		if err != nil {
			return smtpStage("登录", err)
		}
		if err := cl.Auth(auth); err != nil {
			return smtpStage("登录", err)
		}
	}
	if err := cl.Mail(c.from.Address); err != nil {
		return smtpStage("发件人 "+c.from.Address, err)
	}
	for _, a := range c.to {
		if err := cl.Rcpt(a.Address); err != nil {
			return smtpStage("收件人 "+a.Address, err)
		}
	}
	w, err := cl.Data()
	if err != nil {
		return smtpStage("DATA", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return smtpStage("写入邮件", err)
	}
	if err := w.Close(); err != nil {
		return smtpStage("提交邮件", err)
	}
	_ = cl.Quit() // 邮件已被接受,QUIT 失败无所谓
	return nil
}

// pickAuth 优先 PLAIN,不支持时用 LOGIN(部分企业邮箱只开 LOGIN)。
func (c *smtpChannel) pickAuth(cl *smtp.Client) (smtp.Auth, error) {
	ok, mechs := cl.Extension("AUTH")
	if !ok {
		return nil, errors.New("服务器没有提供登录(AUTH);如果不需要登录,请清空用户名")
	}
	has := map[string]bool{}
	for _, m := range strings.Fields(strings.ToUpper(mechs)) {
		has[m] = true
	}
	switch {
	case has["PLAIN"]:
		return smtp.PlainAuth("", c.username, c.password, c.host), nil
	case has["LOGIN"]:
		return &smtpLoginAuth{user: c.username, pass: c.password}, nil
	}
	return nil, fmt.Errorf("服务器只支持 %s 登录方式,暂不支持", mechs)
}

// smtpLoginAuth AUTH LOGIN。与 smtp.PlainAuth 一样,只在 TLS 或本机连接上发密码。
type smtpLoginAuth struct{ user, pass string }

func (a *smtpLoginAuth) Start(s *smtp.ServerInfo) (string, []byte, error) {
	if !s.TLS && !isLocalSMTPHost(s.Name) {
		return "", nil, errors.New("unencrypted connection")
	}
	return "LOGIN", nil, nil
}

func (a *smtpLoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(string(fromServer))
	switch {
	case strings.Contains(prompt, "user"):
		return []byte(a.user), nil
	case strings.Contains(prompt, "pass"):
		return []byte(a.pass), nil
	}
	return nil, fmt.Errorf("无法识别的 LOGIN 提示 %q", fromServer)
}

// wrapErr 把常见失败翻成提示;421 / 450 视为服务商限流,交给 dispatch 退避。
func (c *smtpChannel) wrapErr(ctx context.Context, err error) error {
	var tp *textproto.Error
	if errors.As(err, &tp) && (tp.Code == 421 || tp.Code == 450) {
		return &rateLimitError{retryAfter: time.Minute, err: fmt.Errorf("邮件: 服务器暂时拒收(%w)", err)}
	}
	hint := ""
	switch {
	case errors.As(err, &tp) && tp.Code == 535:
		hint = "(用户名或密码不对;QQ / 163 等邮箱要用「授权码」)"
	case errors.Is(err, os.ErrDeadlineExceeded) || ctx.Err() != nil:
		hint = "(超时:检查端口与加密方式是否匹配,465 一般是 SSL/TLS、587 是 STARTTLS;以及服务器能否连出该端口)"
	case c.security == smtpSecurityTLS && strings.Contains(err.Error(), "first record does not look like a TLS handshake"):
		hint = "(该端口不是 SSL/TLS 直连,试试 STARTTLS)"
	}
	return fmt.Errorf("邮件(%s:%d): %w%s", c.host, c.port, err, hint)
}

// buildMessage 生成 RFC 5322 邮件。头部行尾 \r\n,正文换行由 quoted-printable 写入器转成 \r\n;
// smtp 的 DATA 写入器负责点转义。
func (c *smtpChannel) buildMessage(m Msg, now time.Time) ([]byte, error) {
	title, body := splitTitle(m)
	title = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(title))
	if title == "" {
		title = "妙妙屋通知"
	}
	text := strings.TrimSpace(ToPlainText(body))
	if text == "" {
		text = title
	}

	to := make([]string, len(c.to))
	for i, a := range c.to {
		to[i] = a.String()
	}
	domain := "miaomiaowu.local"
	if at := strings.LastIndexByte(c.from.Address, '@'); at >= 0 {
		domain = c.from.Address[at+1:]
	}
	var rnd [12]byte
	_, _ = rand.Read(rnd[:])

	var buf bytes.Buffer
	header := func(k, v string) { buf.WriteString(k + ": " + v + "\r\n") }
	header("From", c.from.String())
	header("To", strings.Join(to, ", "))
	header("Subject", mime.BEncoding.Encode("UTF-8", title))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(rnd[:])+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	if m.Event != "" {
		header("X-Miaomiaowu-Event", string(m.Event))
	}
	buf.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&buf)
	if _, err := qp.Write([]byte(text)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	buf.WriteString("\r\n")
	return buf.Bytes(), nil
}
