package substore

import (
	"fmt"
	"strings"
)

// SurfboardProducer implements the Producer interface for Surfboard format
type SurfboardProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewSurfboardProducer creates a new Surfboard producer
func NewSurfboardProducer() *SurfboardProducer {
	return &SurfboardProducer{
		producerType: "surfboard",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *SurfboardProducer) GetType() string {
	return p.producerType
}

// Produce converts a single proxy to Surfboard format
// For Surfboard, we expect proxies to be converted one by one
func (p *SurfboardProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	results := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		result, err := p.produceSingle(proxy)
		if err != nil {
			// Skip unsupported proxies if configured
			if opts.IncludeUnsupportedProxy {
				continue
			}
			return nil, err
		}
		results = append(results, result)
	}

	return strings.Join(results, "\n"), nil
}

// produceSingle converts a single proxy to Surfboard format string
func (p *SurfboardProducer) produceSingle(proxy Proxy) (string, error) {
	// Clean proxy name (remove = and , characters)
	name := GetString(proxy, "name")
	name = strings.ReplaceAll(name, "=", "")
	name = strings.ReplaceAll(name, ",", "")
	proxy["name"] = name

	proxyType := p.helper.GetProxyType(proxy)
	switch proxyType {
	case "ss":
		return p.shadowsocks(proxy)
	case "trojan":
		return p.trojan(proxy)
	case "vmess":
		return p.vmess(proxy)
	case "http":
		return p.http(proxy)
	case "socks5":
		return p.socks5(proxy)
	case "wireguard-surge":
		return p.wireguard(proxy)
	default:
		return "", fmt.Errorf("platform Surfboard does not support proxy type: %s", proxyType)
	}
}

// shadowsocks converts a shadowsocks proxy to Surfboard format
func (p *SurfboardProducer) shadowsocks(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	name := GetString(proxy, "name")
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	proxyType := GetString(proxy, "type")

	result.Append(fmt.Sprintf("%s=%s,%s,%d", name, proxyType, server, port))

	// Validate cipher
	cipher := GetString(proxy, "cipher")
	supportedCiphers := map[string]bool{
		"aes-128-gcm":             true,
		"aes-192-gcm":             true,
		"aes-256-gcm":             true,
		"chacha20-ietf-poly1305":  true,
		"xchacha20-ietf-poly1305": true,
		"rc4":                     true,
		"rc4-md5":                 true,
		"aes-128-cfb":             true,
		"aes-192-cfb":             true,
		"aes-256-cfb":             true,
		"aes-128-ctr":             true,
		"aes-192-ctr":             true,
		"aes-256-ctr":             true,
		"bf-cfb":                  true,
		"camellia-128-cfb":        true,
		"camellia-192-cfb":        true,
		"camellia-256-cfb":        true,
		"salsa20":                 true,
		"chacha20":                true,
		"chacha20-ietf":           true,
	}

	if !supportedCiphers[cipher] {
		return "", fmt.Errorf("cipher %s is not supported", cipher)
	}

	result.Append(fmt.Sprintf(",encrypt-method=%s", cipher))

	if IsPresent(proxy, "password") {
		result.Append(fmt.Sprintf(",password=%s", GetString(proxy, "password")))
	}

	// Handle obfs plugin
	if IsPresent(proxy, "plugin") {
		plugin := GetString(proxy, "plugin")
		if plugin == "obfs" {
			pluginOpts := GetMap(proxy, "plugin-opts")
			if pluginOpts != nil {
				result.Append(fmt.Sprintf(",obfs=%s", GetString(pluginOpts, "mode")))

				if IsPresent(pluginOpts, "host") {
					result.Append(fmt.Sprintf(",obfs-host=%s", GetString(pluginOpts, "host")))
				}
				if IsPresent(pluginOpts, "path") {
					result.Append(fmt.Sprintf(",obfs-uri=%s", GetString(pluginOpts, "path")))
				}
			}
		} else {
			return "", fmt.Errorf("plugin %s is not supported", plugin)
		}
	}

	// UDP relay
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	return result.String(), nil
}

// trojan converts a trojan proxy to Surfboard format
func (p *SurfboardProducer) trojan(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	name := GetString(proxy, "name")
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	proxyType := GetString(proxy, "type")

	result.Append(fmt.Sprintf("%s=%s,%s,%d", name, proxyType, server, port))

	if IsPresent(proxy, "password") {
		result.Append(fmt.Sprintf(",password=%s", GetString(proxy, "password")))
	}

	// Transport
	p.handleTransport(result, proxy)

	// TLS
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",tls=%v", GetBool(proxy, "tls")))
	}

	// TLS verification
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// TFO
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",tfo=%v", GetBool(proxy, "tfo")))
	}

	// UDP relay
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	return result.String(), nil
}

// vmess converts a vmess proxy to Surfboard format
func (p *SurfboardProducer) vmess(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	name := GetString(proxy, "name")
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	proxyType := GetString(proxy, "type")

	result.Append(fmt.Sprintf("%s=%s,%s,%d", name, proxyType, server, port))

	if IsPresent(proxy, "uuid") {
		result.Append(fmt.Sprintf(",username=%s", GetString(proxy, "uuid")))
	}

	// Transport
	p.handleTransport(result, proxy)

	// AEAD
	if IsPresent(proxy, "aead") {
		result.Append(fmt.Sprintf(",vmess-aead=%v", GetBool(proxy, "aead")))
	} else {
		alterId := GetInt(proxy, "alterId")
		result.Append(fmt.Sprintf(",vmess-aead=%v", alterId == 0))
	}

	// TLS
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",tls=%v", GetBool(proxy, "tls")))
	}

	// TLS verification
	if IsPresent(proxy, "servernamesni") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// UDP relay
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	return result.String(), nil
}

// http converts an http proxy to Surfboard format
func (p *SurfboardProducer) http(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	name := GetString(proxy, "name")
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	tls := GetBool(proxy, "tls")

	proxyType := "http"
	if tls {
		proxyType = "https"
	}

	result.Append(fmt.Sprintf("%s=%s,%s,%d", name, proxyType, server, port))

	if IsPresent(proxy, "username") {
		result.Append(fmt.Sprintf(",%s", GetString(proxy, "username")))
	}
	if IsPresent(proxy, "password") {
		result.Append(fmt.Sprintf(",%s", GetString(proxy, "password")))
	}

	// TLS verification
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// UDP relay
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	return result.String(), nil
}

// socks5 converts a socks5 proxy to Surfboard format
func (p *SurfboardProducer) socks5(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	name := GetString(proxy, "name")
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	tls := GetBool(proxy, "tls")

	proxyType := "socks5"
	if tls {
		proxyType = "socks5-tls"
	}

	result.Append(fmt.Sprintf("%s=%s,%s,%d", name, proxyType, server, port))

	if IsPresent(proxy, "username") {
		result.Append(fmt.Sprintf(",%s", GetString(proxy, "username")))
	}
	if IsPresent(proxy, "password") {
		result.Append(fmt.Sprintf(",%s", GetString(proxy, "password")))
	}

	// TLS verification
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// UDP relay
	if IsPresent(proxy, "udp") {
		result.Append(fmt.Sprintf(",udp-relay=%v", GetBool(proxy, "udp")))
	}

	return result.String(), nil
}

// wireguard converts a wireguard proxy to Surfboard format
func (p *SurfboardProducer) wireguard(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	name := GetString(proxy, "name")
	result.Append(fmt.Sprintf("%s=wireguard", name))

	if IsPresent(proxy, "section-name") {
		result.Append(fmt.Sprintf(",section-name=%s", GetString(proxy, "section-name")))
	}

	return result.String(), nil
}

// handleTransport handles transport layer configuration
func (p *SurfboardProducer) handleTransport(result *Result, proxy Proxy) {
	if !IsPresent(proxy, "network") {
		return
	}

	network := GetString(proxy, "network")
	if network == "ws" {
		result.Append(",ws=true")

		if IsPresent(proxy, "ws-opts") {
			wsOpts := GetMap(proxy, "ws-opts")
			if wsOpts != nil {
				if IsPresent(wsOpts, "path") {
					result.Append(fmt.Sprintf(",ws-path=%s", GetString(wsOpts, "path")))
				}

				if IsPresent(wsOpts, "headers") {
					headers := GetMap(wsOpts, "headers")
					if headers != nil {
						headerParts := make([]string, 0)
						for k, v := range headers {
							value := fmt.Sprintf("%v", v)
							// Quote Host header value
							if k == "Host" {
								value = fmt.Sprintf(`"%s"`, value)
							}
							headerParts = append(headerParts, fmt.Sprintf("%s:%s", k, value))
						}
						if len(headerParts) > 0 {
							result.Append(fmt.Sprintf(",ws-headers=%s", strings.Join(headerParts, "|")))
						}
					}
				}
			}
		}
	} else {
		// Unsupported network type
		result.Append(fmt.Sprintf(",network=%s", network))
	}
}
