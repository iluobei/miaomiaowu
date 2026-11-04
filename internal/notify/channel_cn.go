package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ===== Server酱 =====
//
// Turbo 版:POST https://sctapi.ftqq.com/{SendKey}.send
// Server酱³:SendKey 形如 sctp{uid}t…,POST https://{uid}.push.ft07.com/send/{SendKey}.send
// (与官方 serverchan-sdk 的判定一致)。两者都收 JSON {title, desp},desp 为 Markdown。
// 免费版 Turbo 每天只有 5 条,超额后服务端拒收 —— 前端会提示只订阅关键事件。

var serverChan3Key = regexp.MustCompile(`^sctp(\d+)t`)

// serverChanEndpoint 测试里替换成 httptest 地址。
var serverChanEndpoint = func(key string) string {
	if m := serverChan3Key.FindStringSubmatch(key); m != nil {
		return "https://" + m[1] + ".push.ft07.com/send/" + url.PathEscape(key) + ".send"
	}
	return "https://sctapi.ftqq.com/" + url.PathEscape(key) + ".send"
}

type serverChanChannel struct{ key string }

func newServerChanChannel(key string) (Channel, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("缺少 SendKey")
	}
	if strings.ContainsAny(key, "/?#: ") {
		return nil, errors.New("SendKey 格式不对(只填 SendKey 本身,不要填整条链接)")
	}
	return &serverChanChannel{key: key}, nil
}

func (c *serverChanChannel) Send(ctx context.Context, m Msg) error {
	title, body := splitTitle(m)
	if strings.TrimSpace(title) == "" {
		title = "妙妙屋通知"
	}
	status, header, resp, err := postJSON(ctx, serverChanEndpoint(c.key), map[string]string{
		// 标题上限 32 字
		"title": truncateRunes(title, 32),
		"desp":  ToStandardMarkdown(body, true),
	})
	if err != nil {
		return fmt.Errorf("Server酱: %w", err)
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Info    string `json:"info"`
	}
	_ = json.Unmarshal(resp, &out)
	if status == http.StatusTooManyRequests {
		return &rateLimitError{retryAfter: retryAfterOf(header, 10*time.Minute), err: fmt.Errorf("Server酱: 发送过于频繁(%s)", shortBody(resp))}
	}
	if status != http.StatusOK || out.Code != 0 {
		msg := out.Message
		if msg == "" {
			msg = out.Info
		}
		if msg == "" {
			msg = shortBody(resp)
		}
		return fmt.Errorf("Server酱 返回错误(HTTP %d, code %d): %s", status, out.Code, msg)
	}
	return nil
}

// ===== 飞书自定义机器人 =====
//
// POST https://open.feishu.cn/open-apis/bot/v2/hook/{token}
// 开了「签名校验」时 body 带 timestamp(秒,字符串)与 sign:
// 以 timestamp+"\n"+secret 为 HMAC-SHA256 的 key、对空串做摘要,再 base64。
// 限频:单机器人 100 次/分钟、5 次/秒。

const feishuHookPrefix = "https://open.feishu.cn/open-apis/bot/v2/hook/"

type feishuChannel struct {
	webhook string
	secret  string
}

func newFeishuChannel(webhook, secret string) (Channel, error) {
	u, err := normalizeWebhook(webhook, feishuHookPrefix)
	if err != nil {
		return nil, fmt.Errorf("飞书 Webhook 地址: %w", err)
	}
	return &feishuChannel{webhook: u, secret: strings.TrimSpace(secret)}, nil
}

// feishuSign 飞书签名。
func feishuSign(timestamp int64, secret string) string {
	key := strconv.FormatInt(timestamp, 10) + "\n" + secret
	h := hmac.New(sha256.New, []byte(key))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// feishuNow 测试里固定时间。
var feishuNow = time.Now

func (c *feishuChannel) payload(m Msg) map[string]any {
	title, body := splitTitle(m)
	content := ToStandardMarkdown(body, false)
	if strings.TrimSpace(content) == "" {
		content = " "
	}
	card := map[string]any{
		"config":   map[string]any{"wide_screen_mode": true},
		"elements": []any{map[string]any{"tag": "markdown", "content": content}},
	}
	if strings.TrimSpace(title) != "" {
		card["header"] = map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": title},
			"template": feishuHeaderColor(m.Event),
		}
	}
	p := map[string]any{"msg_type": "interactive", "card": card}
	if c.secret != "" {
		ts := feishuNow().Unix()
		p["timestamp"] = strconv.FormatInt(ts, 10)
		p["sign"] = feishuSign(ts, c.secret)
	}
	return p
}

// feishuHeaderColor 告警红、恢复绿、到期橙,其余蓝。
func feishuHeaderColor(t EventType) string {
	switch t {
	case EventNodeProbeOffline, EventIPBan:
		return "red"
	case EventNodeProbeOnline:
		return "green"
	case EventExpiry:
		return "orange"
	}
	return "blue"
}

func (c *feishuChannel) Send(ctx context.Context, m Msg) error {
	status, header, resp, err := postJSON(ctx, c.webhook, c.payload(m))
	if err != nil {
		return fmt.Errorf("飞书: %w", err)
	}
	var out struct {
		Code       int    `json:"code"`
		Msg        string `json:"msg"`
		StatusCode int    `json:"StatusCode"`
	}
	_ = json.Unmarshal(resp, &out)
	// 11232 = frequency limited,9499 = too many request
	if status == http.StatusTooManyRequests || out.Code == 11232 || out.Code == 9499 {
		return &rateLimitError{retryAfter: retryAfterOf(header, 10*time.Second), err: fmt.Errorf("飞书: 发送过于频繁(%s)", out.Msg)}
	}
	if status != http.StatusOK || out.Code != 0 || out.StatusCode != 0 {
		msg := out.Msg
		if msg == "" {
			msg = shortBody(resp)
		}
		if out.Code == 19021 {
			msg += "(签名校验失败:检查「签名密钥」是否与机器人设置一致、服务器时间是否准确)"
		}
		return fmt.Errorf("飞书 返回错误(HTTP %d, code %d): %s", status, out.Code, msg)
	}
	return nil
}

// ===== 钉钉自定义机器人 =====
//
// POST https://oapi.dingtalk.com/robot/send?access_token=…
// 加签:timestamp 为毫秒,以 secret 为 key 对 timestamp+"\n"+secret 做 HmacSHA256,base64 后
// URL 编码,拼成 &timestamp=…&sign=…。限频 20 条/分钟,超了会被限流 10 分钟。

const dingTalkSendPrefix = "https://oapi.dingtalk.com/robot/send?access_token="

type dingTalkChannel struct {
	webhook string
	secret  string
}

func newDingTalkChannel(webhook, secret string) (Channel, error) {
	webhook = strings.TrimSpace(webhook)
	if webhook != "" && !strings.Contains(webhook, "://") {
		if strings.ContainsAny(webhook, "/?&=# ") {
			return nil, errors.New("钉钉 Webhook 地址格式不对")
		}
		webhook = dingTalkSendPrefix + url.QueryEscape(webhook)
	}
	u, err := url.Parse(webhook)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("钉钉 Webhook 地址格式不对")
	}
	if u.Query().Get("access_token") == "" {
		return nil, errors.New("钉钉 Webhook 地址缺少 access_token")
	}
	return &dingTalkChannel{webhook: u.String(), secret: strings.TrimSpace(secret)}, nil
}

// dingTalkSign 钉钉加签(未做 URL 编码)。
func dingTalkSign(timestampMs int64, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strconv.FormatInt(timestampMs, 10) + "\n" + secret))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

var dingTalkNow = time.Now

func (c *dingTalkChannel) endpoint() string {
	if c.secret == "" {
		return c.webhook
	}
	ts := dingTalkNow().UnixMilli()
	return c.webhook + "&timestamp=" + strconv.FormatInt(ts, 10) + "&sign=" + url.QueryEscape(dingTalkSign(ts, c.secret))
}

func (c *dingTalkChannel) payload(m Msg) map[string]any {
	title, _ := splitTitle(m)
	if strings.TrimSpace(title) == "" {
		title = "妙妙屋通知"
	}
	// 钉钉的 title 只用于会话列表预览,正文里要自己带标题
	text := m.Body
	if strings.TrimSpace(m.Title) != "" {
		text = "*" + m.Title + "*\n" + m.Body
	}
	return map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  ToStandardMarkdown(text, true),
		},
	}
}

func (c *dingTalkChannel) Send(ctx context.Context, m Msg) error {
	status, header, resp, err := postJSON(ctx, c.endpoint(), c.payload(m))
	if err != nil {
		return fmt.Errorf("钉钉: %w", err)
	}
	var out struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	_ = json.Unmarshal(resp, &out)
	// 130101 = send too fast, exceed 20 times per minute
	if status == http.StatusTooManyRequests || out.ErrCode == 130101 {
		return &rateLimitError{retryAfter: retryAfterOf(header, time.Minute), err: fmt.Errorf("钉钉: 发送过于频繁(%s)", out.ErrMsg)}
	}
	if status != http.StatusOK || out.ErrCode != 0 {
		msg := out.ErrMsg
		if msg == "" {
			msg = shortBody(resp)
		}
		if out.ErrCode == 310000 {
			msg += "(机器人安全设置不通过:检查加签密钥,或关键词 / IP 白名单是否包含本消息)"
		}
		return fmt.Errorf("钉钉 返回错误(HTTP %d, errcode %d): %s", status, out.ErrCode, msg)
	}
	return nil
}

// ===== Bark =====
//
// POST {server}/push,JSON {device_key, title, body, group, level}。server 默认官方
// https://api.day.app,自建的填自己的地址。device_key 也接受直接粘贴 App 里复制的整条
// https://api.day.app/<key>/ 链接。

const barkDefaultServer = "https://api.day.app"

type barkChannel struct {
	server    string
	deviceKey string
	group     string
	level     string
}

var barkLevels = map[string]bool{"": true, "active": true, "timeSensitive": true, "passive": true, "critical": true}

func newBarkChannel(server, deviceKey, group, level string) (Channel, error) {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	deviceKey = strings.TrimSpace(deviceKey)
	if strings.Contains(deviceKey, "://") {
		u, err := url.Parse(deviceKey)
		if err != nil || u.Host == "" {
			return nil, errors.New("Bark Device Key 格式不对")
		}
		key := strings.Trim(u.Path, "/")
		if i := strings.IndexByte(key, '/'); i >= 0 {
			key = key[:i]
		}
		if server == "" {
			server = u.Scheme + "://" + u.Host
		}
		deviceKey = key
	}
	if deviceKey == "" || strings.ContainsAny(deviceKey, "/?# ") {
		return nil, errors.New("Bark Device Key 格式不对")
	}
	if server == "" {
		server = barkDefaultServer
	}
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("Bark 服务器地址格式不对")
	}
	if !barkLevels[level] {
		return nil, fmt.Errorf("Bark 推送级别 %q 不支持", level)
	}
	if strings.TrimSpace(group) == "" {
		group = "妙妙屋"
	}
	return &barkChannel{server: server, deviceKey: deviceKey, group: group, level: level}, nil
}

func (c *barkChannel) payload(m Msg) map[string]string {
	title, body := splitTitle(m)
	p := map[string]string{"device_key": c.deviceKey, "title": title, "body": ToPlainText(body), "group": c.group}
	if c.level != "" {
		p["level"] = c.level
	}
	if strings.TrimSpace(p["body"]) == "" {
		p["body"] = title // Bark 要求 body 非空
	}
	return p
}

func (c *barkChannel) Send(ctx context.Context, m Msg) error {
	status, header, resp, err := postJSON(ctx, c.server+"/push", c.payload(m))
	if err != nil {
		return fmt.Errorf("Bark: %w", err)
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(resp, &out)
	if status == http.StatusTooManyRequests {
		return &rateLimitError{retryAfter: retryAfterOf(header, time.Minute), err: errors.New("Bark: 发送过于频繁")}
	}
	if status != http.StatusOK || out.Code != http.StatusOK {
		msg := out.Message
		if msg == "" {
			msg = shortBody(resp)
		}
		return fmt.Errorf("Bark 返回错误(HTTP %d, code %d): %s", status, out.Code, msg)
	}
	return nil
}

// normalizeWebhook 接受整条 webhook 地址,或只填 token(拼上 prefix)。
func normalizeWebhook(raw, prefix string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("不能为空")
	}
	if !strings.Contains(raw, "://") {
		if strings.ContainsAny(raw, "/?&=# ") {
			return "", errors.New("格式不对")
		}
		return prefix + raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("格式不对")
	}
	return u.String(), nil
}
