package web

import (
	"bytes"
	"embed"
	"html"
	"io/fs"
	"net/http"
	"path"
	"regexp"
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

// 自定义品牌 / 自定义 CSS 的首屏注入(从妙妙屋X 移植)。
//
// 左上角品牌标题与 logo 走 <head> 里两个 <meta> 占位符;自定义 CSS 走 <body> 开头的
// <style> 占位符(放 body 而不是 head:Vite 把应用样式表插在 </head> 之前,注入点在 head
// 里会被应用样式压过去,管理员得给每条规则都写 !important 才生效)。
//
// 标签页标题与 favicon **没有占位符**:构建期 scripts/inject-site-config.js 会按 site.json
// 把 <title> 与 <link rel="icon"> 整条重写,占位符留不住。所以这两处在运行时按标签直接替换
// —— 没设自定义值时原样保留构建期的 site.json 值。
const (
	brandTitlePlaceholder = "__MMW_BRAND_TITLE__"
	brandLogoPlaceholder  = "__MMW_BRAND_LOGO__"
	customCSSPlaceholder  = "__MMW_CUSTOM_CSS__"
)

// titleTagRe 用 [^<]* 而不是 .*?:注释里一旦出现字面量 "<title>",懒惰匹配会从注释一路吃到真正的
// </title>,把中间的标签全部吞掉。标题内容本身经过转义,不会含 "<"。
var (
	titleTagRe   = regexp.MustCompile(`<title>[^<]*</title>`)
	faviconTagRe = regexp.MustCompile(`<link\s+rel="icon"[^>]*>`)
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
	currentSiteTitle    string
	currentSiteIcon     string
	currentBrandTitle   string
	currentBrandLogo    string
	currentCustomCSS    string
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
	// 标签页标题 / favicon:只在设了自定义值时整条替换,否则保留构建期写进去的。
	if currentSiteTitle != "" {
		s = replaceFirst(s, titleTagRe, "<title>"+html.EscapeString(currentSiteTitle)+"</title>")
	}
	if currentSiteIcon != "" {
		s = replaceFirst(s, faviconTagRe, `<link rel="icon" href="`+html.EscapeString(currentSiteIcon)+`" />`)
	}
	s = bytes.ReplaceAll(s, []byte(brandTitlePlaceholder), []byte(html.EscapeString(currentBrandTitle)))
	s = bytes.ReplaceAll(s, []byte(brandLogoPlaceholder), []byte(html.EscapeString(currentBrandLogo)))
	// 自定义 CSS **不做 HTML 转义** —— 它是 <style> 元素的内容,不是 HTML 文本。
	// 转义了 `a > b`、`content: "&"` 这类正常 CSS 全会坏掉。能把注入点顶出 <style>
	// 的只有 `</style`,那一条在写入和读取两侧都被 handler.sanitizeCustomCSS 挡掉了。
	s = bytes.ReplaceAll(s, []byte(customCSSPlaceholder), []byte(currentCustomCSS))
	servedIndex = s
	indexMod = time.Now()
}

// replaceFirst 只替换第一处匹配(<title> 与 favicon 各只有一个)。
func replaceFirst(src []byte, re *regexp.Regexp, repl string) []byte {
	loc := re.FindIndex(src)
	if loc == nil {
		return src
	}
	out := make([]byte, 0, len(src)-(loc[1]-loc[0])+len(repl))
	out = append(out, src[:loc[0]]...)
	out = append(out, repl...)
	return append(out, src[loc[1]:]...)
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

// SetSiteTitle 更新 HTML 首屏的 <title>。空值保留构建期的内置标题。
// React 起来后仍会经 /api/branding 热更新,这里的意义是让第一帧就画对,不再先显示内置名称再跳变。
func SetSiteTitle(title string) {
	initOnce.Do(initialize)
	apprMu.Lock()
	currentSiteTitle = strings.TrimSpace(title)
	rebuildServedIndexLocked()
	apprMu.Unlock()
}

// SetSiteIcon 更新 HTML 首屏的 favicon。空值使用内置图标。
func SetSiteIcon(iconURL string) {
	initOnce.Do(initialize)
	apprMu.Lock()
	currentSiteIcon = strings.TrimSpace(iconURL)
	rebuildServedIndexLocked()
	apprMu.Unlock()
}

// SetBrandMark 更新首屏里左上角的品牌标题与 logo。两个都为空表示用内置的。
func SetBrandMark(title, logoURL string) {
	initOnce.Do(initialize)
	apprMu.Lock()
	currentBrandTitle = strings.TrimSpace(title)
	currentBrandLogo = strings.TrimSpace(logoURL)
	rebuildServedIndexLocked()
	apprMu.Unlock()
}

// SetCustomCSS 更新首屏注入的自定义 CSS。空值表示不注入。
//
// 走首屏注入而不是 React 起来后再拉一次接口:后者每次刷新都要先渲染成默认样子、
// 拿到 CSS 再跳变一次,自定义配色越重跳得越明显。
func SetCustomCSS(css string) {
	initOnce.Do(initialize)
	apprMu.Lock()
	currentCustomCSS = css
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
