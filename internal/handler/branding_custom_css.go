package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// 自定义 CSS:管理员往每个页面注入一段样式,用来改配色、藏元素、换字体等。
//
// 从妙妙屋X 迁移而来,去掉了 license 门控与「按主题绑定」的各套下发;生效的那一套
// 原样注入 index.html(见 internal/web/handler.go)。

const (
	brandingCustomCSSKey = "branding_custom_css"

	// customCSSMaxBytes 限制单份 CSS 大小。这段样式**跟着每个页面一起下发**(注入
	// index.html),包括登录页,大了就是所有人每次访问都多付的流量。
	customCSSMaxBytes = 64 << 10 // 64 KiB
)

// closingStyleTag 匹配能把注入点从 <style> 里顶出去的序列。
//
// 注入点是 index.html 里的 <style> 元素,HTML 解析器遇到 `</style` 就结束该元素 ——
// 后面的内容会被当成 HTML 解析。所以这一条不是洁癖:放过去就是 CSS 里能写 <script>。
// 管理员本来就能控制面板,但「填个样式把页面结构改坏」是实打实的 bug,而且这段样式
// 连未登录的登录页都要下发。
var closingStyleTag = regexp.MustCompile(`(?i)</\s*style`)

// customCSSDisabledByEnv 是应急开关:自定义 CSS 把面板写到点不动/看不见时,
// 改设置又得先进面板,正是进不去的那一环。置 MMW_DISABLE_CUSTOM_CSS=1 重启即可
// 让注入点空着(设置内容不动,关掉开关就回来)。只认环境变量 —— 做成面板设置就还是要先能进面板。
func customCSSDisabledByEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MMW_DISABLE_CUSTOM_CSS"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// sanitizeCustomCSS 校验并规整一段自定义 CSS。返回的错误是给管理员看的中文提示。
func sanitizeCustomCSS(css string) (string, error) {
	css = strings.TrimSpace(css)
	if css == "" {
		return "", nil
	}
	if len(css) > customCSSMaxBytes {
		return "", fmt.Errorf("CSS 太长(%d 字节),上限 %d 字节", len(css), customCSSMaxBytes)
	}
	if closingStyleTag.MatchString(css) {
		return "", fmt.Errorf("CSS 里不能出现 </style —— 它会提前结束样式块,把后面的内容当 HTML 解析")
	}
	return css, nil
}

// EffectiveCustomCSS 返回真正要注入页面的 CSS。空字符串表示不注入。
//
// 读取时**再过一遍 sanitize**:直接改库绕开写入校验的路子也堵上,坏值一律当没有。
func (h *BrandingHandler) EffectiveCustomCSS(ctx context.Context) string {
	if customCSSDisabledByEnv() {
		return ""
	}
	raw, _ := h.repo.GetSystemSetting(ctx, brandingCustomCSSKey)
	css, err := sanitizeCustomCSS(raw)
	if err != nil {
		return ""
	}
	return css
}

// CustomCSS 按方法分发 /api/admin/system-settings/custom-css。
func (h *BrandingHandler) CustomCSS(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getCustomCSS(w, r)
	case http.MethodPut, http.MethodPost:
		h.setCustomCSS(w, r)
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}

func (h *BrandingHandler) getCustomCSS(w http.ResponseWriter, r *http.Request) {
	raw, _ := h.repo.GetSystemSetting(r.Context(), brandingCustomCSSKey)
	profiles := h.loadCustomCSSProfiles(r.Context())
	respondJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"custom_css":      raw,
		"profiles":        profiles.Profiles,
		"active":          profiles.Active,
		"disabled_by_env": customCSSDisabledByEnv(),
		"max_bytes":       customCSSMaxBytes,
	})
}

func (h *BrandingHandler) setCustomCSS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomCSS string             `json:"custom_css"`
		Profiles  []customCSSProfile `json:"profiles"`
		Active    string             `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式错误")
		return
	}
	// 前端提交整套多配置;只发 custom_css 时,改的是当前生效那套。
	profiles := customCSSProfiles{Active: req.Active, Profiles: req.Profiles}
	if len(req.Profiles) == 0 {
		profiles = h.loadCustomCSSProfiles(r.Context())
		profiles.Profiles[profiles.activeIndex()].CSS = req.CustomCSS
	}
	profiles, css, err := normalizeCustomCSSProfiles(profiles)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	profilesJSON, _ := json.Marshal(profiles)
	if err := h.repo.SetSystemSetting(r.Context(), brandingCustomCSSProfilesKey, string(profilesJSON)); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存失败"))
		return
	}
	if err := h.repo.SetSystemSetting(r.Context(), brandingCustomCSSKey, css); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存失败"))
		return
	}
	if h.onCustomCSSChanged != nil {
		h.onCustomCSSChanged(h.EffectiveCustomCSS(r.Context()))
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"custom_css":      css, // 前端拿它当场替换注入点
		"disabled_by_env": customCSSDisabledByEnv(),
	})
}
