// Package captcha 封装 Cloudflare Turnstile 人机验证。配置来自 system_settings 表
// (turnstile_site_key / turnstile_secret_key),管理员后台改完即生效不需要重启。
// 两个 key 任一为空 → Enabled()=false,Verify 直接通过 — 升级用户不强制配置。
//
// 参考 mmwx-license/internal/captcha/turnstile.go(那边走 env 变量,这边走 DB)。
package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"miaomiaowu/internal/storage"
)

const (
	settingKeySiteKey   = "turnstile_site_key"
	settingKeySecretKey = "turnstile_secret_key"
	siteVerifyURL       = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

type Turnstile struct {
	repo   *storage.TrafficRepository
	client *http.Client
}

func New(repo *storage.TrafficRepository) *Turnstile {
	return &Turnstile{
		repo:   repo,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled 两 key 都非空才算启用。每次实时查 — 写完管理后台立即生效不用重启。
func (t *Turnstile) Enabled(ctx context.Context) bool {
	if t == nil || t.repo == nil {
		return false
	}
	site, _ := t.repo.GetSystemSetting(ctx, settingKeySiteKey)
	secret, _ := t.repo.GetSystemSetting(ctx, settingKeySecretKey)
	return strings.TrimSpace(site) != "" && strings.TrimSpace(secret) != ""
}

// SiteKey 给公开 /api/captcha/config 端点用,前端登录页拿来 render widget。
// 未配置返回空字符串(前端据此降级隐藏 widget)。
func (t *Turnstile) SiteKey(ctx context.Context) string {
	if t == nil || t.repo == nil {
		return ""
	}
	v, _ := t.repo.GetSystemSetting(ctx, settingKeySiteKey)
	return strings.TrimSpace(v)
}

// Verify 调 CF siteverify。Enabled 为 false 直接通过,等价"未启用就不拦"。
// token 空 + Enabled 为 true → 必拒;反之走 HTTP 验证看 success 字段。
func (t *Turnstile) Verify(ctx context.Context, token, remoteIP string) bool {
	result, _ := t.VerifyDetailed(ctx, token, remoteIP)
	return result.Success
}

// VerifyResult Cloudflare siteverify 返回的关键字段,转发给前端做诊断。
// 典型 error_codes:
//   - "missing-input-response" — 没传 token
//   - "invalid-input-response" — token 已用过 / 过期 / 错
//   - "invalid-input-secret" — secret_key 错(配错或未保存)
//   - "timeout-or-duplicate" — token 已被消费
type VerifyResult struct {
	Success     bool     `json:"success"`
	ErrorCodes  []string `json:"error_codes,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	ChallengeTS string   `json:"challenge_ts,omitempty"`
	Action      string   `json:"action,omitempty"`
	// Skipped=true 表示根本没调 CF(未启用 / token 空),由调用方区分语义
	Skipped bool `json:"skipped,omitempty"`
}

// VerifyDetailed 跟 Verify 同款流程,但返回 cloudflare 完整响应供前端做"配置测试"诊断。
// 注意 secret_key 来源仍是 DB(避免前端拿到的是 mask 字符串)。
func (t *Turnstile) VerifyDetailed(ctx context.Context, token, remoteIP string) (VerifyResult, error) {
	if !t.Enabled(ctx) {
		return VerifyResult{Success: true, Skipped: true}, nil
	}
	if strings.TrimSpace(token) == "" {
		return VerifyResult{Success: false, ErrorCodes: []string{"missing-input-response"}}, nil
	}
	secret, _ := t.repo.GetSystemSetting(ctx, settingKeySecretKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, siteVerifyURL, strings.NewReader(url.Values{
		"secret":   {secret},
		"response": {token},
		"remoteip": {remoteIP},
	}.Encode()))
	if err != nil {
		return VerifyResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.client.Do(req)
	if err != nil {
		return VerifyResult{}, err
	}
	defer resp.Body.Close()

	var cfResp struct {
		Success     bool     `json:"success"`
		ErrorCodes  []string `json:"error-codes"`
		Hostname    string   `json:"hostname"`
		ChallengeTS string   `json:"challenge_ts"`
		Action      string   `json:"action"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfResp); err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{
		Success:     cfResp.Success,
		ErrorCodes:  cfResp.ErrorCodes,
		Hostname:    cfResp.Hostname,
		ChallengeTS: cfResp.ChallengeTS,
		Action:      cfResp.Action,
	}, nil
}
