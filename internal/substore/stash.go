package substore

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// StashProducer implements Stash format converter
type StashProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewStashProducer creates a new Stash producer
func NewStashProducer() *StashProducer {
	return &StashProducer{
		producerType: "stash",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *StashProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to Stash format
func (p *StashProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	supportedSSCiphers := map[string]bool{
		"aes-128-gcm":               true,
		"aes-192-gcm":               true,
		"aes-256-gcm":               true,
		"aes-128-cfb":               true,
		"aes-192-cfb":               true,
		"aes-256-cfb":               true,
		"aes-128-ctr":               true,
		"aes-192-ctr":               true,
		"aes-256-ctr":               true,
		"rc4-md5":                   true,
		"chacha20-ietf":             true,
		"xchacha20":                 true,
		"chacha20-ietf-poly1305":    true,
		"xchacha20-ietf-poly1305":   true,
		"2022-blake3-aes-128-gcm":   true,
		"2022-blake3-aes-256-gcm":   true,
	}

	supportedVMessCiphers := map[string]bool{
		"auto":               true,
		"aes-128-gcm":        true,
		"chacha20-poly1305":  true,
		"none":               true,
	}

	var result []Proxy
	for _, proxy := range proxies {
		proxyType := p.helper.GetProxyType(proxy)

		// Filter unsupported types
		shouldSkip := false

		// Check supported types
		if !p.isSupportedType(proxyType) {
			shouldSkip = true
		}

		// Check SS cipher
		if proxyType == "ss" {
			cipher := GetString(proxy, "cipher")
			if !supportedSSCiphers[cipher] {
				shouldSkip = true
			}
		}

		// Check Snell version
		if proxyType == "snell" && GetInt(proxy, "version") >= 4 {
			shouldSkip = true
		}

		// Check VLESS reality
		if proxyType == "vless" && IsPresent(proxy, "reality-opts") {
			flow := GetString(proxy, "flow")
			if flow != "xtls-rprx-vision" {
				shouldSkip = true
			}
		}

		// Check underlying-proxy / dialer-proxy
		if IsPresent(proxy, "underlying-proxy") || IsPresent(proxy, "dialer-proxy") {
			shouldSkip = true
		}

		if shouldSkip {
			continue
		}

		transformed := p.helper.CloneProxy(proxy)

		// VMess transformations
		if proxyType == "vmess" {
			// Handle aead
			if IsPresent(transformed, "aead") {
				if GetBool(transformed, "aead") {
					transformed["alterId"] = 0
				}
				delete(transformed, "aead")
			}

			// sni -> servername
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
			} else {
				transformed["alpn"] = []string{"h3"}
			}

			// tfo -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
				delete(transformed, "tfo")
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

			// tfo -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
				delete(transformed, "tfo")
			}

			// down -> down-speed
			if IsPresent(transformed, "down") && !IsPresent(transformed, "down-speed") {
				transformed["down-speed"] = GetString(transformed, "down")
				delete(transformed, "down")
			}

			// up -> up-speed
			if IsPresent(transformed, "up") && !IsPresent(transformed, "up-speed") {
				transformed["up-speed"] = GetString(transformed, "up")
				delete(transformed, "up")
			}

			// Extract numeric values from down-speed and up-speed
			if IsPresent(transformed, "down-speed") {
				downSpeed := fmt.Sprintf("%v", transformed["down-speed"])
				re := regexp.MustCompile(`\d+`)
				if match := re.FindString(downSpeed); match != "" {
					transformed["down-speed"] = match
				} else {
					transformed["down-speed"] = "0"
				}
			}

			if IsPresent(transformed, "up-speed") {
				upSpeed := fmt.Sprintf("%v", transformed["up-speed"])
				re := regexp.MustCompile(`\d+`)
				if match := re.FindString(upSpeed); match != "" {
					transformed["up-speed"] = match
				} else {
					transformed["up-speed"] = "0"
				}
			}
		}

		// Hysteria2 transformations
		if proxyType == "hysteria2" {
			// password -> auth
			if IsPresent(transformed, "password") && !IsPresent(transformed, "auth") {
				transformed["auth"] = GetString(transformed, "password")
				delete(transformed, "password")
			}

			// tfo -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
				delete(transformed, "tfo")
			}

			// down -> down-speed
			if IsPresent(transformed, "down") && !IsPresent(transformed, "down-speed") {
				transformed["down-speed"] = GetString(transformed, "down")
				delete(transformed, "down")
			}

			// up -> up-speed
			if IsPresent(transformed, "up") && !IsPresent(transformed, "up-speed") {
				transformed["up-speed"] = GetString(transformed, "up")
				delete(transformed, "up")
			}

			// Extract numeric values
			if IsPresent(transformed, "down-speed") {
				downSpeed := fmt.Sprintf("%v", transformed["down-speed"])
				re := regexp.MustCompile(`\d+`)
				if match := re.FindString(downSpeed); match != "" {
					transformed["down-speed"] = match
				} else {
					transformed["down-speed"] = "0"
				}
			}

			if IsPresent(transformed, "up-speed") {
				upSpeed := fmt.Sprintf("%v", transformed["up-speed"])
				re := regexp.MustCompile(`\d+`)
				if match := re.FindString(upSpeed); match != "" {
					transformed["up-speed"] = match
				} else {
					transformed["up-speed"] = "0"
				}
			}
		}

		// WireGuard transformations
		if proxyType == "wireguard" {
			keepalive := GetInt(transformed, "keepalive")
			if keepalive == 0 {
				keepalive = GetInt(transformed, "persistent-keepalive")
			}
			transformed["keepalive"] = keepalive
			transformed["persistent-keepalive"] = keepalive

			presharedKey := GetString(transformed, "preshared-key")
			if presharedKey == "" {
				presharedKey = GetString(transformed, "pre-shared-key")
			}
			transformed["preshared-key"] = presharedKey
			transformed["pre-shared-key"] = presharedKey
		}

		// Snell transformations
		if proxyType == "snell" && GetInt(transformed, "version") < 3 {
			delete(transformed, "udp")
		}

		// VLESS transformations
		if proxyType == "vless" {
			// sni -> servername
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

		// Handle WebSocket early data
		if network == "ws" {
			networkOpts := GetMap(transformed, "ws-opts")
			if networkOpts != nil {
				if path := GetString(networkOpts, "path"); path != "" {
					re := regexp.MustCompile(`^(.*?)(?:\?ed=(\d+))?$`)
					matches := re.FindStringSubmatch(path)
					if len(matches) > 1 {
						cleanPath := matches[1]
						networkOpts["path"] = cleanPath

						if len(matches) > 2 && matches[2] != "" {
							networkOpts["early-data-header-name"] = "Sec-WebSocket-Protocol"
							edValue := 0
							fmt.Sscanf(matches[2], "%d", &edValue)
							networkOpts["max-early-data"] = edValue
						}
					}
				} else {
					networkOpts["path"] = "/"
				}
			} else {
				transformed["ws-opts"] = map[string]interface{}{
					"path": "/",
				}
			}
		}

		// Handle plugin-opts TLS
		if pluginOpts := GetMap(transformed, "plugin-opts"); pluginOpts != nil {
			if GetBool(pluginOpts, "tls") && IsPresent(transformed, "skip-cert-verify") {
				pluginOpts["skip-cert-verify"] = GetBool(transformed, "skip-cert-verify")
			}
		}

		// Delete tls for certain types
		if p.shouldDeleteTLS(proxyType) {
			delete(transformed, "tls")
		}

		// tls-fingerprint -> server-cert-fingerprint
		if IsPresent(transformed, "tls-fingerprint") {
			transformed["server-cert-fingerprint"] = GetString(transformed, "tls-fingerprint")
		}
		delete(transformed, "tls-fingerprint")

		// Remove non-boolean tls
		if IsPresent(transformed, "tls") {
			if _, ok := transformed["tls"].(bool); !ok {
				delete(transformed, "tls")
			}
		}

		// test-url -> benchmark-url
		if IsPresent(transformed, "test-url") {
			transformed["benchmark-url"] = GetString(transformed, "test-url")
			delete(transformed, "test-url")
		}

		// test-timeout -> benchmark-timeout
		if IsPresent(transformed, "test-timeout") {
			transformed["benchmark-timeout"] = GetInt(transformed, "test-timeout")
			delete(transformed, "test-timeout")
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

func (p *StashProducer) isSupportedType(proxyType string) bool {
	supportedTypes := []string{
		"ss", "ssr", "vmess", "socks5", "http", "snell",
		"trojan", "tuic", "vless", "wireguard",
		"hysteria", "hysteria2", "ssh", "juicity",
	}

	for _, t := range supportedTypes {
		if t == proxyType {
			return true
		}
	}
	return false
}

func (p *StashProducer) shouldDeleteTLS(proxyType string) bool {
	deleteTLSTypes := []string{
		"trojan", "tuic", "hysteria", "hysteria2", "juicity", "anytls",
	}

	for _, t := range deleteTLSTypes {
		if t == proxyType {
			return true
		}
	}
	return false
}
