package substore

import (
	"encoding/json"
	"strings"
)

// ShadowrocketProducer implements Shadowrocket format converter
type ShadowrocketProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewShadowrocketProducer creates a new Shadowrocket producer
func NewShadowrocketProducer() *ShadowrocketProducer {
	return &ShadowrocketProducer{
		producerType: "shadowrocket",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *ShadowrocketProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to Shadowrocket format
func (p *ShadowrocketProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	supportedVMessCiphers := map[string]bool{
		"auto": true, "none": true, "zero": true,
		"aes-128-gcm": true, "chacha20-poly1305": true,
	}

	// Filter and transform proxies
	var result []Proxy
	for _, proxy := range proxies {
		proxyType := p.helper.GetProxyType(proxy)

		// Filter unsupported types
		if !opts.IncludeUnsupportedProxy {
			if proxyType == "snell" && GetInt(proxy, "version") >= 4 {
				continue
			}
			if proxyType == "mieru" {
				continue
			}
		}

		transformed := p.helper.CloneProxy(proxy)

		// VMess specific transformations
		if proxyType == "vmess" {
			// Handle aead
			if IsPresent(transformed, "aead") {
				if GetBool(transformed, "aead") {
					transformed["alterId"] = 0
				}
				delete(transformed, "aead")
			}

			// SNI -> servername
			if IsPresent(transformed, "sni") {
				transformed["servername"] = GetString(transformed, "sni")
				delete(transformed, "sni")
			}

			// Cipher validation
			if IsPresent(transformed, "cipher") {
				cipher := GetString(transformed, "cipher")
				if !supportedVMessCiphers[cipher] {
					transformed["cipher"] = "auto"
				}
			}
		}

		// TUIC transformations
		if proxyType == "tuic" {
			// Ensure alpn is array
			if IsPresent(transformed, "alpn") {
				alpnVal := transformed["alpn"]
				if alpnSlice, ok := alpnVal.([]interface{}); ok {
					transformed["alpn"] = alpnSlice
				} else if alpnStr, ok := alpnVal.(string); ok {
					transformed["alpn"] = []string{alpnStr}
				}
			}

			// TFO -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
			}

			// Default version
			token := GetString(transformed, "token")
			if token == "" && !IsPresent(transformed, "version") {
				transformed["version"] = 5
			}
		}

		// Hysteria transformations
		if proxyType == "hysteria" {
			// auth_str -> auth-str
			if IsPresent(transformed, "auth_str") && !IsPresent(transformed, "auth-str") {
				transformed["auth-str"] = GetString(transformed, "auth_str")
			}

			// Ensure alpn is array
			if IsPresent(transformed, "alpn") {
				alpnVal := transformed["alpn"]
				if alpnSlice, ok := alpnVal.([]interface{}); ok {
					transformed["alpn"] = alpnSlice
				} else if alpnStr, ok := alpnVal.(string); ok {
					transformed["alpn"] = []string{alpnStr}
				}
			}

			// TFO -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
			}
		}

		// Hysteria2 transformations
		if proxyType == "hysteria2" {
			// Ensure alpn is array
			if IsPresent(transformed, "alpn") {
				alpnVal := transformed["alpn"]
				if alpnSlice, ok := alpnVal.([]interface{}); ok {
					transformed["alpn"] = alpnSlice
				} else if alpnStr, ok := alpnVal.(string); ok {
					transformed["alpn"] = []string{alpnStr}
				}
			}

			// TFO -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
			}
		}

		// WireGuard transformations
		if proxyType == "wireguard" {
			// Keepalive
			if !IsPresent(transformed, "keepalive") && IsPresent(transformed, "persistent-keepalive") {
				transformed["keepalive"] = GetInt(transformed, "persistent-keepalive")
			}
			transformed["persistent-keepalive"] = GetInt(transformed, "keepalive")

			// Preshared key
			if !IsPresent(transformed, "preshared-key") && IsPresent(transformed, "pre-shared-key") {
				transformed["preshared-key"] = GetString(transformed, "pre-shared-key")
			}
			transformed["pre-shared-key"] = GetString(transformed, "preshared-key")
		}

		// Snell transformations
		if proxyType == "snell" {
			version := GetInt(transformed, "version")
			if version < 3 {
				delete(transformed, "udp")
			}
		}

		// VLESS transformations
		if proxyType == "vless" {
			// SNI -> servername
			if IsPresent(transformed, "sni") {
				transformed["servername"] = GetString(transformed, "sni")
				delete(transformed, "sni")
			}
		}

		// Handle HTTP network options for VMess/VLESS
		network := GetString(transformed, "network")
		if (proxyType == "vmess" || proxyType == "vless") && network == "http" {
			if httpOpts := GetMap(transformed, "http-opts"); httpOpts != nil {
				// Ensure path is array
				if IsPresent(transformed, "http-opts", "path") {
					if path, ok := httpOpts["path"].(string); ok {
						httpOpts["path"] = []string{path}
					}
				}

				// Ensure headers.Host is array
				if headers := GetMap(httpOpts, "headers"); headers != nil {
					if IsPresent(transformed, "http-opts", "headers", "Host") {
						if host, ok := headers["Host"].(string); ok {
							headers["Host"] = []string{host}
						}
					}
				}
			}
		}

		// 处理xhttp参数
		if proxyType == "vless" && network == "xhttp" {
			if xhttpOpts := GetMap(transformed, "xhttp-opts"); xhttpOpts != nil {
				transformed["obfs"] = network
				if IsPresent(transformed, "xhttp-opts", "path") {
					if path, ok := xhttpOpts["path"].(string); ok {
						transformed["path"] = path
					}
				}

				if headers := GetMap(xhttpOpts, "headers"); headers != nil {
					if IsPresent(transformed, "xhttp-opts", "headers", "Host") {
						if host, ok := headers["Host"].(string); ok {
							transformed["obfsParam"] = host
						}
					}
				}
			}
		}

		// 兼容0.3.7后续版本xhttp改为splithttp的情况
		if proxyType == "vless" && network == "splithttp" {
			transformed["network"] = "xhttp"
			transformed["obfs"] = "xhttp"
			if splithttpOpts := GetMap(transformed, "splithttp-opts"); splithttpOpts != nil {
				if IsPresent(transformed, "splithttp-opts", "path") {
					if path, ok := splithttpOpts["path"].(string); ok {
						transformed["path"] = path
					}
				}

				if headers := GetMap(splithttpOpts, "headers"); headers != nil {
					if IsPresent(transformed, "splithttp-opts", "headers", "Host") {
						if host, ok := headers["Host"].(string); ok {
							transformed["obfsParam"] = host
						}
					}
				}
			}
		}

		// Handle H2 network options
		if (proxyType == "vmess" || proxyType == "vless") && network == "h2" {
			if h2Opts := GetMap(transformed, "h2-opts"); h2Opts != nil {
				// Ensure path is string (take first element if array)
				if IsPresent(transformed, "h2-opts", "path") {
					if pathSlice, ok := h2Opts["path"].([]interface{}); ok && len(pathSlice) > 0 {
						h2Opts["path"] = pathSlice[0]
					}
				}

				// Ensure host is array
				if headers := GetMap(h2Opts, "headers"); headers != nil {
					if IsPresent(transformed, "h2-opts", "headers", "Host") {
						if host, ok := headers["Host"].(string); ok {
							headers["host"] = []string{host}
						}
					}
				}
			}
		}

		// Handle plugin-opts TLS
		if pluginOpts := GetMap(transformed, "plugin-opts"); pluginOpts != nil {
			if GetBool(pluginOpts, "tls") && IsPresent(transformed, "skip-cert-verify") {
				pluginOpts["skip-cert-verify"] = GetBool(transformed, "skip-cert-verify")
			}
		}

		// Clean up fields
		p.helper.RemoveProxyFields(transformed,
			"subName", "collectionName", "id", "resolved", "no-resolve")

		// Remove null and underscore-prefixed fields for non-internal output
		if outputType != "internal" {
			for key := range transformed {
				if transformed[key] == nil || strings.HasPrefix(key, "_") {
					delete(transformed, key)
				}
			}
		}

		// Clean up grpc options
		if network == "grpc" {
			if grpcOpts := GetMap(transformed, "grpc-opts"); grpcOpts != nil {
				delete(grpcOpts, "_grpc-type")
				delete(grpcOpts, "_grpc-authority")
			}
		}

		result = append(result, transformed)
	}

	// Return based on output type
	if outputType == "internal" {
		return result, nil
	}

	// Generate YAML string
	var sb strings.Builder
	sb.WriteString("proxies:\n")
	for _, proxy := range result {
		jsonBytes, err := json.Marshal(proxy)
		if err != nil {
			continue
		}
		sb.WriteString("  - ")
		sb.Write(jsonBytes)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
