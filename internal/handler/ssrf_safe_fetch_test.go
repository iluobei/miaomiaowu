package handler

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestValidateFetchURL(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com/a", "http:///missing-host"} {
		if err := validateFetchURL(raw); err == nil {
			t.Fatalf("validateFetchURL(%q) unexpectedly succeeded", raw)
		}
	}
	if err := validateFetchURL("https://example.com/sub"); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
}

func TestSSRFSafeDialBlocksPrivateLiteral(t *testing.T) {
	dial := ssrfSafeDialContext(&net.Dialer{Timeout: time.Second})
	_, err := dial(context.Background(), "tcp", "127.0.0.1:80")
	if !errors.Is(err, errSSRFBlocked) {
		t.Fatalf("expected SSRF rejection, got %v", err)
	}
}
