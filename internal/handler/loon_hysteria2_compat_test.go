package handler

import (
	"strings"
	"testing"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
)

func TestNormalizeLoonHysteria2Passwords(t *testing.T) {
	proxies := []substore.Proxy{
		{"type": "hysteria2", "name": "hy2", "server": "example.com", "port": 443, "password": "a%2Bb%3D%40%2F"},
		{"type": "trojan", "password": "leave%40encoded"},
	}

	normalizeLoonHysteria2Passwords(proxies)
	if got := substore.GetString(proxies[0], "password"); got != "a+b=@/" {
		t.Fatalf("HY2 password = %q, want decoded credential", got)
	}
	if got := substore.GetString(proxies[1], "password"); got != "leave%40encoded" {
		t.Fatalf("non-HY2 password changed: %q", got)
	}

	line, err := substore.NewLoonProducer().ProduceOne(proxies[0], "", nil)
	if err != nil {
		t.Fatalf("ProduceOne: %v", err)
	}
	if !strings.Contains(line, `,"a+b=@/"`) {
		t.Fatalf("Loon output still contains encoded password: %q", line)
	}
}

func TestNormalizeLoonHysteria2PasswordsPreservesPlusAndInvalidEscape(t *testing.T) {
	proxies := []substore.Proxy{
		{"type": "hy2", "password": "a+b%2Bc"},
		{"type": "hysteria2", "password": "literal%zz"},
	}
	normalizeLoonHysteria2Passwords(proxies)

	if got := substore.GetString(proxies[0], "password"); got != "a+b+c" {
		t.Fatalf("literal plus was treated as a space: %q", got)
	}
	if got := substore.GetString(proxies[1], "password"); got != "literal%zz" {
		t.Fatalf("invalid escape should be preserved: %q", got)
	}
}
