package handler

import "testing"

// issue #115:mierus:// 分享链接应能解析出正确的端口/凭据/传输协议。
func TestNormalizeMieruShareURI_Issue115(t *testing.T) {
	raw := "mierus://r2RYIiZSxS:GuoNTOfndr@116.126.122.66?handshake-mode=HANDSHAKE_NO_WAIT&mtu=1400&multiplexing=MULTIPLEXING_OFF&port=11211&profile=default&protocol=TCP"

	node, err := ParseProxyURL(raw)
	if err != nil {
		t.Fatalf("解析 mierus:// 失败: %v", err)
	}

	if got := node["type"]; got != "mieru" {
		t.Errorf("type = %v, want mieru", got)
	}
	// 端口在 query 里,规整后必须落到 port 字段,而不是默认的 0。
	switch p := node["port"].(type) {
	case int:
		if p != 11211 {
			t.Errorf("port = %d, want 11211", p)
		}
	default:
		t.Errorf("port 类型/值异常: %#v", node["port"])
	}
	if got := node["username"]; got != "r2RYIiZSxS" {
		t.Errorf("username = %v, want r2RYIiZSxS", got)
	}
	if got := node["password"]; got != "GuoNTOfndr" {
		t.Errorf("password = %v, want GuoNTOfndr", got)
	}
	if got := node["server"]; got != "116.126.122.66" {
		t.Errorf("server = %v, want 116.126.122.66", got)
	}
	if got := node["transport"]; got != "TCP" {
		t.Errorf("transport = %v, want TCP", got)
	}
}

// 非 mierus:// 的输入必须原样透传,不被 shim 破坏。
func TestNormalizeMieruShareURI_Passthrough(t *testing.T) {
	for _, uri := range []string{
		"vmess://abc",
		"mieru://user:pass@host:8080",
		"ss://xxx#name",
	} {
		if got := normalizeMieruShareURI(uri); got != uri {
			t.Errorf("normalizeMieruShareURI(%q) = %q, want 原样返回", uri, got)
		}
	}
}
