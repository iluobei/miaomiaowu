package handler

import (
	"net/http/httptest"
	"testing"
)

func TestDetectClientTypeFromUA(t *testing.T) {
	for ua, want := range map[string]string{
		"Stash/2.5 Clash/1.9": "stash",
		"Quantumult X/1.5":    "qx",
		"v2rayNG/1.9":         "v2ray",
		"Mozilla/5.0":         "",
	} {
		if got := detectClientTypeFromUA(ua); got != want {
			t.Errorf("detectClientTypeFromUA(%q)=%q, want %q", ua, got, want)
		}
	}
}

func TestResolveClientTypeAuto(t *testing.T) {
	for ua, want := range map[string]string{
		"Stash/2.5 Clash/1.9": "stash",
		"Quantumult X/1.5":    "qx",
		"Mozilla/5.0":         "",
	} {
		req := httptest.NewRequest("GET", "/sub?t=auto", nil)
		req.Header.Set("User-Agent", ua)
		if got := resolveClientType(req); got != want {
			t.Errorf("resolveClientType(%q)=%q, want %q", ua, got, want)
		}
	}
	req := httptest.NewRequest("GET", "/sub?t=surge", nil)
	if got := resolveClientType(req); got != "surge" {
		t.Fatalf("explicit client type changed to %q", got)
	}
}

func TestSubscriptionUAGuard(t *testing.T) {
	SetBlockUnknownSubscriptionUA(true)
	t.Cleanup(func() { SetBlockUnknownSubscriptionUA(false) })
	req := httptest.NewRequest("GET", "/sub", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp := httptest.NewRecorder()
	if !rejectBlockedSubscriptionUA(resp, req) || resp.Code != 403 {
		t.Fatalf("unknown UA should be rejected, status=%d", resp.Code)
	}
}
