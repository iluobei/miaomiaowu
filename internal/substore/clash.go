package substore

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ClashProducer implements the Producer interface for Clash format
type ClashProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewClashProducer creates a new Clash producer
func NewClashProducer() *ClashProducer {
	return &ClashProducer{
		producerType: "clash",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *ClashProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to Clash format
func (p *ClashProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	// Supported ciphers for Shadowsocks
	supportedSSCiphers := map[string]bool{
		"aes-128-gcm":              true,
		"aes-192-gcm":              true,
		"aes-256-gcm":              true,
		"aes-128-cfb":              true,
		"aes-192-cfb":              true,
		"aes-256-cfb":              true,
		"aes-128-ctr":              true,
		"aes-192-ctr":              true,
		"aes-256-ctr":              true,
		"rc4-md5":                  true,
		"chacha20-ietf":            true,
		"xchacha20":                true,
		"chacha20-ietf-poly1305":   true,
		"xchacha20-ietf-poly1305":  true,
	}

	// Supported VMess ciphers
	supportedVMessCiphers := map[string]bool{
		"auto":             true,
		"aes-128-gcm":      true,
		"chacha20-poly1305": true,
		"none":             true,
	}

	// Filter proxies
	filtered := make([]Proxy, 0)
	for _, proxy := range proxies {
		proxyType := p.helper.GetProxyType(proxy)

		// Skip if include-unsupported-proxy is not set
		if !opts.IncludeUnsupportedProxy {
			// Check supported proxy types
			supportedTypes := map[string]bool{
				"ss": true, "ssr": true, "vmess": true, "vless": true,
				"socks5": true, "http": true, "snell": true, "trojan": true,
				"wireguard": true,
			}

			if !supportedTypes[proxyType] {
				continue
			}

			// Check SS cipher
			if proxyType == "ss" {
				cipher := GetString(proxy, "cipher")
				if !supportedSSCiphers[cipher] {
					continue
				}
			}

			// Check Snell version
			if proxyType == "snell" {
				version := GetInt(proxy, "version")
				if version >= 4 {
					continue
				}
			}

			// Check VLESS flow and reality
			if proxyType == "vless" {
				if IsPresent(proxy, "flow") || IsPresent(proxy, "reality-opts") {
					continue
				}
			}

			// Check dialer proxy
			if IsPresent(proxy, "underlying-proxy") || IsPresent(proxy, "dialer-proxy") {
				// Clash doesn't support dialer proxy, skip this node
				continue
			}
		}

		filtered = append(filtered, proxy)
	}

	// Transform proxies
	result := make([]Proxy, 0)
	for _, proxy := range filtered {
		transformed := p.helper.CloneProxy(proxy)
		proxyType := p.helper.GetProxyType(transformed)

		// Type-specific transformations
		switch proxyType {
		case "vmess":
			// Handle aead
			if IsPresent(transformed, "aead") {
				if GetBool(transformed, "aead") {
					transformed["alterId"] = 0
				}
				delete(transformed, "aead")
			}

			// Handle sni -> servername
			if IsPresent(transformed, "sni") {
				transformed["servername"] = GetString(transformed, "sni")
				delete(transformed, "sni")
			}

			// Handle cipher
			if IsPresent(transformed, "cipher") {
				cipher := GetString(transformed, "cipher")
				if !supportedVMessCiphers[cipher] {
					transformed["cipher"] = "auto"
				}
			}

		case "wireguard":
			// WireGuard keepalive
			if !IsPresent(transformed, "keepalive") {
				if IsPresent(transformed, "persistent-keepalive") {
					transformed["keepalive"] = GetInt(transformed, "persistent-keepalive")
				}
			}
			transformed["persistent-keepalive"] = GetInt(transformed, "keepalive")

			// preshared-key
			if !IsPresent(transformed, "preshared-key") {
				if IsPresent(transformed, "pre-shared-key") {
					transformed["preshared-key"] = GetString(transformed, "pre-shared-key")
				}
			}
			transformed["pre-shared-key"] = GetString(transformed, "preshared-key")

		case "snell":
			version := GetInt(transformed, "version")
			if version < 3 {
				delete(transformed, "udp")
			}

		case "vless":
			// Handle sni -> servername
			if IsPresent(transformed, "sni") {
				transformed["servername"] = GetString(transformed, "sni")
				delete(transformed, "sni")
			}
		}

		// Handle HTTP network options
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
						h2Opts["path"] = fmt.Sprintf("%v", pathSlice[0])
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
			wsOpts := GetMap(transformed, "ws-opts")
			if wsOpts == nil {
				wsOpts = make(map[string]interface{})
				transformed["ws-opts"] = wsOpts
			}

			path := GetString(wsOpts, "path")
			if path != "" {
				// Extract early data from path
				re := regexp.MustCompile(`^(.*?)(?:\?ed=(\d+))?$`)
				matches := re.FindStringSubmatch(path)
				if len(matches) > 0 {
					wsOpts["path"] = matches[1]
					if len(matches) > 2 && matches[2] != "" {
						wsOpts["early-data-header-name"] = "Sec-WebSocket-Protocol"
						ed, _ := strconv.Atoi(matches[2])
						wsOpts["max-early-data"] = ed
					}
				}
			} else {
				wsOpts["path"] = "/"
			}
		}

		// Handle plugin-opts TLS
		if pluginOpts := GetMap(transformed, "plugin-opts"); pluginOpts != nil {
			if GetBool(pluginOpts, "tls") && IsPresent(transformed, "skip-cert-verify") {
				pluginOpts["skip-cert-verify"] = GetBool(transformed, "skip-cert-verify")
			}
		}

		// Delete tls for certain proxy types
		deleteTLSTypes := map[string]bool{
			"trojan": true, "tuic": true, "hysteria": true,
			"hysteria2": true, "juicity": true, "anytls": true,
		}
		if deleteTLSTypes[proxyType] {
			delete(transformed, "tls")
		}

		// Handle tls-fingerprint -> fingerprint
		if IsPresent(transformed, "tls-fingerprint") {
			transformed["fingerprint"] = GetString(transformed, "tls-fingerprint")
		}
		delete(transformed, "tls-fingerprint")

		// Remove invalid tls field
		if IsPresent(transformed, "tls") {
			if _, ok := transformed["tls"].(bool); !ok {
				delete(transformed, "tls")
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
