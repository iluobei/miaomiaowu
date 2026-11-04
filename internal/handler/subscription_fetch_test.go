package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

// 订阅获取开关:真的把各个订阅出口的 handler 跑起来,开 → 不是 403,关 → 403 且带说明。
// 关掉之后再打开要能恢复(凭据不动,只是拦在门口)。
type subFetchFixture struct {
	repo        *storage.TrafficRepository
	subEndpoint http.Handler // /api/clash/subscribe
	shortLink   *shortLinkHandler
	tempAccess  http.Handler
	provider    http.Handler
	admin       http.Handler // /api/admin/users/subscription-fetch
	token       string
	shortCode   string // /{文件短码}{用户短码}
}

func newSubFetchFixture(t *testing.T) *subFetchFixture {
	t.Helper()
	dir := t.TempDir()
	repo, err := storage.NewTrafficRepository(filepath.Join(dir, "subfetch.db"))
	if err != nil {
		t.Fatalf("建库: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	ctx := context.Background()
	if err := repo.CreateUser(ctx, "alice", "a@t.local", "alice", "h", storage.RoleUser, ""); err != nil {
		t.Fatalf("建用户: %v", err)
	}
	baseDir := filepath.Join(dir, "subscribes")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rawYAML := "proxies:\n  - {name: HK, type: ss, server: 9.9.9.9, port: 443, cipher: aes-128-gcm, password: p}\n"
	if err := os.WriteFile(filepath.Join(baseDir, "alice.yaml"), []byte(rawYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sf, err := repo.CreateSubscribeFile(ctx, storage.SubscribeFile{
		Name: "alice-sub", Type: storage.SubscribeTypeCreate, Filename: "alice.yaml",
	})
	if err != nil {
		t.Fatalf("建订阅文件: %v", err)
	}
	if err := repo.SetUserSubscriptions(ctx, "alice", []int64{sf.ID}); err != nil {
		t.Fatalf("分配订阅: %v", err)
	}
	token, err := repo.GetOrCreateUserToken(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	userCode, err := repo.GetEffectiveUserShortCode(ctx, "alice")
	if err != nil || userCode == "" {
		t.Fatalf("用户短码: %q %v", userCode, err)
	}

	store := auth.NewTokenStore(0)
	return &subFetchFixture{
		repo:        repo,
		subEndpoint: NewSubscriptionEndpoint(store, repo, baseDir),
		shortLink:   NewShortLinkHandler(repo, NewSubscriptionHandlerConcrete(repo, baseDir)),
		tempAccess:  NewTempSubscriptionAccessHandler(repo),
		provider:    NewProxyProviderServeHandler(repo),
		admin:       NewUserSubscriptionFetchHandler(repo),
		token:       token,
		shortCode:   sf.FileShortCode + userCode,
	}
}

func subFetchReq(target string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Header.Set("User-Agent", "mihomo/1.19.0") // 临时订阅只认 mihomo / ClashMetaForAndroid
	return r
}

// adminCall 走管理员接口改开关。
func (f *subFetchFixture) adminCall(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.admin.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/users/subscription-fetch", strings.NewReader(body)))
	return rec
}

// fetch 用 token 拉一次 clash 订阅。
func (f *subFetchFixture) fetch() *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.subEndpoint.ServeHTTP(rec, subFetchReq("/api/clash/subscribe?filename=alice.yaml&token="+f.token))
	return rec
}

func (f *subFetchFixture) raw(t *testing.T) storage.SubscriptionFetchStatus {
	t.Helper()
	st, err := f.repo.GetUserSubscriptionFetch(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// listEntry 用户列表里 alice 的开关状态(已折算到期)。
func (f *subFetchFixture) listEntry(t *testing.T) (string, int64) {
	t.Helper()
	rec := httptest.NewRecorder()
	NewUserListHandler(f.repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/users", nil))
	var list struct {
		Users []struct {
			Username               string `json:"username"`
			SubscriptionFetch      string `json:"subscription_fetch"`
			SubscriptionFetchUntil int64  `json:"subscription_fetch_until"`
		} `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("用户列表: %v %s", err, rec.Body.String())
	}
	for _, u := range list.Users {
		if u.Username == "alice" {
			return u.SubscriptionFetch, u.SubscriptionFetchUntil
		}
	}
	t.Fatalf("用户列表里没有 alice: %s", rec.Body.String())
	return "", 0
}

// fakeSubFetchClock 把开关用的时钟换成假的,返回可推进的指针。
func fakeSubFetchClock(t *testing.T) *time.Time {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	orig := subscriptionFetchNow
	subscriptionFetchNow = func() time.Time { return now }
	t.Cleanup(func() { subscriptionFetchNow = orig })
	return &now
}

func TestSubscriptionFetchSwitchBlocksAllSubscriptionPaths(t *testing.T) {
	f := newSubFetchFixture(t)

	type route struct {
		name  string
		serve func() *httptest.ResponseRecorder
	}
	via := func(h http.Handler, target string) func() *httptest.ResponseRecorder {
		return func() *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, subFetchReq(target))
			return rec
		}
	}
	routes := []route{
		{"clash token", f.fetch},
		{"短码", func() *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			if !f.shortLink.TryServe(rec, subFetchReq("/"+f.shortCode)) {
				t.Fatalf("短码 %s 未命中", f.shortCode)
			}
			return rec
		}},
		{"代理集合", via(f.provider, "/api/proxy-provider/1?token="+f.token)},
	}

	check := func(wantBlocked bool) {
		t.Helper()
		for _, rt := range routes {
			rec := rt.serve()
			blocked := rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "订阅获取已被管理员关闭")
			if blocked != wantBlocked {
				t.Errorf("%s: 期望拦截=%v,得到 %d %q", rt.name, wantBlocked, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
		}
	}

	// 默认开:新用户(以及补列后的存量用户)都是 on
	if st := f.raw(t); st.State != storage.SubscriptionFetchOn || st.Until != 0 {
		t.Fatalf("默认应为 on,得到 %+v", st)
	}
	check(false)
	// clash token 这条开着时得真拿到订阅,不只是「不是 403」
	if rec := f.fetch(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HK") {
		t.Fatalf("开着时 clash token 应 200 带节点,得到 %d %q", rec.Code, rec.Body.String())
	}

	if rec := f.adminCall(`{"username":"alice","enabled":false}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"off"`) {
		t.Fatalf("管理员关闭: %d %s", rec.Code, rec.Body.String())
	}
	check(true)
	if st, _ := f.listEntry(t); st != storage.SubscriptionFetchOff {
		t.Fatalf("用户列表应带 subscription_fetch=off,得到 %s", st)
	}

	if rec := f.adminCall(`{"username":"alice","enabled":true}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"on"`) {
		t.Fatalf("管理员打开: %d %s", rec.Code, rec.Body.String())
	}
	check(false)

	// 参数校验
	if rec := f.adminCall(`{"username":"nobody","enabled":true}`); rec.Code != http.StatusNotFound {
		t.Errorf("不存在的用户应 404,得到 %d", rec.Code)
	}
	if rec := f.adminCall(`{"username":"alice"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("缺 enabled 应 400,得到 %d", rec.Code)
	}
}

// 临时订阅按签发人拦;拒掉的那次不能把一次性链接耗掉。
func TestSubscriptionFetchSwitchBlocksTempSubscription(t *testing.T) {
	f := newSubFetchFixture(t)
	sub := tempSubStore.CreateFor("alice", []any{map[string]any{"name": "T1", "type": "ss", "server": "1.1.1.1", "port": 443, "cipher": "aes-128-gcm", "password": "p"}}, 1, 60)
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		f.tempAccess.ServeHTTP(rec, subFetchReq("/t/"+sub.ID))
		return rec
	}

	if rec := f.adminCall(`{"username":"alice","enabled":false}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := get(); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "订阅获取") {
		t.Fatalf("关闭后临时订阅应 403,得到 %d %q", rec.Code, rec.Body.String())
	}
	if rec := f.adminCall(`{"username":"alice","enabled":true}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := get(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "T1") {
		t.Fatalf("重新打开后(max_access=1 未被拒绝的那次耗掉)应 200,得到 %d %q", rec.Code, rec.Body.String())
	}
}

// 限时打开(#1497):到期后不靠定时任务,读的时候按 off 处理并写回库;到期后回到打开前的关闭状态。
func TestSubscriptionFetchTimedOpenExpires(t *testing.T) {
	f := newSubFetchFixture(t)
	now := fakeSubFetchClock(t)

	// 平时关着,限时打开 10 分钟
	if rec := f.adminCall(`{"username":"alice","enabled":false}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := f.fetch(); rec.Code != http.StatusForbidden {
		t.Fatalf("关着时应 403,得到 %d", rec.Code)
	}
	wantUntil := now.Unix() + 600
	rec := f.adminCall(`{"username":"alice","enabled":true,"duration_seconds":600}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"subscription_fetch_until":`+strconv.FormatInt(wantUntil, 10)) {
		t.Fatalf("限时打开应回到期时刻 %d: %d %s", wantUntil, rec.Code, rec.Body.String())
	}
	if rec := f.fetch(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HK") {
		t.Fatalf("限时打开期间应能拉订阅,得到 %d %q", rec.Code, rec.Body.String())
	}
	if st, until := f.listEntry(t); st != storage.SubscriptionFetchOn || until != wantUntil {
		t.Fatalf("用户列表应显示 on + until=%d,得到 %s %d", wantUntil, st, until)
	}

	// 到期:列表先按 off 显示(只折算不写库)
	*now = now.Add(601 * time.Second)
	if st, until := f.listEntry(t); st != storage.SubscriptionFetchOff || until != 0 {
		t.Fatalf("到期后用户列表应显示 off,得到 %s %d", st, until)
	}
	if st := f.raw(t); st.State != storage.SubscriptionFetchOn || st.Until != wantUntil {
		t.Fatalf("用户列表只折算不写库,库里应仍是 on/until,得到 %+v", st)
	}
	// 拉订阅被拒,且库里已写回 off
	if rec := f.fetch(); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "订阅获取已被管理员关闭") {
		t.Fatalf("到期后应 403,得到 %d %q", rec.Code, rec.Body.String())
	}
	if st := f.raw(t); st.State != storage.SubscriptionFetchOff || st.Until != 0 {
		t.Fatalf("到期后库里应写回 off/0,得到 %+v", st)
	}

	// 打开前是永久开,限时打开到期同样回到 off
	if rec := f.adminCall(`{"username":"alice","enabled":true}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := f.adminCall(`{"username":"alice","enabled":true,"duration_seconds":3600}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	*now = now.Add(2 * time.Hour)
	if rec := f.fetch(); rec.Code != http.StatusForbidden {
		t.Fatalf("永久开 → 限时打开到期后应 403,得到 %d", rec.Code)
	}
	if st := f.raw(t); st.State != storage.SubscriptionFetchOff || st.Until != 0 {
		t.Fatalf("到期后应回到 off,得到 %+v", st)
	}

	// 永久打开不会过期,且清掉 until
	if rec := f.adminCall(`{"username":"alice","enabled":true}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if st := f.raw(t); st.State != storage.SubscriptionFetchOn || st.Until != 0 {
		t.Fatalf("永久打开应清 until,得到 %+v", st)
	}
	*now = now.Add(30 * 24 * time.Hour)
	if rec := f.fetch(); rec.Code != http.StatusOK {
		t.Fatalf("永久打开不应过期,得到 %d", rec.Code)
	}
}

// 续期换 until;限时打开后手动关闭清 until,到点也不变;时长要校验;写回不覆盖续期。
func TestSubscriptionFetchTimedOpenRules(t *testing.T) {
	f := newSubFetchFixture(t)
	now := fakeSubFetchClock(t)
	ctx := context.Background()

	// 续期
	if rec := f.adminCall(`{"username":"alice","enabled":true,"duration_seconds":600}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if rec := f.adminCall(`{"username":"alice","enabled":true,"duration_seconds":3600}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"subscription_fetch_until":`+strconv.FormatInt(now.Unix()+3600, 10)) {
		t.Fatalf("续期: %d %s", rec.Code, rec.Body.String())
	}
	// 限时打开后关闭 → off,清 until,到点也不变
	if rec := f.adminCall(`{"username":"alice","enabled":false}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if st := f.raw(t); st.State != storage.SubscriptionFetchOff || st.Until != 0 {
		t.Fatalf("关闭应清 until,得到 %+v", st)
	}
	*now = now.Add(2 * 24 * time.Hour)
	if st := subscriptionFetchState(ctx, f.repo, "alice"); st.State != storage.SubscriptionFetchOff || st.Until != 0 {
		t.Fatalf("off 不受到期影响,得到 %+v", st)
	}

	// 时长校验
	if rec := f.adminCall(`{"username":"alice","enabled":true}`); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	for _, body := range []string{
		`{"username":"alice","enabled":false,"duration_seconds":600}`,
		`{"username":"alice","enabled":true,"duration_seconds":30}`,
		`{"username":"alice","enabled":true,"duration_seconds":691200}`,
		`{"username":"alice","enabled":true,"duration_seconds":-1}`,
	} {
		if rec := f.adminCall(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s 应 400,得到 %d", body, rec.Code)
		}
	}
	if st := f.raw(t); st.State != storage.SubscriptionFetchOn || st.Until != 0 {
		t.Fatalf("被拒的请求不应改动开关,得到 %+v", st)
	}

	// 写回只认读到的那个 until:期间被续期的不会被旧的到期覆盖
	if _, err := f.repo.SetUserSubscriptionFetch(ctx, "alice", true, now.Unix()+100); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetUserSubscriptionFetch(ctx, "alice", true, now.Unix()+9000); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.ExpireUserSubscriptionFetch(ctx, "alice", now.Unix()+100); err != nil {
		t.Fatal(err)
	}
	if st := f.raw(t); st.State != storage.SubscriptionFetchOn || st.Until != now.Unix()+9000 {
		t.Fatalf("续期后不应被旧的到期写回覆盖,得到 %+v", st)
	}
}
