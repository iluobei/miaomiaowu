package auth

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// RP ID / origin 按请求 Host 推导,覆盖直连 localhost、反代、裸 IP、upstream 名几种部署形态。
func TestWebAuthnFor(t *testing.T) {
	cases := []struct {
		name       string
		host       string
		headers    map[string]string
		wantRPID   string
		wantOrigin string
		wantErr    bool
	}{
		{name: "直连 localhost 允许 http", host: "localhost:8080", wantRPID: "localhost", wantOrigin: "http://localhost:8080"},
		{name: "域名一律按 https", host: "mmw.example.com", wantRPID: "mmw.example.com", wantOrigin: "https://mmw.example.com"},
		{name: "反代带 X-Forwarded-Host", host: "127.0.0.1:8080", headers: map[string]string{"X-Forwarded-Host": "mmw.example.com", "X-Forwarded-Proto": "https"}, wantRPID: "mmw.example.com", wantOrigin: "https://mmw.example.com"},
		{name: "裸 IP 拒绝", host: "1.2.3.4:8080", wantErr: true},
		{name: "反代回源 127.0.0.1 不算本机", host: "127.0.0.1:8080", headers: map[string]string{"X-Forwarded-Proto": "https"}, wantErr: true},
		{name: "upstream 名含下划线拒绝", host: "mmw_example_com", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/login/passkey/begin", nil)
			r.Host = tc.host
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			wa, err := WebAuthnFor(r, "")
			if tc.wantErr {
				if !errors.Is(err, ErrWebAuthnInsecureContext) {
					t.Fatalf("expected ErrWebAuthnInsecureContext, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wa.Config.RPID != tc.wantRPID {
				t.Errorf("RPID = %q, want %q", wa.Config.RPID, tc.wantRPID)
			}
			if len(wa.Config.RPOrigins) != 1 || wa.Config.RPOrigins[0] != tc.wantOrigin {
				t.Errorf("RPOrigins = %v, want [%s]", wa.Config.RPOrigins, tc.wantOrigin)
			}
		})
	}
}

func TestWebAuthnSessionStoreConsumeOnce(t *testing.T) {
	store := NewWebAuthnSessionStore(time.Second)
	data := webauthn.SessionData{}
	token, err := store.Issue(&data, "alice", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, username, remember, ok := store.Consume(token); !ok || username != "alice" || !remember {
		t.Fatalf("first consume: ok=%v username=%q remember=%v", ok, username, remember)
	}
	if _, _, _, ok := store.Consume(token); ok {
		t.Fatal("token should be one-shot")
	}
}
