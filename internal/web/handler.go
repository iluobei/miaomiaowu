package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

//go:embed dist/*
var embeddedFiles embed.FS

// 首屏注入占位符。液态玻璃主题的背景是视觉主体,让前端起来后再拉接口的话每次刷新都会先渲染
// 一帧默认极光再跳变;所以壁纸/色调/降透明度在服务端就替换进 index.html。
// 占位符若从未被 SetPanelAppearance 赋值,initialize 也会用默认值(空壁纸/空色调/不降透明度)替掉。
const (
	panelWallpaperPlaceholder     = "__MMW_PANEL_WALLPAPER__"
	glassTonePlaceholder          = "__MMW_GLASS_TONE__"
	reduceTransparencyPlaceholder = "__MMW_REDUCE_TRANSPARENCY__"
)

var (
	initOnce    sync.Once
	staticFS    fs.FS
	staticFiles http.Handler
	indexBytes  []byte

	apprMu              sync.RWMutex
	servedIndex         []byte // indexBytes 替换占位符后的实际下发内容
	indexMod            time.Time
	currentWallpaperCSS string
	currentGlassTone    string
	currentReduce       bool
)

// safeGlassTone 白名单化:__MMW_GLASS_TONE__ 进的是 JS 字符串字面量,只放行那几个词,
// 别的一律清空 —— 留一个能带引号的值进去就是脚本注入。
var validGlassTones = map[string]bool{"sea": true, "amber": true, "ice": true, "graphite": true, "custom": true}

func safeGlassTone(t string) string {
	if validGlassTones[t] {
		return t
	}
	return ""
}

// rebuildServedIndexLocked 必须在持有 apprMu 写锁时调用。
func rebuildServedIndexLocked() {
	s := bytes.ReplaceAll(indexBytes, []byte(panelWallpaperPlaceholder), []byte(currentWallpaperCSS))
	s = bytes.ReplaceAll(s, []byte(glassTonePlaceholder), []byte(safeGlassTone(currentGlassTone)))
	rt := "false"
	if currentReduce {
		rt = "true"
	}
	s = bytes.ReplaceAll(s, []byte(reduceTransparencyPlaceholder), []byte(rt))
	servedIndex = s
	indexMod = time.Now()
}

func initialize() {
	sub, err := fs.Sub(embeddedFiles, "dist")
	if err != nil {
		panic(err)
	}

	staticFS = sub
	staticFiles = http.FileServer(http.FS(sub))

	indexBytes, err = fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err)
	}

	apprMu.Lock()
	rebuildServedIndexLocked()
	apprMu.Unlock()
}

// SetPanelAppearance 更新首屏注入的面板外观(壁纸 CSS / 玻璃色调 / 降低透明度)。
// main.go 启动时用当前配置预热一次,之后配置变更时由 handler 回调再调。
func SetPanelAppearance(wallpaperCSS, glassTone string, reduceTransparency bool) {
	initOnce.Do(initialize)
	apprMu.Lock()
	currentWallpaperCSS = wallpaperCSS
	currentGlassTone = glassTone
	currentReduce = reduceTransparency
	rebuildServedIndexLocked()
	apprMu.Unlock()
}

// Handler returns an HTTP handler that serves the embedded frontend SPA.
func Handler() http.Handler {
	initOnce.Do(initialize)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/traffic/") {
			http.NotFound(w, r)
			return
		}

		cleaned := path.Clean(r.URL.Path)
		if cleaned == "." {
			cleaned = "/"
		}

		if cleaned == "/" {
			serveIndex(w, r)
			return
		}

		resource := strings.TrimPrefix(cleaned, "/")
		if resource == "" {
			serveIndex(w, r)
			return
		}

		if fileExists(resource) {
			staticFiles.ServeHTTP(w, r)
			return
		}

		serveIndex(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(initialize)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	apprMu.RLock()
	body := servedIndex
	mod := indexMod
	apprMu.RUnlock()

	http.ServeContent(w, r, "index.html", mod, bytes.NewReader(body))
}

func fileExists(name string) bool {
	initOnce.Do(initialize)

	info, err := fs.Stat(staticFS, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
