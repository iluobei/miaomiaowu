package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 面板壁纸上传:把图片落到 data/wallpapers/,回一个可直接引用的公开路径。
//
// 为什么落盘而不是存 data URI:壁纸值会注入首屏 HTML,一张内联 base64 图会把每次首屏撑大几 MB
// —— sanitizeWallpaperURLForCSS 因此直接拒 data:。为什么公开可读:登录页也在同一个玻璃壳里,
// 背景要在登录前就正确;而且它是被 CSS background-image 拉的。/wallpapers/ 不以 /api/ 开头。

var errNoDataDir = errors.New("数据目录未配置,无法保存壁纸")

const (
	wallpaperDirName = "wallpapers"
	// 8MiB:够放一张 4K 壁纸,又不至于让人把原图直接怼上来。
	wallpaperMaxBytes = 8 << 20
	// WallpaperURLPrefix 存进设置里的路径前缀。sanitizeWallpaperURLForCSS 放行 "/" 开头的相对路径。
	WallpaperURLPrefix = "/wallpapers/"
)

// wallpaperExtByType 认哪些图片。按魔数判(http.DetectContentType),不信客户端给的 Content-Type/扩展名。
var wallpaperExtByType = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// wallpaperNameRe 文件名一律由服务端生成:32 位十六进制 + 已知扩展名。路径穿越因此不可能发生。
var wallpaperNameRe = regexp.MustCompile(`^[0-9a-f]{32}\.(jpg|png|webp|gif)$`)

// SetDataDir 注入数据目录(壁纸落盘用)。main.go 启动时调一次。
func (h *SystemSettingsHandler) SetDataDir(dir string) { h.dataDir = dir }

func (h *SystemSettingsHandler) wallpaperDir() string {
	return filepath.Join(h.dataDir, wallpaperDirName)
}

// UploadPanelWallpaper POST /api/admin/system-settings/panel-wallpaper/upload
// 上传即生效:写完盘就把 panel_wallpaper 指过去并触发首屏同步。
func (h *SystemSettingsHandler) UploadPanelWallpaper(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	if h.dataDir == "" {
		writeError(w, http.StatusInternalServerError, errNoDataDir)
		return
	}
	// 双保险:MaxBytesReader 掐住整个请求体,ParseMultipartForm 限制内存驻留量。
	r.Body = http.MaxBytesReader(w, r.Body, wallpaperMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeBadRequest(w, "图片过大或表单格式错误(上限 8MB)")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeBadRequest(w, "缺少文件字段 file")
		return
	}
	defer file.Close()

	// 先读头部嗅探类型,再把整个文件读出来 —— 嗅探消费掉的 512 字节用 MultiReader 接回去。
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		writeBadRequest(w, "读取文件失败")
		return
	}
	head = head[:n]
	ext, ok := wallpaperExtByType[strings.Split(http.DetectContentType(head), ";")[0]]
	if !ok {
		writeBadRequest(w, "只支持 JPG / PNG / WebP / GIF 图片")
		return
	}

	if err := os.MkdirAll(h.wallpaperDir(), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	name := hex.EncodeToString(raw) + ext
	dst, err := os.Create(filepath.Join(h.wallpaperDir(), name))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, err = io.Copy(dst, io.LimitReader(io.MultiReader(strings.NewReader(string(head)), file), wallpaperMaxBytes))
	closeErr := dst.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(filepath.Join(h.wallpaperDir(), name))
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	ctx := r.Context()
	// 换图时把上一张删掉。只删本目录里自己生成的那种名字,正则不匹配的一律不碰(比如管理员手填的外链)。
	if old, _ := h.repo.GetSystemSetting(ctx, PanelWallpaperKey); old != "" {
		if prev := strings.TrimPrefix(strings.TrimSpace(old), WallpaperURLPrefix); prev != old && wallpaperNameRe.MatchString(prev) {
			_ = os.Remove(filepath.Join(h.wallpaperDir(), prev))
		}
	}

	url := WallpaperURLPrefix + name
	if err := h.repo.SetSystemSetting(ctx, PanelWallpaperKey, url); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if h.onPanelWallpaperChanged != nil {
		h.onPanelWallpaperChanged(LoadPanelWallpaperConfig(ctx, h.repo))
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"panel_wallpaper": url,
		"message":         "壁纸已上传并应用",
	})
}

// ServeWallpaperFile GET /wallpapers/{name} —— 免鉴权。名字必须完全匹配服务端生成的形态,不匹配 404。
func (h *SystemSettingsHandler) ServeWallpaperFile(w http.ResponseWriter, r *http.Request) {
	if h.dataDir == "" {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, WallpaperURLPrefix)
	if !wallpaperNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	// 文件名带内容随机数,同名即同图 —— 可以放心长缓存。
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(h.wallpaperDir(), name))
}
