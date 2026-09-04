package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"miaomiaowu/internal/auth"
)

// 安全 H2:POST /api/user/external-subscriptions 由普通用户可达,URL 用户提供。
// 必须走 SSRF 安全客户端 —— 指向本机/内网的地址不能被服务端真的请求到。
func TestExternalSubscription_CreateBlocksSSRF(t *testing.T) {
	repo := relayTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateUser(ctx, "bob", "", "", "x", "user", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 一个绑在 127.0.0.1 的内网服务 —— SSRF 客户端应当拒绝拨号,它绝不该被命中。
	hit := false
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("subscription-userinfo", "upload=1; download=1; total=999999")
		_, _ = w.Write([]byte("proxies: []\n"))
	}))
	defer internal.Close()

	h := NewExternalSubscriptionsHandler(repo)
	body := `{"name":"x","url":"` + internal.URL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/user/external-subscriptions", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithUsername(ctx, "bob"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if hit {
		t.Fatalf("SSRF 未拦截:服务端请求到了内网 httptest 地址 %s", internal.URL)
	}
}
