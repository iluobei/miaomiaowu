package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"miaomiaowu/internal/auth"
)

func TestParseFetchedSubscriptionContent(t *testing.T) {
	uriList := "ss://YWVzLTI1Ni1nY206cGFzcw==@1.2.3.4:8388#test"
	encodedURIList := base64.StdEncoding.EncodeToString([]byte(uriList))
	clashYAML := "proxies:\n  - name: test\n    type: ss\n    server: 1.2.3.4\n    port: 8388\n    cipher: aes-256-gcm\n    password: pass\n"

	tests := []struct {
		name       string
		content    string
		wantFormat string
		wantCount  int
		wantErr    string
	}{
		{
			name:       "base64 URI list",
			content:    encodedURIList,
			wantFormat: "URI 列表",
			wantCount:  1,
		},
		{
			name:       "Clash YAML",
			content:    clashYAML,
			wantFormat: "Clash YAML",
			wantCount:  1,
		},
		{
			name:       "base64 Clash YAML",
			content:    base64.StdEncoding.EncodeToString([]byte(clashYAML)),
			wantFormat: "Clash YAML",
			wantCount:  1,
		},
		{
			name:    "HTML",
			content: "<!DOCTYPE html><html><body>login</body></html>",
			wantErr: "HTML",
		},
		{
			name:    "invalid text",
			content: "not a subscription",
			wantErr: "解析订阅内容失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxies, format, err := parseFetchedSubscriptionContent([]byte(tt.content))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFetchedSubscriptionContent: %v", err)
			}
			if format != tt.wantFormat {
				t.Fatalf("format = %q, want %q", format, tt.wantFormat)
			}
			if len(proxies) != tt.wantCount {
				t.Fatalf("proxy count = %d, want %d", len(proxies), tt.wantCount)
			}
		})
	}
}

func TestFetchSubscriptionDetectsBase64URIWithDefaultClashUA(t *testing.T) {
	const uriList = "ss://YWVzLTI1Ni1nY206cGFzcw==@1.2.3.4:8388#test"
	encodedURIList := base64.StdEncoding.EncodeToString([]byte(uriList))
	receivedUA := make(chan string, 1)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA <- r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(encodedURIList))
	}))
	defer upstream.Close()

	repo := relayTestRepo(t)
	h := NewNodesHandler(repo, t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/fetch-subscription", strings.NewReader(`{"url":"`+upstream.URL+`","force_node_skip_cert":true}`))
	req = req.WithContext(auth.ContextWithUsername(context.Background(), "admin"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("fetch status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := <-receivedUA; got != "clash-meta/2.4.0" {
		t.Fatalf("upstream User-Agent = %q, want default Clash UA", got)
	}

	var response struct {
		Proxies []map[string]any `json:"proxies"`
		Count   int              `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	if response.Count != 1 || len(response.Proxies) != 1 {
		t.Fatalf("response proxies=%d count=%d, want one proxy", len(response.Proxies), response.Count)
	}
	if got := response.Proxies[0]["skip-cert-verify"]; got != true {
		t.Fatalf("skip-cert-verify = %v, want true", got)
	}
}
