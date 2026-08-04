package handler

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
)

func TestPinnedPeerCertSha256V2RayRoundTrip(t *testing.T) {
	const fingerprint = "0123456789ABCDEF"
	node, err := ParseProxyURL("vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&pinnedPeerCertSha256=" + fingerprint + "#pinned")
	if err != nil {
		t.Fatalf("parse VLESS URI: %v", err)
	}
	if got := node["tls-fingerprint"]; got != fingerprint {
		t.Fatalf("tls-fingerprint = %v, want %q", got, fingerprint)
	}

	producer, err := substore.GetDefaultFactory().GetProducer("v2ray")
	if err != nil {
		t.Fatalf("get V2Ray producer: %v", err)
	}
	result, err := producer.Produce([]substore.Proxy{node}, "", nil)
	if err != nil {
		t.Fatalf("produce V2Ray subscription: %v", err)
	}
	encoded, ok := result.(string)
	if !ok {
		t.Fatalf("V2Ray result type = %T, want string", result)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode V2Ray subscription: %v", err)
	}
	uri, err := url.Parse(strings.TrimSpace(string(decoded)))
	if err != nil {
		t.Fatalf("parse generated VLESS URI: %v", err)
	}
	if got := uri.Query().Get("pcs"); got != fingerprint {
		t.Fatalf("generated pcs = %q, want %q; URI: %s", got, fingerprint, uri)
	}
}

func TestAnyTLSStashAndQuantumultXOutput(t *testing.T) {
	proxy := substore.Proxy{
		"name": "anytls-test", "type": "anytls", "server": "example.com",
		"port": 443, "password": "secret", "sni": "example.com",
	}
	for _, target := range []struct {
		name string
		want string
	}{
		{"stash", `"type":"anytls"`},
		{"qx", "anytls=example.com:443"},
	} {
		producer, err := substore.GetDefaultFactory().GetProducer(target.name)
		if err != nil {
			t.Fatalf("get %s producer: %v", target.name, err)
		}
		result, err := producer.Produce([]substore.Proxy{proxy}, "", nil)
		if err != nil {
			t.Fatalf("produce %s: %v", target.name, err)
		}
		if !strings.Contains(result.(string), target.want) {
			t.Fatalf("%s output omitted AnyTLS node: %s", target.name, result)
		}
	}
}
