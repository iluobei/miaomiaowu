package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"miaomiaowu/internal/storage"
)

// </style 必须被拒:注入点是 index.html 里的 <style> 元素,放过去等于 CSS 里能写 HTML。
func TestSanitizeCustomCSSRejectsStyleBreakout(t *testing.T) {
	breakouts := []string{
		`body{} </style><script>alert(1)</script>`,
		`body{} </STYLE ><img src=x onerror=alert(1)>`,
		`body{} </ style>`,
		"body{}\n</\tstyle>",
	}
	for _, css := range breakouts {
		if _, err := sanitizeCustomCSS(css); err == nil {
			t.Errorf("应当拒绝但放行了: %q", css)
		}
	}
}

// 正常 CSS 不能被误伤 —— 尤其是 `>` 子选择器和 content 里的尖括号,
// 一旦有人改成"HTML 转义"或"过滤 <",这些会静默坏掉。
func TestSanitizeCustomCSSKeepsRealCSS(t *testing.T) {
	css := `.sidebar > .item { color: #fff }
.badge::after { content: "<PRO>" }
@media (max-width: 640px) { .topbar { display: none } }`
	got, err := sanitizeCustomCSS(css)
	if err != nil {
		t.Fatalf("正常 CSS 被拒: %v", err)
	}
	if got != css {
		t.Errorf("正常 CSS 被改写了:\n%s", got)
	}
}

func TestSanitizeCustomCSSEnforcesSizeLimit(t *testing.T) {
	if _, err := sanitizeCustomCSS(strings.Repeat("a", customCSSMaxBytes+1)); err == nil {
		t.Error("超长 CSS 应当被拒")
	}
	if _, err := sanitizeCustomCSS(strings.Repeat("a", customCSSMaxBytes)); err != nil {
		t.Errorf("正好到上限不该被拒: %v", err)
	}
}

// 应急开关只认这几个值,别的一律当没开(免得 MMW_DISABLE_CUSTOM_CSS=0 反而关掉)。
func TestCustomCSSDisabledByEnv(t *testing.T) {
	for _, on := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv("MMW_DISABLE_CUSTOM_CSS", on)
		if !customCSSDisabledByEnv() {
			t.Errorf("%q 应当关闭自定义 CSS", on)
		}
	}
	for _, off := range []string{"", "0", "false", "no", "off", "随便"} {
		t.Setenv("MMW_DISABLE_CUSTOM_CSS", off)
		if customCSSDisabledByEnv() {
			t.Errorf("%q 不该关闭自定义 CSS", off)
		}
	}
}

func newCSSHandler(t *testing.T) *BrandingHandler {
	t.Helper()
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "css.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return NewBrandingHandler(repo)
}

func putCSS(t *testing.T, h *BrandingHandler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.CustomCSS(rec, httptest.NewRequest(http.MethodPut, "/api/admin/system-settings/custom-css", bytes.NewReader(raw)))
	return rec
}

// 多套配置切换 —— 生效那套写进 branding_custom_css(注入页面的那条路不动)。
func TestCustomCSSProfilesActiveIsInjected(t *testing.T) {
	h := newCSSHandler(t)
	rec := putCSS(t, h, map[string]any{
		"active": "dark",
		"profiles": []map[string]any{
			{"id": "light", "name": "浅色", "css": "body{color:#000}"},
			{"id": "dark", "name": "深色", "css": "body{color:#fff}", "source_url": "https://example.com/dark.css"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("保存失败: %d %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	if got, _ := h.repo.GetSystemSetting(ctx, brandingCustomCSSKey); got != "body{color:#fff}" {
		t.Fatalf("生效的应是「深色」那套, got %q", got)
	}
	p := h.loadCustomCSSProfiles(ctx)
	if p.Active != "dark" || len(p.Profiles) != 2 || p.Profiles[1].SourceURL != "https://example.com/dark.css" {
		t.Fatalf("多套配置没存对: %+v", p)
	}

	// 切到「浅色」:只换 active,生效 CSS 跟着变
	putCSS(t, h, map[string]any{"active": "light", "profiles": p.Profiles})
	if got, _ := h.repo.GetSystemSetting(ctx, brandingCustomCSSKey); got != "body{color:#000}" {
		t.Fatalf("切换后生效 CSS 应是浅色, got %q", got)
	}
}

// 只发 custom_css:改的是当前生效那套,别的配置不丢。
func TestCustomCSSLegacyPutEditsActiveProfile(t *testing.T) {
	h := newCSSHandler(t)
	putCSS(t, h, map[string]any{"active": "b", "profiles": []map[string]any{
		{"id": "a", "name": "A", "css": "a{}"}, {"id": "b", "name": "B", "css": "b{}"},
	}})
	if rec := putCSS(t, h, map[string]any{"custom_css": "b{color:red}"}); rec.Code != http.StatusOK {
		t.Fatalf("老接口保存失败: %s", rec.Body.String())
	}
	p := h.loadCustomCSSProfiles(context.Background())
	if p.Profiles[0].CSS != "a{}" || p.Profiles[1].CSS != "b{color:red}" {
		t.Fatalf("老接口应只改生效那套: %+v", p.Profiles)
	}
	// 从没存过多套配置时,老数据合成一套「默认」
	h2 := newCSSHandler(t)
	_ = h2.repo.SetSystemSetting(context.Background(), brandingCustomCSSKey, "x{}")
	if p := h2.loadCustomCSSProfiles(context.Background()); len(p.Profiles) != 1 || p.Profiles[0].CSS != "x{}" {
		t.Fatalf("旧数据应合成一套默认配置: %+v", p)
	}
}

func TestCustomCSSProfilesValidation(t *testing.T) {
	h := newCSSHandler(t)
	cases := map[string]map[string]any{
		"重复 ID":     {"active": "a", "profiles": []map[string]any{{"id": "a", "css": ""}, {"id": "a", "css": ""}}},
		"生效配置不存在":   {"active": "zz", "profiles": []map[string]any{{"id": "a", "css": ""}}},
		"CSS 越狱":    {"active": "a", "profiles": []map[string]any{{"id": "a", "css": "</style><script>"}}},
		"来源不是 http": {"active": "a", "profiles": []map[string]any{{"id": "a", "css": "", "source_url": "file:///etc/passwd"}}},
	}
	for name, body := range cases {
		if rec := putCSS(t, h, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: 应拒绝, got %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestRawCSSURLConvertsGitHubPages(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/u/repo/blob/main/themes/dark.css": "https://raw.githubusercontent.com/u/repo/main/themes/dark.css",
		"https://gist.github.com/u/0123abcd":                  "https://gist.githubusercontent.com/u/0123abcd/raw",
		"https://cdn.example.com/a.css":                       "https://cdn.example.com/a.css",
	} {
		if got := rawCSSURL(in); got != want {
			t.Errorf("rawCSSURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// 拉取只回填、不保存:远端被改坏也不会直接打到所有页面上。
func TestFetchCustomCSSDoesNotSave(t *testing.T) {
	h := newCSSHandler(t)
	prev := fetchCustomCSS
	t.Cleanup(func() { fetchCustomCSS = prev })
	fetchCustomCSS = func(_ context.Context, u string) (string, error) {
		if strings.Contains(u, "bad") {
			return "", errors.New("下载失败")
		}
		return "body{background:#123}", nil
	}
	do := func(url string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]string{"url": url})
		rec := httptest.NewRecorder()
		h.FetchCustomCSS(rec, httptest.NewRequest(http.MethodPost, "/api/admin/system-settings/custom-css/fetch", bytes.NewReader(raw)))
		return rec
	}
	rec := do("https://github.com/u/r/blob/main/a.css")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "body{background:#123}") ||
		!strings.Contains(rec.Body.String(), "raw.githubusercontent.com") {
		t.Fatalf("应返回 CSS 与换算后的原始地址: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := h.repo.GetSystemSetting(context.Background(), brandingCustomCSSKey); got != "" {
		t.Fatalf("拉取不应保存, got %q", got)
	}
	if rec := do("https://bad.example.com/a.css"); rec.Code != http.StatusBadRequest {
		t.Fatalf("下载失败应 400, got %d", rec.Code)
	}
}
