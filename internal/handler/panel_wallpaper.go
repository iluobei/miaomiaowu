package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"miaomiaowu/internal/storage"
)

// 面板壁纸:液态玻璃主题的背景层。
//
// 为什么值要注入首屏 HTML 而不是让前端拉接口:玻璃主题的背景是视觉主体,
// React 起来后再拉一次的话,每次刷新都会先渲染成默认极光再跳变一次。见 internal/web/handler.go。
//
// 从妙妙屋X 迁移而来,去掉了"跟随登录壁纸"(miaomiaowu 无独立登录壁纸,面板壁纸经首屏注入
// 对登录页与面板一体生效)与 license 门控(miaomiaowu 无授权系统,液态玻璃免费)。

const (
	// PanelWallpaperKey 面板背景图 URL。空 = 用内置极光。
	PanelWallpaperKey = "panel_wallpaper"
	// PanelWallpaperDimKey 遮罩强度百分比(0–60)。照片五花八门,没有遮罩时文字对比度保不住。
	PanelWallpaperDimKey = "panel_wallpaper_dim"
	// PanelWallpaperToneKey 内置极光色调:sea / amber / ice / graphite / custom。
	PanelWallpaperToneKey = "panel_wallpaper_tone"
	// PanelWallpaperToneAKey / BKey 自定义色调的两个光斑颜色(#rrggbb),仅 tone=custom 时生效。
	PanelWallpaperToneAKey = "panel_wallpaper_tone_a"
	PanelWallpaperToneBKey = "panel_wallpaper_tone_b"
	// ReduceTransparencyKey 降低透明度:玻璃退化为实色着色面,不做 blur(可及性 + 性能)。
	ReduceTransparencyKey = "reduce_transparency"

	panelWallpaperDimDefault = 22
	panelWallpaperDimMax     = 60
	panelWallpaperToneFallbk = "sea"
	// 自定义色调没设时的默认两色,取内置 sea 的那对(天青 + 琥珀)。
	panelWallpaperToneADefault = "#4cb8f5"
	panelWallpaperToneBDefault = "#f0a35b"
)

// panelWallpaperTones 是内置极光的可选色调。custom 不是一档配色,而是"用下面两个颜色键"的开关。
var panelWallpaperTones = map[string]bool{
	"sea": true, "amber": true, "ice": true, "graphite": true, "custom": true,
}

// SystemSettingsHandler 承载面板外观相关端点(目前只有面板壁纸)。
type SystemSettingsHandler struct {
	repo                    *storage.TrafficRepository
	dataDir                 string
	onPanelWallpaperChanged func(PanelWallpaperConfig)
}

// NewSystemSettingsHandler 构造。dataDir 与变更回调分别用 SetDataDir / SetOnPanelWallpaperChanged 注入。
func NewSystemSettingsHandler(repo *storage.TrafficRepository) *SystemSettingsHandler {
	return &SystemSettingsHandler{repo: repo}
}

// SetOnPanelWallpaperChanged 注入配置变更回调(用于同步首屏注入)。
func (h *SystemSettingsHandler) SetOnPanelWallpaperChanged(fn func(PanelWallpaperConfig)) {
	h.onPanelWallpaperChanged = fn
}

// ServeHTTP 路由 /api/admin/system-settings/panel-wallpaper[/upload]。
func (h *SystemSettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/upload"):
		h.UploadPanelWallpaper(w, r)
	case r.Method == http.MethodGet:
		h.GetPanelWallpaper(w, r)
	case r.Method == http.MethodPut:
		h.SetPanelWallpaper(w, r)
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}

// PanelWallpaperConfig 是一次性读出的完整面板外观配置。
type PanelWallpaperConfig struct {
	Wallpaper          string `json:"panel_wallpaper"`
	Dim                int    `json:"panel_wallpaper_dim"`
	Tone               string `json:"panel_wallpaper_tone"`
	ToneA              string `json:"panel_wallpaper_tone_a"`
	ToneB              string `json:"panel_wallpaper_tone_b"`
	ReduceTransparency bool   `json:"reduce_transparency"`
}

// normalizeHexColor 规整成 #rrggbb,不合法返回空。走白名单式校验(只认 6 位十六进制)而不是转义:
// 这个值最终会进注入首屏的 <style>,合法颜色里本来就不该有别的字符。
func normalizeHexColor(raw string) string {
	v := strings.TrimSpace(raw)
	v = strings.TrimPrefix(v, "#")
	if len(v) != 6 {
		return ""
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("#%06x", n)
}

// hexToRGBTriplet 把 #rrggbb 转成 CSS 变量要的 "r, g, b"(极光层用 rgba(var(--g-tone-a), <alpha>))。
func hexToRGBTriplet(hex string) string {
	v := normalizeHexColor(hex)
	if v == "" {
		return ""
	}
	n, _ := strconv.ParseUint(v[1:], 16, 32)
	return fmt.Sprintf("%d, %d, %d", n>>16&0xff, n>>8&0xff, n&0xff)
}

func normalizePanelWallpaperToneColor(raw, fallback string) string {
	if v := normalizeHexColor(raw); v != "" {
		return v
	}
	return fallback
}

func normalizePanelWallpaperDim(raw string) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return panelWallpaperDimDefault
	}
	if v < 0 {
		return 0
	}
	if v > panelWallpaperDimMax {
		return panelWallpaperDimMax
	}
	return v
}

func normalizePanelWallpaperTone(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if panelWallpaperTones[t] {
		return t
	}
	return panelWallpaperToneFallbk
}

// LoadPanelWallpaperConfig 读出完整配置,已规整。
func LoadPanelWallpaperConfig(ctx context.Context, repo *storage.TrafficRepository) PanelWallpaperConfig {
	get := func(k string) string { v, _ := repo.GetSystemSetting(ctx, k); return strings.TrimSpace(v) }
	return PanelWallpaperConfig{
		Wallpaper:          get(PanelWallpaperKey),
		Dim:                normalizePanelWallpaperDim(get(PanelWallpaperDimKey)),
		Tone:               normalizePanelWallpaperTone(get(PanelWallpaperToneKey)),
		ToneA:              normalizePanelWallpaperToneColor(get(PanelWallpaperToneAKey), panelWallpaperToneADefault),
		ToneB:              normalizePanelWallpaperToneColor(get(PanelWallpaperToneBKey), panelWallpaperToneBDefault),
		ReduceTransparency: get(ReduceTransparencyKey) == "1",
	}
}

// GetPanelWallpaper GET /api/admin/system-settings/panel-wallpaper
func (h *SystemSettingsHandler) GetPanelWallpaper(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"config":  LoadPanelWallpaperConfig(r.Context(), h.repo),
		"tones":   []string{"sea", "amber", "ice", "graphite", "custom"},
	})
}

// SetPanelWallpaper PUT /api/admin/system-settings/panel-wallpaper
func (h *SystemSettingsHandler) SetPanelWallpaper(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Wallpaper          *string `json:"panel_wallpaper"`
		Dim                *int    `json:"panel_wallpaper_dim"`
		Tone               *string `json:"panel_wallpaper_tone"`
		ToneA              *string `json:"panel_wallpaper_tone_a"`
		ToneB              *string `json:"panel_wallpaper_tone_b"`
		ReduceTransparency *bool   `json:"reduce_transparency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式错误")
		return
	}
	ctx := r.Context()
	if req.Wallpaper != nil {
		v := strings.TrimSpace(*req.Wallpaper)
		if len(v) > 2000 {
			writeBadRequest(w, "URL 过长")
			return
		}
		if err := h.repo.SetSystemSetting(ctx, PanelWallpaperKey, v); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.Dim != nil {
		v := normalizePanelWallpaperDim(strconv.Itoa(*req.Dim))
		if err := h.repo.SetSystemSetting(ctx, PanelWallpaperDimKey, strconv.Itoa(v)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.Tone != nil {
		if err := h.repo.SetSystemSetting(ctx, PanelWallpaperToneKey, normalizePanelWallpaperTone(*req.Tone)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	for _, c := range []struct {
		key string
		val *string
	}{
		{PanelWallpaperToneAKey, req.ToneA},
		{PanelWallpaperToneBKey, req.ToneB},
	} {
		if c.val == nil {
			continue
		}
		v := normalizeHexColor(*c.val)
		if v == "" {
			writeBadRequest(w, "颜色格式错误,应为 #RRGGBB")
			return
		}
		if err := h.repo.SetSystemSetting(ctx, c.key, v); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.ReduceTransparency != nil {
		if err := h.repo.SetSystemSetting(ctx, ReduceTransparencyKey, boolSetting(*req.ReduceTransparency)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if h.onPanelWallpaperChanged != nil {
		h.onPanelWallpaperChanged(LoadPanelWallpaperConfig(ctx, h.repo))
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "message": "面板外观已更新"})
}

func boolSetting(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// GetPanelWallpaperPublic GET /api/public/panel-wallpaper —— 免鉴权。
// 玻璃主题的背景在用户登录之前就要正确(登录页也在同一个壳里)。只暴露外观参数,不含敏感信息。
func (h *SystemSettingsHandler) GetPanelWallpaperPublic(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, LoadPanelWallpaperConfig(r.Context(), h.repo))
}

// PanelWallpaperCSS 把配置渲染成一段注入首屏 <style> 的 CSS 变量声明。
// 只发变量、不发规则:规则都在 glass-theme.css 里。注入失败也只回落到内置极光,不会整个坏掉。
func PanelWallpaperCSS(cfg PanelWallpaperConfig) string {
	var b strings.Builder
	b.WriteString(":root{")
	b.WriteString("--g-dim:")
	b.WriteString(strconv.FormatFloat(float64(cfg.Dim)/100, 'f', 3, 64))
	b.WriteString(";")
	if url := sanitizeWallpaperURLForCSS(cfg.Wallpaper); url != "" {
		b.WriteString("--g-wallpaper:url(\"")
		b.WriteString(url)
		b.WriteString("\");")
	}
	b.WriteString("}")
	// 自定义色调:预设靠 [data-glass-tone=xxx] 选中,custom 没有对应规则,颜色得在这里直接发。
	// 必须带上同样的属性选择器,裸 :root{} 压不过 :root.theme-glass。
	if cfg.Tone == "custom" {
		a := hexToRGBTriplet(cfg.ToneA)
		bb := hexToRGBTriplet(cfg.ToneB)
		if a != "" && bb != "" {
			b.WriteString(":root.theme-glass[data-glass-tone='custom']{--g-tone-a:")
			b.WriteString(a)
			b.WriteString(";--g-tone-b:")
			b.WriteString(bb)
			b.WriteString(";}")
		}
	}
	return b.String()
}

// sanitizeWallpaperURLForCSS 校验并放行要塞进 url() 的地址(注入点)。
// 只放行相对路径与 http(s),把能破坏 CSS 语法的字符一律拒掉;data: 一律拒(会把首屏撑大几 MB)。
func sanitizeWallpaperURLForCSS(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > 2000 {
		return ""
	}
	if strings.ContainsAny(v, "\"'()\\\n\r\t;{}<>") {
		return ""
	}
	lower := strings.ToLower(v)
	if strings.HasPrefix(v, "/") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return v
	}
	return ""
}
