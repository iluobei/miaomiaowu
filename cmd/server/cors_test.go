package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseAllowedOrigins(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect []string
	}{
		{"empty defaults", "", []string{"*"}},
		{"single", "http://example.com", []string{"http://example.com"}},
		{"multiple", "http://a.com, http://b.com", []string{"http://a.com", "http://b.com"}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAllowedOrigins(tt.input)
			if len(got) != len(tt.expect) {
				t.Fatalf("unexpected length: got %v want %v", got, tt.expect)
			}
			for i, origin := range tt.expect {
				if got[i] != origin {
					t.Fatalf("unexpected origin at %d: got %s want %s", i, got[i], origin)
				}
			}
		})
	}
}

func TestWithCORSAllowsOrigin(t *testing.T) {
	allowed := []string{"http://localhost:5173"}
	called := false

	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}), allowed)

	req := httptest.NewRequest(http.MethodGet, "/api/login", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if !called {
		t.Fatalf("expected handler to be called")
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("unexpected header: %s", got)
	}
}

func TestWithCORSHandlesPreflight(t *testing.T) {
	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("should not reach next handler on preflight")
	}), []string{"*"})

	req := httptest.NewRequest(http.MethodOptions, "/any", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("unexpected origin header: %s", got)
	}
}
