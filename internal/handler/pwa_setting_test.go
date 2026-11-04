package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"miaomiaowu/internal/storage"
)

// 没设过 = 开;PUT 存下来之后回调同步首屏注入(web.SetPWAEnabled)。
func TestPWASettingDefaultsOnAndPersists(t *testing.T) {
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "pwa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if !LoadPWAEnabled(context.Background(), repo) {
		t.Fatal("没设过时应默认开启")
	}

	var synced []bool
	h := NewPWASettingHandler(repo, func(on bool) { synced = append(synced, on) })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/admin/system-settings/pwa", bytes.NewBufferString(`{"enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rec.Code, rec.Body.String())
	}
	if LoadPWAEnabled(context.Background(), repo) {
		t.Fatal("关掉后读出来仍是开")
	}
	if len(synced) != 1 || synced[0] {
		t.Fatalf("回调 = %v,应收到一次 false", synced)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/system-settings/pwa", nil))
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"enabled":false`)) {
		t.Fatalf("GET body=%s", rec.Body.String())
	}

	// 缺字段不能被当成 false 存进去
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/admin/system-settings/pwa", bytes.NewBufferString(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 enabled 时 status=%d,应为 400", rec.Code)
	}
	if len(synced) != 1 {
		t.Fatal("非法请求不应触发同步回调")
	}
}
