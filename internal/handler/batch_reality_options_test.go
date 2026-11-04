package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

// 批量开关 VLESS REALITY 节点的 reality-opts.support-x25519mlkem768。
// 只动 VLESS REALITY 节点;普通 VLESS(TLS)和其他协议跳过;关闭时删键而不是写 false。
func TestBatchRealityOptionsTogglesMLKEM(t *testing.T) {
	ctx := context.Background()
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "reality-opts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	create := func(name, protocol, cfg string) int64 {
		t.Helper()
		n, err := repo.CreateNode(ctx, storage.Node{
			Username: "admin", NodeName: name, Protocol: protocol, ParsedConfig: cfg, ClashConfig: cfg, Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	realityID := create("r1", "vless", `{"name":"r1","type":"vless","server":"1.2.3.4","port":443,"uuid":"u","tls":true,"servername":"www.example.com","reality-opts":{"public-key":"pk","short-id":"ab"},"client-fingerprint":"chrome"}`)
	tlsID := create("t1", "vless", `{"name":"t1","type":"vless","server":"1.2.3.4","port":8443,"uuid":"u","tls":true,"servername":"a.example.com"}`)
	ssID := create("s1", "ss", `{"name":"s1","type":"ss","server":"1.2.3.4","port":8388,"cipher":"aes-128-gcm","password":"p"}`)

	h := NewNodesHandler(repo, t.TempDir())
	post := func(body string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/batch-reality-options", bytes.NewReader([]byte(body)))
		req = req.WithContext(auth.ContextWithUsername(req.Context(), "admin"))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST %s: status=%d body=%s", body, rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	opts := func(id int64) (clash, parsed map[string]any) {
		t.Helper()
		n, err := repo.GetNode(ctx, id, "admin")
		if err != nil {
			t.Fatal(err)
		}
		var c, p map[string]any
		_ = json.Unmarshal([]byte(n.ClashConfig), &c)
		_ = json.Unmarshal([]byte(n.ParsedConfig), &p)
		co, _ := c["reality-opts"].(map[string]any)
		po, _ := p["reality-opts"].(map[string]any)
		return co, po
	}
	ids := fmt.Sprintf(`[%d,%d,%d]`, realityID, tlsID, ssID)

	resp := post(`{"node_ids":` + ids + `,"support_x25519mlkem768":true}`)
	if resp["success"] != float64(1) || resp["skipped"] != float64(2) {
		t.Fatalf("开启应只改 REALITY 节点: %v", resp)
	}
	co, po := opts(realityID)
	if co["support-x25519mlkem768"] != true || po["support-x25519mlkem768"] != true {
		t.Fatalf("clash/parsed 都应开启: clash=%v parsed=%v", co, po)
	}
	if co["public-key"] != "pk" || co["short-id"] != "ab" {
		t.Fatalf("其余 reality-opts 不应受影响: %v", co)
	}
	if c, _ := opts(tlsID); c != nil {
		t.Fatalf("普通 VLESS TLS 节点不应凭空长出 reality-opts: %v", c)
	}

	// 重复开启:已是目标值,算跳过
	if resp := post(`{"node_ids":` + ids + `,"support_x25519mlkem768":true}`); resp["success"] != float64(0) {
		t.Fatalf("重复开启不应再计成功: %v", resp)
	}

	resp = post(`{"node_ids":` + ids + `,"support_x25519mlkem768":false}`)
	if resp["success"] != float64(1) {
		t.Fatalf("关闭应改回 REALITY 节点: %v", resp)
	}
	co, po = opts(realityID)
	if _, has := co["support-x25519mlkem768"]; has {
		t.Fatalf("关闭应删键: %v", co)
	}
	if _, has := po["support-x25519mlkem768"]; has {
		t.Fatalf("parsed_config 也应删键: %v", po)
	}
}

func TestBatchRealityOptionsRequiresValue(t *testing.T) {
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "reality-opts-req.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/batch-reality-options", bytes.NewReader([]byte(`{"node_ids":[1]}`)))
	req = req.WithContext(auth.ContextWithUsername(req.Context(), "admin"))
	rec := httptest.NewRecorder()
	NewNodesHandler(repo, t.TempDir()).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺开关值应 400, got %d", rec.Code)
	}
}
