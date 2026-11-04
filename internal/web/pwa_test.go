package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func getManifest(t *testing.T) (map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/manifest.webmanifest", nil))
	if rec.Code != 200 {
		t.Fatalf("GET /manifest.webmanifest status=%d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("manifest 不是合法 JSON: %v\n%s", err, rec.Body.String())
	}
	return m, rec
}

func TestManifestDefaultsAndHeaders(t *testing.T) {
	m, rec := getManifest(t)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/manifest+json") {
		t.Fatalf("Content-Type=%q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control=%q", cc)
	}
	if m["name"] != manifestAppName || m["short_name"] != manifestAppName {
		t.Fatalf("name=%v short_name=%v", m["name"], m["short_name"])
	}
	for k, want := range map[string]string{"start_url": "/", "scope": "/", "display": "standalone"} {
		if m[k] != want {
			t.Fatalf("%s=%v want %s", k, m[k], want)
		}
	}
	icons, _ := m["icons"].([]any)
	if len(icons) != 2 {
		t.Fatalf("内置图标应为 192 + 512 两张,得到 %v", m["icons"])
	}
	first := icons[0].(map[string]any)
	if first["sizes"] != "192x192" || first["type"] != "image/png" {
		t.Fatalf("icons[0]=%v", first)
	}
}

func serveIndexBody(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("GET / status=%d", rec.Code)
	}
	return rec.Body.String()
}

// 首屏里的 PWA 开关占位符随 SetPWAEnabled 变化。
func TestIndexInjectsPWAPlaceholder(t *testing.T) {
	initOnce.Do(initialize)
	apprMu.Lock()
	saved := indexBytes
	indexBytes = []byte(`<meta name="mmw-pwa" content="__MMW_PWA_ENABLED__">`)
	rebuildServedIndexLocked()
	apprMu.Unlock()
	defer func() {
		apprMu.Lock()
		indexBytes = saved
		rebuildServedIndexLocked()
		apprMu.Unlock()
		SetPWAEnabled(true)
	}()

	SetPWAEnabled(true)
	if got := serveIndexBody(t); !strings.Contains(got, `name="mmw-pwa" content="true"`) {
		t.Fatalf("默认应为 true:\n%s", got)
	}
	SetPWAEnabled(false)
	if got := serveIndexBody(t); !strings.Contains(got, `name="mmw-pwa" content="false"`) {
		t.Fatalf("关掉后应为 false:\n%s", got)
	}
}

func withStaticFS(t *testing.T, files fstest.MapFS) {
	t.Helper()
	initOnce.Do(initialize)
	saved := staticFS
	staticFS = files
	t.Cleanup(func() { staticFS = saved })
}

func TestServiceWorkerHeadersAndVersion(t *testing.T) {
	withStaticFS(t, fstest.MapFS{
		"sw.js": {Data: []byte(`const VERSION = '__MMW_SW_VERSION__'`)},
	})
	SetPWAEnabled(true)

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/sw.js", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Fatalf("Content-Type=%q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control=%q", cc)
	}
	if v := rec.Header().Get("Service-Worker-Allowed"); v != "/" {
		t.Fatalf("Service-Worker-Allowed=%q", v)
	}
	body := rec.Body.String()
	if strings.Contains(body, swVersionPlaceholder) || !strings.Contains(body, swVersion()) {
		t.Fatalf("版本占位符没替换: %s", body)
	}
}

// 关掉 PWA 后 /sw.js 改发自我注销脚本:已装上的 SW 下次检查更新就把自己和缓存一起删掉。
func TestServiceWorkerKillSwitchWhenDisabled(t *testing.T) {
	withStaticFS(t, fstest.MapFS{"sw.js": {Data: []byte(`self.addEventListener('fetch', () => {})`)}})
	SetPWAEnabled(false)
	defer SetPWAEnabled(true)

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/sw.js", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "unregister()") || strings.Contains(body, "'fetch'") {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal("自我注销脚本同样不能被缓存")
	}
}

// dist 里没有 sw.js(没跑过前端构建)时必须 404,不能被 SPA 兜底成 index.html ——
// 那会让浏览器把 HTML 当脚本注册,反复失败。
func TestServiceWorkerMissingIs404(t *testing.T) {
	withStaticFS(t, fstest.MapFS{"index.html": {Data: []byte("<html></html>")}})
	SetPWAEnabled(true)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/sw.js", nil))
	if rec.Code != 404 {
		t.Fatalf("status=%d", rec.Code)
	}
}
