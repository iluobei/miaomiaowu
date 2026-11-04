package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

// ruleTemplateSourceTestSetup 切到临时目录(处理器按相对路径读 rule_templates/),建库并造一个管理员和一个普通用户。
func ruleTemplateSourceTestSetup(t *testing.T) *storage.TrafficRepository {
	t.Helper()
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	repo, err := storage.NewTrafficRepository(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	ctx := context.Background()
	if err := repo.CreateUser(ctx, "admin", "", "", "x", storage.RoleAdmin, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(ctx, "bob", "", "", "x", "user", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("rule_templates", 0755); err != nil {
		t.Fatal(err)
	}
	return repo
}

func ruleTemplateReq(t *testing.T, h http.Handler, user, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req = req.WithContext(auth.ContextWithUsername(req.Context(), user))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 从链接导入的模板记下来源,改名 / 删除跟着走;记来源不影响普通用户看到内置模板。
func TestRuleTemplateSourceURL(t *testing.T) {
	repo := ruleTemplateSourceTestSetup(t)
	_ = os.WriteFile(filepath.Join("rule_templates", "builtin.yaml"), []byte("rules: []\n"), 0644)
	h := NewRuleTemplatesHandler(repo)

	// 上传时带 source_url
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("template", "remote.yaml")
	_, _ = fw.Write([]byte("rules: []\n"))
	_ = mw.WriteField("source_url", "https://example.com/t.yaml")
	_ = mw.Close()
	if rec := ruleTemplateReq(t, h, "admin", http.MethodPost, "/api/admin/rule-templates/upload", buf.Bytes(), mw.FormDataContentType()); rec.Code != 200 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	list := func(user string) map[string]any {
		rec := ruleTemplateReq(t, h, user, http.MethodGet, "/api/admin/rule-templates", nil, "")
		var m map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return m
	}
	if src := list("admin")["sources"].(map[string]any); src["remote.yaml"] != "https://example.com/t.yaml" {
		t.Fatalf("列表应带上来源链接: %v", src)
	}

	// 给内置模板也记一个来源:普通用户照样看得到它(不能把它当成「有归属」藏起来),但拿不到来源链接
	_ = repo.SetRuleTemplateSource(context.Background(), "builtin.yaml", "https://example.com/b.yaml")
	m := list("bob")
	if !strings.Contains(strings.Join(toStrings(m["templates"]), ","), "builtin.yaml") {
		t.Fatalf("普通用户应能看到内置模板: %v", m["templates"])
	}
	if len(m["sources"].(map[string]any)) != 0 {
		t.Fatalf("普通用户不该拿到别人模板的来源: %v", m["sources"])
	}

	// 改名跟着走,删除清掉
	body, _ := json.Marshal(map[string]string{"old_name": "remote.yaml", "new_name": "remote2.yaml"})
	if rec := ruleTemplateReq(t, h, "admin", http.MethodPost, "/api/admin/rule-templates/rename", body, "application/json"); rec.Code != 200 {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	srcs, _ := repo.ListRuleTemplateSources(context.Background())
	if srcs["remote2.yaml"] != "https://example.com/t.yaml" || srcs["remote.yaml"] != "" {
		t.Fatalf("改名后来源应跟着走: %v", srcs)
	}
	if rec := ruleTemplateReq(t, h, "admin", http.MethodDelete, "/api/admin/rule-templates/remote2.yaml", nil, ""); rec.Code != 200 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if srcs, _ = repo.ListRuleTemplateSources(context.Background()); srcs["remote2.yaml"] != "" {
		t.Fatalf("删除后来源记录应清掉: %v", srcs)
	}
}

func TestRuleTemplateUpdateFromSourceGuards(t *testing.T) {
	repo := ruleTemplateSourceTestSetup(t)
	_ = os.WriteFile(filepath.Join("rule_templates", "a.yaml"), []byte("rules: []\n"), 0644)
	h := NewRuleTemplatesHandler(repo)
	post := func(user string, payload map[string]string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(payload)
		return ruleTemplateReq(t, h, user, http.MethodPost, "/api/admin/rule-templates/update-from-source", b, "application/json")
	}
	if rec := post("admin", map[string]string{"name": "a.yaml"}); rec.Code != 400 || !strings.Contains(rec.Body.String(), "没有记录来源链接") {
		t.Fatalf("没有来源又没填链接应提示填写: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post("admin", map[string]string{"name": "a.yaml", "url": "ftp://x/y"}); rec.Code != 400 {
		t.Fatalf("非 http(s) 链接应拒绝: %d", rec.Code)
	}
	if rec := post("admin", map[string]string{"name": "../etc/passwd", "url": "https://x/y"}); rec.Code != 400 {
		t.Fatalf("非法模板名应拒绝: %d", rec.Code)
	}
	if rec := post("bob", map[string]string{"name": "a.yaml", "url": "https://x/y"}); rec.Code != 403 {
		t.Fatalf("普通用户不能改内置模板: %d", rec.Code)
	}
	// 指向本机 / 内网:经防 SSRF 的客户端拉取,被拒,模板内容不变
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("rules: [x]\n")) }))
	defer srv.Close()
	if rec := post("admin", map[string]string{"name": "a.yaml", "url": srv.URL}); rec.Code != http.StatusBadGateway {
		t.Fatalf("指向本机的链接应被防 SSRF 拦下: %d %s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(filepath.Join("rule_templates", "a.yaml")); string(b) != "rules: []\n" {
		t.Fatalf("拉取失败时模板内容不能变: %q", b)
	}
}

func toStrings(v any) []string {
	var out []string
	if arr, ok := v.([]any); ok {
		for _, x := range arr {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
