package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/storage"
)

func newNotifyChannelsTestRepo(t *testing.T) *storage.TrafficRepository {
	t.Helper()
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "notify-channels.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = strings.NewReader(string(raw))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, rd))
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// 增删改查 + 密钥掩码 + 掩码提交回来不覆盖 + 改完热加载进 Notifier 并真的发出去。
func TestNotifyChannelsCRUD(t *testing.T) {
	repo := newNotifyChannelsTestRepo(t)
	ctx := context.Background()
	InitNotifier(notify.Config{Enabled: true, NotifyLogin: true})
	t.Cleanup(func() { InitNotifier(notify.Config{}) })
	AttachNotifyStatusSink(repo)

	var mu sync.Mutex
	var bodies []string
	bark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"code":200,"message":"success"}`)
	}))
	defer bark.Close()

	h := NewNotifyChannelsHandler(repo)
	code, created := doJSON(t, h, http.MethodPost, "/api/admin/notify-channels", map[string]any{
		"type": "bark", "name": "我的手机", "enabled": true,
		"params": map[string]string{"server": bark.URL, "device_key": "DEVICEKEY1234"},
		"events": []string{"login", "nope"},
	})
	if code != http.StatusOK {
		t.Fatalf("新建失败 %d %v", code, created)
	}
	id := int64(created["id"].(float64))
	params := created["params"].(map[string]any)
	if params["device_key"] != "****1234" || params["server"] != bark.URL {
		t.Fatalf("密钥应掩码、非密钥原样: %v", params)
	}
	if ev := created["events"].([]any); len(ev) != 1 || ev[0] != "login" {
		t.Fatalf("未知事件分组应丢掉: %v", ev)
	}

	// 原样提交掩码 + 改名:密钥不变
	code, _ = doJSON(t, h, http.MethodPut, "/api/admin/notify-channels/"+strconv.FormatInt(id, 10), map[string]any{
		"name": "改名", "params": map[string]string{"server": bark.URL, "device_key": "****1234", "group": "告警"},
	})
	if code != http.StatusOK {
		t.Fatalf("修改失败 %d", code)
	}
	saved, _ := repo.GetNotifyChannel(ctx, id)
	if saved.Params["device_key"] != "DEVICEKEY1234" || saved.Name != "改名" || saved.Params["group"] != "告警" {
		t.Fatalf("合并不对: %+v", saved)
	}

	// 已热加载:发登录通知就到 Bark
	if err := GetNotifier().Send(ctx, notify.Event{Type: notify.EventLogin, Title: "登录", Message: "用户 `a\\_b` 登录"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(bodies) != 1 || !strings.Contains(bodies[0], `"body":"用户 a_b 登录"`) || !strings.Contains(bodies[0], `"group":"告警"`) {
		t.Fatalf("Bark 没收到或正文不对: %v", bodies)
	}
	mu.Unlock()
	// 发送状态落库
	deadline := time.Now().Add(time.Second)
	for {
		saved, _ = repo.GetNotifyChannel(ctx, id)
		if saved.LastOKAt > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if saved.LastOKAt == 0 {
		t.Fatal("成功时间应落库")
	}

	// 列表:内置 Telegram 在第一个
	code, list := doJSON(t, h, http.MethodGet, "/api/admin/notify-channels", nil)
	chs := list["channels"].([]any)
	if code != http.StatusOK || len(chs) != 2 || chs[0].(map[string]any)["builtin"] != true || chs[0].(map[string]any)["configured"] != false {
		t.Fatalf("列表不对: %v", list)
	}

	// 启用但参数不全 → 拒绝;停用的半成品可以存
	if code, _ := doJSON(t, h, http.MethodPost, "/api/admin/notify-channels", map[string]any{"type": "feishu", "enabled": true}); code != http.StatusBadRequest {
		t.Fatalf("启用缺参数应 400,得 %d", code)
	}
	if code, _ := doJSON(t, h, http.MethodPost, "/api/admin/notify-channels", map[string]any{"type": "feishu", "enabled": false}); code != http.StatusOK {
		t.Fatalf("停用的半成品应可保存,得 %d", code)
	}

	// 删除;内置不能删
	if code, _ := doJSON(t, h, http.MethodDelete, "/api/admin/notify-channels/0", nil); code != http.StatusBadRequest {
		t.Fatalf("内置不能删,得 %d", code)
	}
	if code, _ := doJSON(t, h, http.MethodDelete, "/api/admin/notify-channels/"+strconv.FormatInt(id, 10), nil); code != http.StatusOK {
		t.Fatalf("删除失败 %d", code)
	}
	if ok, _ := GetNotifier().CheckEnabled(notify.EventLogin); ok {
		t.Fatal("删掉后不该还有渠道接收登录事件")
	}
}

// 内置 Telegram:只改启用与事件订阅,存 system_settings,立即生效;保存通知设置不会把它们冲掉。
func TestNotifyChannelsBuiltinTelegram(t *testing.T) {
	repo := newNotifyChannelsTestRepo(t)
	ctx := context.Background()
	sysCfg, _ := repo.GetSystemConfig(ctx)
	sysCfg.NotifyEnabled, sysCfg.TelegramBotToken, sysCfg.TelegramChatID = true, "123456:ABCDEFGH", "42"
	sysCfg.NotifyLogin, sysCfg.NotifyIPBan = true, true
	if err := repo.UpdateSystemConfig(ctx, sysCfg); err != nil {
		t.Fatal(err)
	}
	InitNotifier(BuildNotifyConfig(ctx, repo, sysCfg))
	t.Cleanup(func() { InitNotifier(notify.Config{}) })
	if ok, _ := GetNotifier().CheckEnabled(notify.EventIPBan); !ok {
		t.Fatal("老配置零改动应照旧发")
	}

	h := NewNotifyChannelsHandler(repo)
	code, v := doJSON(t, h, http.MethodPut, "/api/admin/notify-channels/0", map[string]any{"events": []string{"login"}})
	if code != http.StatusOK || v["params"].(map[string]any)["bot_token"] != "****EFGH" || v["configured"] != true {
		t.Fatalf("got %d %v", code, v)
	}
	if ok, _ := GetNotifier().CheckEnabled(notify.EventIPBan); ok {
		t.Fatal("只订阅登录后 IP 封禁不该再发到 Telegram")
	}
	if ok, _ := GetNotifier().CheckEnabled(notify.EventLogin); !ok {
		t.Fatal("登录照发")
	}
	// 保存一次通知设置(notify-config):事件订阅应保留
	raw, _ := json.Marshal(map[string]any{"notify_enabled": true, "telegram_chat_id": "42", "notify_login": true, "notify_ip_ban": true})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/notify-config", strings.NewReader(string(raw)))
	rec := httptest.NewRecorder()
	NewNotifyConfigHandler(repo).ServeHTTP(rec, req.WithContext(auth.ContextWithUsername(ctx, "admin")))
	if rec.Code != http.StatusOK {
		t.Fatalf("保存通知设置: %d %s", rec.Code, rec.Body.String())
	}
	if n := GetNotifier(); !n.IsEnabled(notify.EventLogin) || n.IsEnabled(notify.EventIPBan) {
		t.Fatal("保存通知设置后内置渠道的事件订阅应保留")
	}
	disabled := false
	doJSON(t, h, http.MethodPut, "/api/admin/notify-channels/0", map[string]any{"enabled": disabled})
	if ok, reason := GetNotifier().CheckEnabled(notify.EventLogin); ok || reason != notify.ReasonNoChannel {
		t.Fatalf("停用内置后: %v %s", ok, reason)
	}
	// token / chat_id 没被动过
	after, _ := repo.GetSystemConfig(ctx)
	if after.TelegramBotToken != "123456:ABCDEFGH" || after.TelegramChatID != "42" {
		t.Fatal("内置渠道的 token / chat_id 不该被改")
	}
}

// 邮件渠道:密码读出整段掩码,编辑时留空 / 回传掩码都沿用旧密码,换了新密码才覆盖。
func TestNotifyChannelsSMTPPassword(t *testing.T) {
	repo := newNotifyChannelsTestRepo(t)
	ctx := context.Background()
	h := NewNotifyChannelsHandler(repo)
	code, created := doJSON(t, h, http.MethodPost, "/api/admin/notify-channels", map[string]any{
		"type": "smtp", "name": "邮件", "enabled": true,
		"params": map[string]string{"host": "smtp.example.com", "port": "465", "security": "tls",
			"username": "me@example.com", "password": "authcode-123", "to": "a@example.com, b@example.com"},
		"events": []string{"expiry"},
	})
	if code != http.StatusOK {
		t.Fatalf("新建失败 %d %v", code, created)
	}
	id := strconv.FormatInt(int64(created["id"].(float64)), 10)
	params := created["params"].(map[string]any)
	if params["password"] != "********" || params["username"] != "me@example.com" || created["configured"] != true {
		t.Fatalf("密码应整段掩码: %v", created)
	}
	for _, pw := range []string{"", "********"} {
		code, _ = doJSON(t, h, http.MethodPut, "/api/admin/notify-channels/"+id, map[string]any{
			"params": map[string]string{"host": "smtp.example.com", "port": "465", "security": "tls",
				"username": "me@example.com", "password": pw, "to": "a@example.com"},
		})
		if code != http.StatusOK {
			t.Fatalf("修改失败 %d", code)
		}
		saved, _ := repo.GetNotifyChannel(ctx, int64(created["id"].(float64)))
		if saved.Params["password"] != "authcode-123" || saved.Params["to"] != "a@example.com" {
			t.Fatalf("password=%q 应沿用旧密码: %+v", pw, saved.Params)
		}
	}
	// 启用状态下明文发密码到外网:拒绝
	code, resp := doJSON(t, h, http.MethodPut, "/api/admin/notify-channels/"+id, map[string]any{
		"params": map[string]string{"host": "smtp.example.com", "security": "none", "username": "me@example.com", "to": "a@example.com"},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("明文发密码应 400: %d %v", code, resp)
	}
}
