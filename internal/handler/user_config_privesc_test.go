package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

func putUserConfigAs(t *testing.T, h http.Handler, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/user/config", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithUsername(context.Background(), user))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 安全 H1:/api/user/config 任意登录用户可达。普通用户不得借它改动全局安全配置
// (否则可关掉全站防爆破/登录限流再爆破管理员,或把 proxy_groups_source_url 指向内网做 SSRF 跳板)。
func TestUserConfig_NonAdminCannotChangeGlobalSecurity(t *testing.T) {
	repo := relayTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateUser(ctx, "admin", "", "", "x", storage.RoleAdmin, ""); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := repo.CreateUser(ctx, "bob", "", "", "x", "user", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 已知基线:防爆破开启、不跳过本地 IP、代理组源为空。
	base, _ := repo.GetSystemConfig(ctx)
	base.BruteForceEnabled = true
	base.SkipLocalIP = false
	base.ProxyGroupsSourceURL = ""
	if err := repo.UpdateSystemConfig(ctx, base); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	h := NewUserConfigHandler(repo)

	// 普通用户 bob 试图关掉防爆破 + 塞内网代理组源 + 跳过本地 IP。
	attack := `{"brute_force_enabled":false,"login_rate_max_attempts":0,` +
		`"sub_rate_limit_enabled":false,"skip_local_ip":true,` +
		`"proxy_groups_source_url":"http://169.254.169.254/"}`
	rec := putUserConfigAs(t, h, "bob", attack)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob PUT status=%d body=%s", rec.Code, rec.Body.String())
	}

	after, _ := repo.GetSystemConfig(ctx)
	if !after.BruteForceEnabled {
		t.Errorf("越权:普通用户关掉了全局防爆破(BruteForceEnabled=false)")
	}
	if after.SkipLocalIP {
		t.Errorf("越权:普通用户改了 skip_local_ip")
	}
	if after.ProxyGroupsSourceURL != "" {
		t.Errorf("越权:普通用户改了 proxy_groups_source_url = %q", after.ProxyGroupsSourceURL)
	}

	// 对照:管理员改同一字段应当生效(证明没把功能改废)。
	rec = putUserConfigAs(t, h, "admin", `{"brute_force_enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PUT status=%d body=%s", rec.Code, rec.Body.String())
	}
	after, _ = repo.GetSystemConfig(ctx)
	if after.BruteForceEnabled {
		t.Errorf("管理员关闭防爆破未生效")
	}
}
