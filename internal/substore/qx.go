package substore

import (
	"fmt"
	"strings"
)

const targetPlatformQX = "QX"

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

	return strings.Join(result, "\n") + "\n", nil
}

// produceOne converts a single proxy to QuantumultX format
func (p *QXProducer) produceOne(proxy Proxy, _ string, _ *ProduceOptions) (string, error) {
	// Check for unsupported ws network with v2ray-http-upgrade
	if GetString(proxy, "network") == "ws" {
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if GetBool(wsOpts, "v2ray-http-upgrade") {
				return "", fmt.Errorf("platform %s does not support network %s with http upgrade", targetPlatformQX, "ws")
			}
		}
	}

	proxyType := p.helper.GetProxyType(proxy)

	var result string
	var err error

	switch proxyType {
	case "ss":
		result, err = p.shadowsocks(proxy)
	case "ssr":
		result, err = p.shadowsocksr(proxy)
	case "trojan":
		result, err = p.trojan(proxy)
	case "vmess":
		result, err = p.vmess(proxy)
	case "http":
		result, err = p.http(proxy)
	case "socks5":
		result, err = p.socks5(proxy)
	case "vless":
		result, err = p.vless(proxy)
	default:
		return "", fmt.Errorf("platform %s does not support proxy type: %s", targetPlatformQX, proxyType)
	}

	if err != nil {
		return "", err
	}

	// Handle flow validation (after producing the base result)
	if IsPresent(proxy, "flow") {
		flow := GetString(proxy, "flow")
		if flow != "" && flow != "xtls-rprx-vision" {
			return "", fmt.Errorf("platform %s does not support flow %s", targetPlatformQX, flow)
		}
	}

	// Handle reality-opts (append to result)
	if realityOpts := GetMap(proxy, "reality-opts"); realityOpts != nil {
		if publicKey := GetString(realityOpts, "public-key"); publicKey != "" {
			result = fmt.Sprintf("%s,reality-base64-pubkey=%s", result, publicKey)
		}
		if shortID := GetAnyString(realityOpts, "short-id"); shortID != "" {
			result = fmt.Sprintf("%s,reality-hex-shortid=%s", result, shortID)
		}
	}

	return result, nil
}

func (p *QXProducer) shadowsocks(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	cipher := GetString(proxy, "cipher")
	if cipher == "" {
		cipher = "none"
	}

	// Validate cipher
	supportedCiphers := map[string]bool{
		"none": true, "rc4-md5": true, "rc4-md5-6": true,
		"aes-128-cfb": true, "aes-192-cfb": true, "aes-256-cfb": true,
		"aes-128-ctr": true, "aes-192-ctr": true, "aes-256-ctr": true,
		"bf-cfb": true, "cast5-cfb": true, "des-cfb": true, "rc2-cfb": true,
		"salsa20": true, "chacha20": true, "chacha20-ietf": true,
		"aes-128-gcm": true, "aes-192-gcm": true, "aes-256-gcm": true,
		"chacha20-ietf-poly1305": true, "xchacha20-ietf-poly1305": true,
		"2022-blake3-aes-128-gcm": true, "2022-blake3-aes-256-gcm": true,
	}

	if !supportedCiphers[cipher] {
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
		p.appendTLSOptions(result, proxy)
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
		if version == 0 || version == 1 {
			result.Append(",udp-over-tcp=sp.v1")
		} else if version == 2 {
			result.Append(",udp-over-tcp=sp.v2")
		}
	}

	// server_check_url
	if testURL := GetString(proxy, "test-url"); testURL != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", testURL))
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
	if protocolParam := GetString(proxy, "protocol-param"); protocolParam != "" {
		result.Append(fmt.Sprintf(",ssr-protocol-param=%s", protocolParam))
	}

	// obfs
	if obfs := GetString(proxy, "obfs"); obfs != "" {
		result.Append(fmt.Sprintf(",obfs=%s", obfs))
	}
	if obfsParam := GetString(proxy, "obfs-param"); obfsParam != "" {
		result.Append(fmt.Sprintf(",obfs-host=%s", obfsParam))
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
	if testURL := GetString(proxy, "test-url"); testURL != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", testURL))
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
		} else if network != "tcp" {
			return "", fmt.Errorf("network %s is unsupported", network)
		}
	}

	// over tls (when not using ws network)
	if GetString(proxy, "network") != "ws" && p.needTLS(proxy) {
		result.Append(",over-tls=true")
	}

	if p.needTLS(proxy) {
		p.appendTLSOptions(result, proxy)
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
	if testURL := GetString(proxy, "test-url"); testURL != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", testURL))
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
		case "tcp":
			// tcp network is supported, no obfs needed unless tls
		default:
			return "", fmt.Errorf("network %s is unsupported", network)
		}

		// Get transport options
		networkOpts := GetMap(proxy, network+"-opts")
		if networkOpts != nil {
			transportPath := networkOpts["path"]
			path := p.getFirstStringValue(transportPath)
			if path != "" {
				result.Append(fmt.Sprintf(",obfs-uri=%s", path))
			}

			if headers := GetMap(networkOpts, "headers"); headers != nil {
				transportHost := headers["Host"]
				host := p.getFirstStringValue(transportHost)
				if host != "" {
					result.Append(fmt.Sprintf(",obfs-host=%s", host))
				}
			}
		}
	} else {
		// over-tls (when no network specified)
		if GetBool(proxy, "tls") {
			result.Append(",obfs=over-tls")
		}
	}

	if p.needTLS(proxy) {
		p.appendTLSOptions(result, proxy)
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
	if testURL := GetString(proxy, "test-url"); testURL != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", testURL))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) vless(proxy Proxy) (string, error) {
	// Check encryption
	if encryption := GetString(proxy, "encryption"); encryption != "" && encryption != "none" {
		return "", fmt.Errorf("VLESS encryption is not supported")
	}

	result := NewResult(proxy)

	result.Append(fmt.Sprintf("vless=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))

	// The method field for vless should be none
	result.Append(",method=none")

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
		case "tcp":
			if GetBool(proxy, "tls") {
				result.Append(",obfs=over-tls")
			}
		default:
			return "", fmt.Errorf("network %s is unsupported", network)
		}

		// Get transport options
		networkOpts := GetMap(proxy, network+"-opts")
		if networkOpts != nil {
			transportPath := networkOpts["path"]
			path := p.getFirstStringValue(transportPath)
			if path != "" {
				result.Append(fmt.Sprintf(",obfs-uri=%s", path))
			}

			if headers := GetMap(networkOpts, "headers"); headers != nil {
				transportHost := headers["Host"]
				host := p.getFirstStringValue(transportHost)
				if host != "" {
					result.Append(fmt.Sprintf(",obfs-host=%s", host))
				}
			}
		}
	} else {
		// over-tls (when no network specified)
		if GetBool(proxy, "tls") {
			result.Append(",obfs=over-tls")
		}
	}

	if p.needTLS(proxy) {
		p.appendTLSOptions(result, proxy)
	}

	// vless-flow
	if flow := GetString(proxy, "flow"); flow != "" {
		result.Append(fmt.Sprintf(",vless-flow=%s", flow))
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
	if testURL := GetString(proxy, "test-url"); testURL != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", testURL))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) http(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	result.Append(fmt.Sprintf("http=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))

	if username := GetString(proxy, "username"); username != "" {
		result.Append(fmt.Sprintf(",username=%s", username))
	}
	if password := GetString(proxy, "password"); password != "" {
		result.Append(fmt.Sprintf(",password=%s", password))
	}

	// tls
	if p.needTLS(proxy) {
		proxy["tls"] = true
	}
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",over-tls=%v", GetBool(proxy, "tls")))
	}

	if p.needTLS(proxy) {
		p.appendTLSOptions(result, proxy)
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
	if testURL := GetString(proxy, "test-url"); testURL != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", testURL))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

func (p *QXProducer) socks5(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	result.Append(fmt.Sprintf("socks5=%s:%d", GetString(proxy, "server"), GetInt(proxy, "port")))

	if username := GetString(proxy, "username"); username != "" {
		result.Append(fmt.Sprintf(",username=%s", username))
	}
	if password := GetString(proxy, "password"); password != "" {
		result.Append(fmt.Sprintf(",password=%s", password))
	}

	// tls
	if p.needTLS(proxy) {
		proxy["tls"] = true
	}
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",over-tls=%v", GetBool(proxy, "tls")))
	}

	if p.needTLS(proxy) {
		p.appendTLSOptions(result, proxy)
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
	if testURL := GetString(proxy, "test-url"); testURL != "" {
		result.Append(fmt.Sprintf(",server_check_url=%s", testURL))
	}

	// tag
	result.Append(fmt.Sprintf(",tag=%s", GetString(proxy, "name")))

	return result.String(), nil
}

// needTLS checks if TLS is needed for the proxy
func (p *QXProducer) needTLS(proxy Proxy) bool {
	return GetBool(proxy, "tls")
}

// appendTLSOptions appends common TLS options to the result
func (p *QXProducer) appendTLSOptions(result *Result, proxy Proxy) {
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

	// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
	if sni := GetSNI(proxy); sni != "" {
		result.Append(fmt.Sprintf(",tls-host=%s", sni))
	}
}

// getFirstStringValue extracts the first string value from a value that could be a string or array
func (p *QXProducer) getFirstStringValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			return fmt.Sprintf("%v", v[0])
		}
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return ""
}
