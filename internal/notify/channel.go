package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Channel 一个可发送的推送渠道。实现必须是无状态的(串行、限流、退避都在 dispatch 里做)。
type Channel interface {
	Send(ctx context.Context, m Msg) error
}

// 非 Telegram 渠道(飞书 / 钉钉 / Server酱 / Bark / 邮件)都在国内可直连。
var webhookHTTPClient = &http.Client{Timeout: 10 * time.Second}

// channelSpec 每种渠道的参数说明:必填项与需要掩码的密钥项。
// passwords 是密码类密钥:读出时整段掩码(不留末 4 位)、保存时留空 = 沿用旧值、不去首尾空白。
type channelSpec struct {
	required  []string
	secrets   []string
	passwords []string
}

var channelSpecs = map[ChannelType]channelSpec{
	ChannelTelegram:   {required: []string{"bot_token", "chat_id"}, secrets: []string{"bot_token"}},
	ChannelServerChan: {required: []string{"sendkey"}, secrets: []string{"sendkey"}},
	ChannelFeishu:     {required: []string{"webhook"}, secrets: []string{"webhook", "secret"}},
	ChannelDingTalk:   {required: []string{"webhook"}, secrets: []string{"webhook", "secret"}},
	ChannelBark:       {required: []string{"device_key"}, secrets: []string{"device_key"}},
	ChannelSMTP:       {passwords: []string{"password"}}, // 必填项由 newSMTPChannel 给出更具体的提示
}

// ValidChannelType 是否是认识的渠道类型。
func ValidChannelType(t ChannelType) bool {
	_, ok := channelSpecs[t]
	return ok
}

// SecretParam 该参数是否是密钥(读出时要掩码)。
func SecretParam(t ChannelType, key string) bool {
	for _, k := range channelSpecs[t].secrets {
		if k == key {
			return true
		}
	}
	return PasswordParam(t, key)
}

// PasswordParam 该参数是否是密码类密钥(见 channelSpec.passwords)。
func PasswordParam(t ChannelType, key string) bool {
	for _, k := range channelSpecs[t].passwords {
		if k == key {
			return true
		}
	}
	return false
}

// passwordMask 密码类密钥的掩码,一个字符都不露。
const passwordMask = "********"

// MaskSecret 密钥掩码:只留末 4 位。URL(webhook)保留协议与主机,方便认出是哪家的地址。
func MaskSecret(v string) string {
	if v == "" {
		return ""
	}
	tail := ""
	if r := []rune(v); len(r) > 8 {
		tail = string(r[len(r)-4:])
	}
	if u, err := url.Parse(v); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host + "/****" + tail
	}
	return "****" + tail
}

// IsMasked 前端提交回来的值是不是掩码(没改动)。
func IsMasked(v string) bool { return strings.Contains(v, "****") }

// MaskParams 返回密钥项已掩码的参数副本。
func MaskParams(t ChannelType, params map[string]string) map[string]string {
	out := make(map[string]string, len(params))
	for k, v := range params {
		if PasswordParam(t, k) {
			if v != "" {
				v = passwordMask
			}
		} else if SecretParam(t, k) {
			v = MaskSecret(v)
		}
		out[k] = v
	}
	return out
}

// MergeParams 保存时合并:提交的密钥项是掩码 → 沿用已保存的值;其余以提交为准(去首尾空白)。
// 只收该渠道认识的参数之外也照收(Bark 的 group / level 等可选项),但丢掉空值。
// 密码类密钥留空也沿用已保存的值(编辑表单里密码框是空的),且不去空白(密码可以带空格)。
func MergeParams(t ChannelType, submitted, saved map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range submitted {
		if PasswordParam(t, k) {
			if v == "" || IsMasked(v) {
				v = saved[k]
			}
		} else {
			v = strings.TrimSpace(v)
		}
		if SecretParam(t, k) && IsMasked(v) {
			v = saved[k]
		}
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// ValidateChannel 校验渠道配置能不能用。
func ValidateChannel(c ChannelConfig) error {
	spec, ok := channelSpecs[c.Type]
	if !ok {
		return fmt.Errorf("不支持的推送渠道类型 %q", c.Type)
	}
	for _, k := range spec.required {
		if strings.TrimSpace(c.Params[k]) == "" {
			return fmt.Errorf("缺少参数 %s", k)
		}
	}
	_, err := NewChannel(c)
	return err
}

// NewChannel 按类型构造渠道。
func NewChannel(c ChannelConfig) (Channel, error) {
	p := c.Params
	switch c.Type {
	case ChannelTelegram:
		if p["bot_token"] == "" || p["chat_id"] == "" {
			return nil, errors.New("bot token or chat ID is empty")
		}
		return &telegramChannel{token: p["bot_token"], chatID: p["chat_id"]}, nil
	case ChannelServerChan:
		return newServerChanChannel(p["sendkey"])
	case ChannelFeishu:
		return newFeishuChannel(p["webhook"], p["secret"])
	case ChannelDingTalk:
		return newDingTalkChannel(p["webhook"], p["secret"])
	case ChannelBark:
		return newBarkChannel(p["server"], p["device_key"], p["group"], p["level"])
	case ChannelSMTP:
		return newSMTPChannel(p)
	}
	return nil, fmt.Errorf("不支持的推送渠道类型 %q", c.Type)
}

// rateLimitError 服务商明确表示「发太快 / 超额」。dispatch 据此暂停该渠道一段时间。
type rateLimitError struct {
	retryAfter time.Duration
	err        error
}

func (e *rateLimitError) Error() string { return e.err.Error() }
func (e *rateLimitError) Unwrap() error { return e.err }

// postJSON 发 JSON 并读回响应。错误里不带请求 URL —— webhook 地址本身就是密钥。
func postJSON(ctx context.Context, endpoint string, payload any) (int, http.Header, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, nil, redactURLError(err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := webhookHTTPClient.Do(req)
	if err != nil {
		return 0, nil, nil, redactURLError(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, resp.Header, body, nil
}

// redactURLError 把 *url.Error 里的完整 URL 换成「协议://主机」。webhook 地址(飞书 / 钉钉的 token、
// Server酱的 SendKey、Bark 的 device key)都在 URL 里,原样进日志就等于泄漏。非 *url.Error 原样返回。
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		host := "unknown"
		scheme := "https"
		if u, perr := url.Parse(ue.URL); perr == nil && u.Host != "" {
			host, scheme = u.Host, u.Scheme
		}
		return fmt.Errorf("%s %s://%s: %w", ue.Op, scheme, host, ue.Err)
	}
	return err
}

// retryAfterOf 读 Retry-After 头(秒),读不到用 fallback。
func retryAfterOf(h http.Header, fallback time.Duration) time.Duration {
	if h != nil {
		if s, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After"))); err == nil && s > 0 {
			return time.Duration(s) * time.Second
		}
	}
	return fallback
}

func shortBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
