package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/storage"
)

// 自定义品牌:站点标题 / 左上角标题 / logo / 浏览器图标。
//
// 从妙妙屋X 迁移而来,去掉了 license 门控(miaomiaowu 无授权系统,对所有管理员直接可用)、
// 站点描述与 PWA 安装图标。上传的 logo / favicon 与面板壁纸一样落到数据目录(data/branding/),
// 经 /api/branding/logo、/api/branding/icon 公开可读 —— 登录页也要显示品牌。
//
// 标题 / favicon / 左上角品牌都注入首屏 HTML(见 internal/web/handler.go):React 起来后再拉
// 接口的话,每次刷新都会先画出内置的「妙妙屋」再跳变一次。

const (
	brandingSiteTitleKey  = "branding_site_title"  // 浏览器标签页标题
	brandingBrandTitleKey = "branding_brand_title" // 左上角标题文字
	brandingLogoURLKey    = "branding_logo_url"    // logo:外部 URL 或内部 /api/branding/logo?v=<ts>
	brandingLogoExtKey    = "branding_logo_ext"    // 上传 logo 的扩展名(serve 时据此定 content-type)
	brandingIconURLKey    = "branding_icon_url"    // favicon:外部 URL 或内部 /api/branding/icon?v=<ts>
	brandingIconExtKey    = "branding_icon_ext"    // 上传 favicon 的扩展名

	brandingDirName     = "branding"
	brandingLogoMaxSize = 2 << 20 // 2MB
	brandingURLMaxLen   = 2000
)

var errBrandingNoDataDir = errors.New("数据目录未配置,无法保存图片")

// 允许的 logo 扩展名 → content-type
var brandingLogoTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".ico":  "image/x-icon",
}

// BrandingHandler 提供自定义品牌的读写、logo / favicon 上传与服务,以及自定义 CSS。
type BrandingHandler struct {
	repo               *storage.TrafficRepository
	dataDir            string
	onSiteTitleChanged func(string)
	onSiteIconChanged  func(string)
	onBrandMarkChanged func(title, logoURL string)
	onCustomCSSChanged func(string)
}

func NewBrandingHandler(repo *storage.TrafficRepository) *BrandingHandler {
	return &BrandingHandler{repo: repo}
}

// SetDataDir 注入数据目录(logo / favicon 落盘用)。main.go 启动时调一次。
func (h *BrandingHandler) SetDataDir(dir string) { h.dataDir = dir }

func (h *BrandingHandler) brandingDir() string {
	return filepath.Join(h.dataDir, brandingDirName)
}

// SetOnSiteTitleChanged 注册运行时标题同步回调。用于让静态 HTML 首屏和公开品牌接口保持一致。
func (h *BrandingHandler) SetOnSiteTitleChanged(fn func(string)) {
	h.onSiteTitleChanged = fn
}

// SetOnSiteIconChanged 注册运行时 favicon 同步回调。
func (h *BrandingHandler) SetOnSiteIconChanged(fn func(string)) {
	h.onSiteIconChanged = fn
}

// SetOnBrandMarkChanged 注册左上角品牌标题/logo 的首屏同步回调。
// 与 site title/icon 分开:那对是浏览器标签页,这对是页面左上角。
func (h *BrandingHandler) SetOnBrandMarkChanged(fn func(title, logoURL string)) {
	h.onBrandMarkChanged = fn
}

// SetOnCustomCSSChanged 注册运行时自定义 CSS 同步回调 —— 保存后要立刻反映到首屏注入,
// 否则新样式得等进程重启才生效。
func (h *BrandingHandler) SetOnCustomCSSChanged(fn func(string)) {
	h.onCustomCSSChanged = fn
}

type brandingConfig struct {
	SiteTitle  string `json:"site_title"`
	BrandTitle string `json:"brand_title"`
	LogoURL    string `json:"logo_url"`
	IconURL    string `json:"icon_url"`
}

func (h *BrandingHandler) load(ctx context.Context) brandingConfig {
	get := func(k string) string { v, _ := h.repo.GetSystemSetting(ctx, k); return strings.TrimSpace(v) }
	return brandingConfig{
		SiteTitle:  get(brandingSiteTitleKey),
		BrandTitle: get(brandingBrandTitleKey),
		LogoURL:    get(brandingLogoURLKey),
		IconURL:    get(brandingIconURLKey),
	}
}

// SyncInjected 把当前品牌与自定义 CSS 全部推给首屏注入回调。启动预热与保存后都走它。
func (h *BrandingHandler) SyncInjected(ctx context.Context) {
	cfg := h.load(ctx)
	if h.onSiteTitleChanged != nil {
		h.onSiteTitleChanged(cfg.SiteTitle)
	}
	if h.onSiteIconChanged != nil {
		h.onSiteIconChanged(cfg.IconURL)
	}
	if h.onBrandMarkChanged != nil {
		h.onBrandMarkChanged(cfg.BrandTitle, cfg.LogoURL)
	}
	if h.onCustomCSSChanged != nil {
		h.onCustomCSSChanged(h.EffectiveCustomCSS(ctx))
	}
}

// Admin 按方法分发 /api/admin/system-settings/branding:GET 读、POST 写。
func (h *BrandingHandler) Admin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		respondJSON(w, http.StatusOK, map[string]any{"success": true, "branding": h.load(r.Context())})
	case http.MethodPost:
		h.adminSet(w, r)
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}

// adminSet POST /api/admin/system-settings/branding — 设置标题 / logo URL / favicon URL。
func (h *BrandingHandler) adminSet(w http.ResponseWriter, r *http.Request) {
	var req brandingConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式错误")
		return
	}
	ctx := r.Context()
	// LogoURL / IconURL:允许清空或填外部 URL(上传走单独接口,会覆盖成内部路径;此处原样保留前端传的值)。
	for _, kv := range []struct{ key, val string }{
		{brandingSiteTitleKey, req.SiteTitle},
		{brandingBrandTitleKey, req.BrandTitle},
		{brandingLogoURLKey, req.LogoURL},
		{brandingIconURLKey, req.IconURL},
	} {
		v := strings.TrimSpace(kv.val)
		if len(v) > brandingURLMaxLen {
			writeBadRequest(w, "内容过长")
			return
		}
		if err := h.repo.SetSystemSetting(ctx, kv.key, v); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	h.SyncInjected(ctx)
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "message": "品牌设置已保存"})
}

// UploadLogo POST /api/admin/system-settings/branding/logo — 上传 logo 文件,存盘并把 logo_url 指到内部路径。
func (h *BrandingHandler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	h.uploadAsset(w, r, "logo", brandingLogoExtKey, brandingLogoURLKey, "/api/branding/logo")
}

// UploadIcon POST /api/admin/system-settings/branding/icon — 上传浏览器标签页图标。
func (h *BrandingHandler) UploadIcon(w http.ResponseWriter, r *http.Request) {
	h.uploadAsset(w, r, "icon", brandingIconExtKey, brandingIconURLKey, "/api/branding/icon")
}

func (h *BrandingHandler) uploadAsset(w http.ResponseWriter, r *http.Request, asset, extKey, urlKey, publicPath string) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	if h.dataDir == "" {
		writeError(w, http.StatusInternalServerError, errBrandingNoDataDir)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, brandingLogoMaxSize+(1<<20))
	if err := r.ParseMultipartForm(brandingLogoMaxSize + (1 << 20)); err != nil {
		writeBadRequest(w, "图片过大或表单格式错误(上限 2MB)")
		return
	}
	file, header, err := r.FormFile(asset)
	if err != nil {
		writeBadRequest(w, "缺少 "+asset+" 文件")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if _, ok := brandingLogoTypes[ext]; !ok {
		writeBadRequest(w, "只支持 png/jpg/webp/gif/svg/ico 图片")
		return
	}
	if header.Size > brandingLogoMaxSize {
		writeBadRequest(w, fmt.Sprintf("%s 文件过大,不能超过 %dMB", asset, brandingLogoMaxSize>>20))
		return
	}

	if err := os.MkdirAll(h.brandingDir(), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("创建目录失败: %w", err))
		return
	}
	// 清掉旧的其它扩展名文件,避免残留(serve 按当前 ext 定位)。
	for e := range brandingLogoTypes {
		_ = os.Remove(filepath.Join(h.brandingDir(), asset+e))
	}
	dstPath := filepath.Join(h.brandingDir(), asset+ext)
	dst, err := os.Create(dstPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存失败: %w", err))
		return
	}
	written, cerr := io.Copy(dst, io.LimitReader(file, brandingLogoMaxSize+1))
	_ = dst.Close()
	if cerr != nil || written > brandingLogoMaxSize {
		_ = os.Remove(dstPath)
		writeBadRequest(w, "保存失败或文件过大")
		return
	}
	ctx := r.Context()
	url := fmt.Sprintf("%s?v=%d", publicPath, time.Now().Unix()) // 时间戳 cache-bust
	if err := h.repo.SetSystemSetting(ctx, extKey, ext); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := h.repo.SetSystemSetting(ctx, urlKey, url); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 上传同样改了 URL —— 首屏注入也得跟着走一遍,否则新图要等进程重启才不闪。
	h.SyncInjected(ctx)
	respondJSON(w, http.StatusOK, map[string]any{"success": true, asset + "_url": url})
}

// ServeLogo GET /api/branding/logo — 公开服务上传的 logo 文件。
func (h *BrandingHandler) ServeLogo(w http.ResponseWriter, r *http.Request) {
	h.serveAsset(w, r, "logo", brandingLogoExtKey)
}

// ServeIcon GET /api/branding/icon — 公开服务上传的 favicon。
func (h *BrandingHandler) ServeIcon(w http.ResponseWriter, r *http.Request) {
	h.serveAsset(w, r, "icon", brandingIconExtKey)
}

func (h *BrandingHandler) serveAsset(w http.ResponseWriter, r *http.Request, asset, extKey string) {
	if h.dataDir == "" {
		http.NotFound(w, r)
		return
	}
	raw, _ := h.repo.GetSystemSetting(r.Context(), extKey)
	ext := strings.ToLower(strings.TrimSpace(raw))
	ctype, ok := brandingLogoTypes[ext]
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(h.brandingDir(), asset+ext))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = io.Copy(w, f)
}

// PublicGet GET /api/branding — 公开返回当前品牌。无 auth:登录页也要能拿到品牌。
func (h *BrandingHandler) PublicGet(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "branding": h.load(r.Context())})
}
