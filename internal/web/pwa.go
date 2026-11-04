package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

// PWA 安装支持:manifest + service worker,从妙妙屋X 移植。
//
// manifest 不放 public/ 当静态文件:http.FileServer 认不出 .webmanifest 的 Content-Type,
// 而且名称 / 图标这类值以后要跟着设置走,放这里一处生成更稳。
//
// sw.js 是 public/ 里手写的文件(不用 vite-plugin-pwa:它要接管构建产物),这里只做两件事:
//   - 把 __MMW_SW_VERSION__ 换成本次构建的指纹,缓存名随之变化,升级后旧缓存在 activate 时清掉;
//   - 管理员关掉 PWA 时改发一个「自我注销」的脚本,已经装上的 SW 下次检查更新就把自己连同缓存一起删掉,
//     不必等用户打开新版页面(前端那一侧也会主动注销,两边都兜)。

const (
	pwaEnabledPlaceholder = "__MMW_PWA_ENABLED__"
	swVersionPlaceholder  = "__MMW_SW_VERSION__"

	manifestAppName    = "妙妙屋"
	manifestThemeColor = "#ffffff"
)

// currentPWAEnabled 系统设置里的 PWA 开关(默认开),受 apprMu 保护。
var currentPWAEnabled = true

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

var builtinManifestIcons = []manifestIcon{
	{Src: "/images/android-chrome-192x192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
	{Src: "/images/android-chrome-512x512.png", Sizes: "512x512", Type: "image/png", Purpose: "any"},
}

// SetPWAEnabled 系统设置里的 PWA 开关(默认开)。关掉后首屏 meta 告诉前端不要注册并注销已有的
// service worker,/sw.js 也改发自我注销脚本。manifest 照常提供 —— 没有 SW 时「安装」只是一个
// 独立窗口的快捷方式,没有缓存,不会带来关 PWA 想避免的那类问题。
func SetPWAEnabled(enabled bool) {
	initOnce.Do(initialize)
	apprMu.Lock()
	defer apprMu.Unlock()
	currentPWAEnabled = enabled
	rebuildServedIndexLocked()
}

func buildManifest() []byte {
	body, _ := json.Marshal(map[string]any{
		"id":               "/",
		"name":             manifestAppName,
		"short_name":       manifestAppName,
		"start_url":        "/",
		"scope":            "/",
		"display":          "standalone",
		"theme_color":      manifestThemeColor,
		"background_color": manifestThemeColor,
		"icons":            builtinManifestIcons,
	})
	return body
}

func serveManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	body := buildManifest()
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

// swKillScript PWA 关闭时下发的 /sw.js:装上即接管、清掉本站的缓存、注销自己,不拦截任何请求。
// 缓存名前缀与 public/sw.js 的 CACHE_PREFIX 保持一致。
const swKillScript = `// PWA 已在系统设置里关闭:清理缓存并注销自己。
self.addEventListener('install', function () { self.skipWaiting() })
self.addEventListener('activate', function (event) {
  event.waitUntil(
    caches.keys()
      .then(function (keys) {
        return Promise.all(keys.filter(function (k) { return k.indexOf('mmw-') === 0 }).map(function (k) { return caches.delete(k) }))
      })
      .then(function () { return self.registration.unregister() })
  )
})
`

// swVersion 本次构建的指纹:取嵌入的 index.html(替换占位符之前的原始字节)的哈希。
// index.html 引用的是带内容哈希的 chunk,前端一重新构建它就变;只改后端时它不变,
// 浏览器里的静态缓存也就不必作废。
func swVersion() string {
	sum := sha256.Sum256(indexBytes)
	return hex.EncodeToString(sum[:6])
}

func serveServiceWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apprMu.RLock()
	enabled := currentPWAEnabled
	apprMu.RUnlock()

	var body []byte
	if enabled {
		raw, err := fs.ReadFile(staticFS, "sw.js")
		if err != nil {
			// 没跑过前端构建的 dist 里没有 sw.js —— 不能退回 index.html(那会被当成脚本注册失败,
			// 还会让浏览器反复重试),明确 404。
			http.NotFound(w, r)
			return
		}
		body = []byte(strings.ReplaceAll(string(raw), swVersionPlaceholder, swVersion()))
	} else {
		body = []byte(swKillScript)
	}

	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	// SW 脚本必须每次都回源比对:浏览器正是靠字节变化发现新版本的。
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
