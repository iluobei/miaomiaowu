package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// 模板列举接口必须把五种模板都列出来并标对类型:前端按 type 分流,
// 漏了哪种,那种模板就在默认模板 / 订阅绑定里选不到。
func TestTemplateListIncludesAllKinds(t *testing.T) {
	// handleListTemplates 读的是相对路径 "rule_templates",所以切到临时目录再造这个子目录。
	root := t.TempDir()
	dir := filepath.Join(root, "rule_templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建模板目录: %v", err)
	}
	for _, f := range []string{"a__v3.yaml", "b__surge.conf", "c.lcf", "d__qx.qx", "e__singbox.json"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatalf("写模板 %s: %v", f, err)
		}
	}
	t.Chdir(root)

	h := &TemplateV3Handler{}
	rec := httptest.NewRecorder()
	h.handleListTemplates(rec, httptest.NewRequest(http.MethodGet, "/api/admin/template-v3/templates", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d", rec.Code)
	}
	var resp struct {
		Templates []struct {
			Name     string `json:"name"`
			Filename string `json:"filename"`
			Type     string `json:"type"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	byName := map[string]string{}
	displayName := map[string]string{}
	for _, tpl := range resp.Templates {
		byName[tpl.Filename] = tpl.Type
		displayName[tpl.Filename] = tpl.Name
	}
	for file, want := range map[string]string{
		"a__v3.yaml":      "clash",
		"b__surge.conf":   "surge",
		"c.lcf":           "loon",
		"d__qx.qx":        "qx",
		"e__singbox.json": "singbox",
	} {
		if got, ok := byName[file]; !ok {
			t.Errorf("%s 没被列出来;实际返回 %+v", file, resp.Templates)
		} else if got != want {
			t.Errorf("%s 的 type 应为 %s,得到 %q", file, want, got)
		}
	}
	if displayName["d__qx.qx"] != "d" || displayName["e__singbox.json"] != "e" {
		t.Errorf("显示名应去掉种类后缀: qx=%q singbox=%q", displayName["d__qx.qx"], displayName["e__singbox.json"])
	}
}
