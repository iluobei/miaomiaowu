package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"miaomiaowu/internal/auth"
)

// postNodes 向 nodes handler 发请求并返回状态码。
func postNodes(t *testing.T, h http.Handler, user, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes"+path, strings.NewReader(body))
	req = req.WithContext(auth.ContextWithUsername(context.Background(), user))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestBatchCreate_DefaultsEnabled 回归:批量导入的节点应为启用。
// 之前因 handleBatchCreate 直接用 n.Enabled 而未解析 RawEnabled,导致恒为禁用。
func TestBatchCreate_DefaultsEnabled(t *testing.T) {
	repo := relayTestRepo(t)
	const user = "admin"
	h := NewNodesHandler(repo, t.TempDir())

	body := `{"nodes":[
		{"node_name":"WithEnabled","protocol":"ss","clash_config":"{\"name\":\"WithEnabled\",\"type\":\"ss\",\"server\":\"a.com\",\"port\":443}","enabled":true},
		{"node_name":"NoEnabled","protocol":"ss","clash_config":"{\"name\":\"NoEnabled\",\"type\":\"ss\",\"server\":\"b.com\",\"port\":443}"}
	]}`
	rec := postNodes(t, h, user, "/batch", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("batch create status=%d body=%s", rec.Code, rec.Body.String())
	}

	nodes, err := repo.ListNodes(context.Background(), user)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("应创建 2 个节点, got %d", len(nodes))
	}
	for _, n := range nodes {
		if !n.Enabled {
			t.Errorf("批量导入节点 %q 应为启用, 实为禁用", n.NodeName)
		}
	}
}

// TestCreate_DefaultsEnabled 单建未带 enabled 时默认启用;显式 enabled:false 仍生效。
func TestCreate_DefaultsEnabled(t *testing.T) {
	repo := relayTestRepo(t)
	const user = "admin"
	h := NewNodesHandler(repo, t.TempDir())

	// 不带 enabled → 默认启用
	rec := postNodes(t, h, user, "", `{"node_name":"DefaultOn","protocol":"ss","clash_config":"{\"name\":\"DefaultOn\",\"type\":\"ss\",\"server\":\"a.com\",\"port\":443}"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	// 显式 enabled:false → 仍可禁用
	rec2 := postNodes(t, h, user, "", `{"node_name":"ExplicitOff","protocol":"ss","clash_config":"{\"name\":\"ExplicitOff\",\"type\":\"ss\",\"server\":\"b.com\",\"port\":443}","enabled":false}`)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("create2 status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	nodes, _ := repo.ListNodes(context.Background(), user)
	got := map[string]bool{}
	for _, n := range nodes {
		got[n.NodeName] = n.Enabled
	}
	if !got["DefaultOn"] {
		t.Error("未带 enabled 的单建节点应默认启用")
	}
	if got["ExplicitOff"] {
		t.Error("显式 enabled:false 应使节点禁用")
	}
}
