package substore

import (
	"fmt"
)

// QXProducer implements QuantumultX format converter
type QXProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewQXProducer creates a new QuantumultX producer
func NewQXProducer() *QXProducer {
	return &QXProducer{
		producerType: "qx",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *QXProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to QuantumultX format
func (p *QXProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	if outputType == "internal" {
		return proxies, nil
	}

	var result []string
	for _, proxy := range proxies {
		line, err := p.produceOne(proxy, outputType, opts)
		if err != nil {
			if !opts.IncludeUnsupportedProxy {
				continue
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}

	output := ""
	for _, line := range result {
		output += line + "\n"
	}
	return output, nil
}

// produceOne converts a single proxy to QuantumultX format
func (p *QXProducer) produceOne(proxy Proxy, _ string, _ *ProduceOptions) (string, error) {
	proxyType := p.helper.GetProxyType(proxy)

	switch proxyType {
	case "ss":
		return p.shadowsocks(proxy)
	case "ssr":
		return p.shadowsocksr(proxy)
	case "trojan":
		return p.trojan(proxy)
	case "vmess":
		return p.vmess(proxy)
	case "http":
		return p.http(proxy)
	case "socks5":
		return p.socks5(proxy)
	case "vless":
		return p.vless(proxy)
	default:
		return "", fmt.Errorf("platform QX does not support proxy type: %s", proxyType)
	}
}

func (p *QXProducer) shadowsocks(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	cipher := GetString(proxy, "cipher")
	if cipher == "" {
		cipher = "none"
	}

	// Validate cipher
	supportedCiphers := []string{
		"none", "rc4-md5", "rc4-md5-6",
		"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
		"aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
		"bf-cfb", "cast5-cfb", "des-cfb", "rc2-cfb",
		"salsa20", "chacha20", "chacha20-ietf",
		"aes-128-gcm", "aes-192-gcm", "aes-256-gcm",
		"chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
		"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm",
	}

	found := false
	for _, c := range supportedCiphers {
		if c == cipher {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("cipher %s is not supported", cipher)
	}

	result.Append(fmt.Sprintf("shadowsocks=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))
	result.Append(fmt.Sprintf(",method=%s", cipher))
	result.Append(fmt.Sprintf(",password=%s", GetString(proxy, "password")))

	// obfs
	if p.needTLS(proxy) {
		proxy["tls"] = true
	}
	if IsPresent(proxy, "plugin") {
		plugin := GetString(proxy, "plugin")
		switch plugin {
		case "obfs":
			pluginOpts := GetMap(proxy, "plugin-opts")
			if pluginOpts != nil {
				result.Append(fmt.Sprintf(",obfs=%s", GetString(pluginOpts, "mode")))
			}
		case "v2ray-plugin":
			pluginOpts := GetMap(proxy, "plugin-opts")
			if pluginOpts != nil && GetString(pluginOpts, "mode") == "websocket" {
				if GetBool(pluginOpts, "tls") {
					result.Append(",obfs=wss")
				} else {
					result.Append(",obfs=ws")
				}
			}
		default:
			return "", fmt.Errorf("plugin is not supported")
		}

		pluginOpts := GetMap(proxy, "plugin-opts")
		if pluginOpts != nil {
			if host := GetString(pluginOpts, "host"); host != "" {
				result.Append(fmt.Sprintf(",obfs-host=%s", host))
			}
			if path := GetString(pluginOpts, "path"); path != "" {
				result.Append(fmt.Sprintf(",obfs-uri=%s", path))
			}
		}
	}

	if p.needTLS(proxy) {
		if val := GetString(proxy, "tls-pubkey-sha256"); val != "" {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", val))
		}
		if val := GetString(proxy, "tls-alpn"); val != "" {
			result.Append(fmt.Sprintf(",tls-alpn=%s", val))
		}
		if IsPresent(proxy, "tls-no-session-ticket") {
			result.Append(fmt.Sprintf(",tls-no-session-ticket=%v", GetBool(proxy, "tls-no-session-ticket")))
		}
		if IsPresent(proxy, "tls-no-session-reuse") {
			result.Append(fmt.Sprintf(",tls-no-session-reuse=%v", GetBool(proxy, "tls-no-session-reuse")))
		}
		// tls fingerprint
		if val := GetString(proxy, "tls-fingerprint"); val != "" {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", val))
		}
		// tls verification
		if IsPresent(proxy, "skip-cert-verify") {
			result.Append(fmt.Sprintf(",tls-verification=%v", !GetBool(proxy, "skip-cert-verify")))
		}
		if val := GetString(proxy, "sni"); val != "" {
			result.Append(fmt.Sprintf(",tls-host=%s", val))
		}
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// udp
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	// udp over tcp
	if GetBool(proxy, "_ssr_python_uot") {
		result.Append(",udp-over-tcp=true")
	} else if GetBool(proxy, "udp-over-tcp") {
		version := GetInt(proxy, "udp-over-tcp-version")
		switch version {
		case 0, 1:
			result.Append(",udp-over-tcp=sp.v1")
		case 2:
			result.Append(",udp-over-tcp=sp.v2")
		}
	}

	// server_check_url
	if val := GetString(proxy, "test-url"); val != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", val))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) shadowsocksr(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	result.Append(fmt.Sprintf("shadowsocks=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))
	result.Append(fmt.Sprintf(",method=%s", GetString(proxy, "cipher")))
	result.Append(fmt.Sprintf(",password=%s", GetString(proxy, "password")))

	// ssr protocol
	result.Append(fmt.Sprintf(",ssr-protocol=%s", GetString(proxy, "protocol")))
	if val := GetString(proxy, "protocol-param"); val != "" {
		result.Append(fmt.Sprintf(",ssr-protocol-param=%s", val))
	}

	// obfs
	if val := GetString(proxy, "obfs"); val != "" {
		result.Append(fmt.Sprintf(",obfs=%s", val))
	}
	if val := GetString(proxy, "obfs-param"); val != "" {
		result.Append(fmt.Sprintf(",obfs-host=%s", val))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// udp
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	// server_check_url
	if val := GetString(proxy, "test-url"); val != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", val))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) trojan(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	result.Append(fmt.Sprintf("trojan=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))
	result.Append(fmt.Sprintf(",password=%s", GetString(proxy, "password")))

	// obfs ws
	if IsPresent(proxy, "network") {
		network := GetString(proxy, "network")
		if network == "ws" {
			if p.needTLS(proxy) {
				result.Append(",obfs=wss")
			} else {
				result.Append(",obfs=ws")
			}

			wsOpts := GetMap(proxy, "ws-opts")
			if wsOpts != nil {
				if path := GetString(wsOpts, "path"); path != "" {
					result.Append(fmt.Sprintf(",obfs-uri=%s", path))
				}
				if headers := GetMap(wsOpts, "headers"); headers != nil {
					if host := GetString(headers, "Host"); host != "" {
						result.Append(fmt.Sprintf(",obfs-host=%s", host))
					}
				}
			}
		} else {
			return "", fmt.Errorf("network %s is unsupported", network)
		}
	}

	// over tls
	if GetString(proxy, "network") != "ws" && p.needTLS(proxy) {
		result.Append(",over-tls=true")
	}

	if p.needTLS(proxy) {
		if val := GetString(proxy, "tls-pubkey-sha256"); val != "" {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", val))
		}
		if val := GetString(proxy, "tls-alpn"); val != "" {
			result.Append(fmt.Sprintf(",tls-alpn=%s", val))
		}
		if IsPresent(proxy, "tls-no-session-ticket") {
			result.Append(fmt.Sprintf(",tls-no-session-ticket=%v", GetBool(proxy, "tls-no-session-ticket")))
		}
		if IsPresent(proxy, "tls-no-session-reuse") {
			result.Append(fmt.Sprintf(",tls-no-session-reuse=%v", GetBool(proxy, "tls-no-session-reuse")))
		}
		// tls fingerprint
		if val := GetString(proxy, "tls-fingerprint"); val != "" {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", val))
		}
		// tls verification
		if IsPresent(proxy, "skip-cert-verify") {
			result.Append(fmt.Sprintf(",tls-verification=%v", !GetBool(proxy, "skip-cert-verify")))
		}
		if val := GetString(proxy, "sni"); val != "" {
			result.Append(fmt.Sprintf(",tls-host=%s", val))
		}
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// udp
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	// server_check_url
	if val := GetString(proxy, "test-url"); val != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", val))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) vmess(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	result.Append(fmt.Sprintf("vmess=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))

	// cipher
	cipher := GetString(proxy, "cipher")
	if cipher == "auto" {
		cipher = "chacha20-ietf-poly1305"
	}
	if cipher == "" {
		cipher = "chacha20-ietf-poly1305"
	}
	result.Append(fmt.Sprintf(",method=%s", cipher))

	result.Append(fmt.Sprintf(",password=%s", GetString(proxy, "uuid")))

	// obfs
	if p.needTLS(proxy) {
		proxy["tls"] = true
	}
	if IsPresent(proxy, "network") {
		network := GetString(proxy, "network")
		switch network {
		case "ws":
			if GetBool(proxy, "tls") {
				result.Append(",obfs=wss")
			} else {
				result.Append(",obfs=ws")
			}
		case "http":
			result.Append(",obfs=http")
		default:
			return "", fmt.Errorf("network %s is unsupported", network)
		}

		// Get transport options
		networkOpts := GetMap(proxy, network+"-opts")
		if networkOpts != nil {
			transportPath := networkOpts["path"]
			var path string
			if pathSlice, ok := transportPath.([]interface{}); ok && len(pathSlice) > 0 {
				path = fmt.Sprintf("%v", pathSlice[0])
			} else if pathStr, ok := transportPath.(string); ok {
				path = pathStr
			}
			if path != "" {
				result.Append(fmt.Sprintf(",obfs-uri=%s", path))
			}

			if headers := GetMap(networkOpts, "headers"); headers != nil {
				transportHost := headers["Host"]
				var host string
				if hostSlice, ok := transportHost.([]interface{}); ok && len(hostSlice) > 0 {
					host = fmt.Sprintf("%v", hostSlice[0])
				} else if hostStr, ok := transportHost.(string); ok {
					host = hostStr
				}
				if host != "" {
					result.Append(fmt.Sprintf(",obfs-host=%s", host))
				}
			}
		}
	} else {
		// over-tls
		if GetBool(proxy, "tls") {
			result.Append(",obfs=over-tls")
		}
	}

	if p.needTLS(proxy) {
		if val := GetString(proxy, "tls-pubkey-sha256"); val != "" {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", val))
		}
		if val := GetString(proxy, "tls-alpn"); val != "" {
			result.Append(fmt.Sprintf(",tls-alpn=%s", val))
		}
		if IsPresent(proxy, "tls-no-session-ticket") {
			result.Append(fmt.Sprintf(",tls-no-session-ticket=%v", GetBool(proxy, "tls-no-session-ticket")))
		}
		if IsPresent(proxy, "tls-no-session-reuse") {
			result.Append(fmt.Sprintf(",tls-no-session-reuse=%v", GetBool(proxy, "tls-no-session-reuse")))
		}
		// tls fingerprint
		if val := GetString(proxy, "tls-fingerprint"); val != "" {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", val))
		}
		// tls verification
		if IsPresent(proxy, "skip-cert-verify") {
			result.Append(fmt.Sprintf(",tls-verification=%v", !GetBool(proxy, "skip-cert-verify")))
		}
		if val := GetString(proxy, "sni"); val != "" {
			result.Append(fmt.Sprintf(",tls-host=%s", val))
		}
	}

	// AEAD
	if IsPresent(proxy, "aead") {
		result.Append(fmt.Sprintf(",aead=%v", GetBool(proxy, "aead")))
	} else {
		result.Append(fmt.Sprintf(",aead=%v", GetInt(proxy, "alterId") == 0))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// udp
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	// server_check_url
	if val := GetString(proxy, "test-url"); val != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", val))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) vless(proxy Proxy) (string, error) {
	if IsPresent(proxy, "flow") || IsPresent(proxy, "reality-opts") {
		return "", fmt.Errorf("VLESS XTLS/REALITY is not supported")
	}

	result := NewResult(proxy)

	result.Append(fmt.Sprintf("vless=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))

	// The method field for vless should be none
	cipher := "none"
	result.Append(fmt.Sprintf(",method=%s", cipher))

	result.Append(fmt.Sprintf(",password=%s", GetString(proxy, "uuid")))

	// obfs
	if p.needTLS(proxy) {
		proxy["tls"] = true
	}
	if IsPresent(proxy, "network") {
		network := GetString(proxy, "network")
		if network == "ws" {
			if GetBool(proxy, "tls") {
				result.Append(",obfs=wss")
			} else {
				result.Append(",obfs=ws")
			}
		} else if network == "http" {
			result.Append(",obfs=http")
		} else if network == "tcp" {
			if GetBool(proxy, "tls") {
				result.Append(",obfs=over-tls")
			}
		} else if network != "tcp" {
			return "", fmt.Errorf("network %s is unsupported", network)
		}

		// Get transport options
		networkOpts := GetMap(proxy, network+"-opts")
		if networkOpts != nil {
			transportPath := networkOpts["path"]
			var path string
			if pathSlice, ok := transportPath.([]interface{}); ok && len(pathSlice) > 0 {
				path = fmt.Sprintf("%v", pathSlice[0])
			} else if pathStr, ok := transportPath.(string); ok {
				path = pathStr
			}
			if path != "" {
				result.Append(fmt.Sprintf(",obfs-uri=%s", path))
			}

			if headers := GetMap(networkOpts, "headers"); headers != nil {
				transportHost := headers["Host"]
				var host string
				if hostSlice, ok := transportHost.([]interface{}); ok && len(hostSlice) > 0 {
					host = fmt.Sprintf("%v", hostSlice[0])
				} else if hostStr, ok := transportHost.(string); ok {
					host = hostStr
				}
				if host != "" {
					result.Append(fmt.Sprintf(",obfs-host=%s", host))
				}
			}
		}
	} else {
		// over-tls
		if GetBool(proxy, "tls") {
			result.Append(",obfs=over-tls")
		}
	}

	if p.needTLS(proxy) {
		if val := GetString(proxy, "tls-pubkey-sha256"); val != "" {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", val))
		}
		if val := GetString(proxy, "tls-alpn"); val != "" {
			result.Append(fmt.Sprintf(",tls-alpn=%s", val))
		}
		if IsPresent(proxy, "tls-no-session-ticket") {
			result.Append(fmt.Sprintf(",tls-no-session-ticket=%v", GetBool(proxy, "tls-no-session-ticket")))
		}
		if IsPresent(proxy, "tls-no-session-reuse") {
			result.Append(fmt.Sprintf(",tls-no-session-reuse=%v", GetBool(proxy, "tls-no-session-reuse")))
		}
		// tls fingerprint
		if val := GetString(proxy, "tls-fingerprint"); val != "" {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", val))
		}
		// tls verification
		if IsPresent(proxy, "skip-cert-verify") {
			result.Append(fmt.Sprintf(",tls-verification=%v", !GetBool(proxy, "skip-cert-verify")))
		}
		if val := GetString(proxy, "sni"); val != "" {
			result.Append(fmt.Sprintf(",tls-host=%s", val))
		}
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// udp
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	// server_check_url
	if val := GetString(proxy, "test-url"); val != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", val))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) http(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	result.Append(fmt.Sprintf("http=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))

	if val := GetString(proxy, "username"); val != "" {
		result.Append(fmt.Sprintf(",username=%s", val))
	}
	if val := GetString(proxy, "password"); val != "" {
		result.Append(fmt.Sprintf(",password=%s", val))
	}

	// tls
	if p.needTLS(proxy) {
		proxy["tls"] = true
	}
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",over-tls=%v", GetBool(proxy, "tls")))
	}

	if p.needTLS(proxy) {
		if val := GetString(proxy, "tls-pubkey-sha256"); val != "" {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", val))
		}
		if val := GetString(proxy, "tls-alpn"); val != "" {
			result.Append(fmt.Sprintf(",tls-alpn=%s", val))
		}
		if IsPresent(proxy, "tls-no-session-ticket") {
			result.Append(fmt.Sprintf(",tls-no-session-ticket=%v", GetBool(proxy, "tls-no-session-ticket")))
		}
		if IsPresent(proxy, "tls-no-session-reuse") {
			result.Append(fmt.Sprintf(",tls-no-session-reuse=%v", GetBool(proxy, "tls-no-session-reuse")))
		}
		// tls fingerprint
		if val := GetString(proxy, "tls-fingerprint"); val != "" {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", val))
		}
		// tls verification
		if IsPresent(proxy, "skip-cert-verify") {
			result.Append(fmt.Sprintf(",tls-verification=%v", !GetBool(proxy, "skip-cert-verify")))
		}
		if val := GetString(proxy, "sni"); val != "" {
			result.Append(fmt.Sprintf(",tls-host=%s", val))
		}
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// udp
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	// server_check_url
	if val := GetString(proxy, "test-url"); val != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", val))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) socks5(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	result.Append(fmt.Sprintf("socks5=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))

	if val := GetString(proxy, "username"); val != "" {
		result.Append(fmt.Sprintf(",username=%s", val))
	}
	if val := GetString(proxy, "password"); val != "" {
		result.Append(fmt.Sprintf(",password=%s", val))
	}

	// tls
	if p.needTLS(proxy) {
		proxy["tls"] = true
	}
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",over-tls=%v", GetBool(proxy, "tls")))
	}

	if p.needTLS(proxy) {
		if val := GetString(proxy, "tls-pubkey-sha256"); val != "" {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", val))
		}
		if val := GetString(proxy, "tls-alpn"); val != "" {
			result.Append(fmt.Sprintf(",tls-alpn=%s", val))
		}
		if IsPresent(proxy, "tls-no-session-ticket") {
			result.Append(fmt.Sprintf(",tls-no-session-ticket=%v", GetBool(proxy, "tls-no-session-ticket")))
		}
		if IsPresent(proxy, "tls-no-session-reuse") {
			result.Append(fmt.Sprintf(",tls-no-session-reuse=%v", GetBool(proxy, "tls-no-session-reuse")))
		}
		// tls fingerprint
		if val := GetString(proxy, "tls-fingerprint"); val != "" {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", val))
		}
		// tls verification
		if IsPresent(proxy, "skip-cert-verify") {
			result.Append(fmt.Sprintf(",tls-verification=%v", !GetBool(proxy, "skip-cert-verify")))
		}
		if val := GetString(proxy, "sni"); val != "" {
			result.Append(fmt.Sprintf(",tls-host=%s", val))
		}
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// udp
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	// server_check_url
	if val := GetString(proxy, "test-url"); val != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", val))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

// needTLS checks if TLS is needed for the proxy
func (p *QXProducer) needTLS(proxy Proxy) bool {
	return GetBool(proxy, "tls")
}
