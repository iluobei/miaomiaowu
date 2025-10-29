package substore

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// ClashMetaProducer implements the Producer interface for ClashMeta format
type ClashMetaProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewClashMetaProducer creates a new ClashMeta producer
func NewClashMetaProducer() *ClashMetaProducer {
	return &ClashMetaProducer{
		producerType: "clashmeta",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *ClashMetaProducer) GetType() string {
	return p.producerType
}

// IP version mapping
var ipVersionMapping = map[string]string{
	"dual":       "dual",
	"v4-only":    "ipv4",
	"v6-only":    "ipv6",
	"prefer-v4":  "ipv4-prefer",
	"prefer-v6":  "ipv6-prefer",
}

// Produce converts proxies to ClashMeta format
func (p *ClashMetaProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	// Supported ciphers for Shadowsocks in ClashMeta
	supportedSSCiphers := map[string]bool{
		"aes-128-ctr":               true,
		"aes-192-ctr":               true,
		"aes-256-ctr":               true,
		"aes-128-cfb":               true,
		"aes-192-cfb":               true,
		"aes-256-cfb":               true,
		"aes-128-gcm":               true,
		"aes-192-gcm":               true,
		"aes-256-gcm":               true,
		"aes-128-ccm":               true,
		"aes-192-ccm":               true,
		"aes-256-ccm":               true,
		"aes-128-gcm-siv":           true,
		"aes-256-gcm-siv":           true,
		"chacha20-ietf":             true,
		"chacha20":                  true,
		"xchacha20":                 true,
		"chacha20-ietf-poly1305":    true,
		"xchacha20-ietf-poly1305":   true,
		"chacha8-ietf-poly1305":     true,
		"xchacha8-ietf-poly1305":    true,
		"2022-blake3-aes-128-gcm":   true,
		"2022-blake3-aes-256-gcm":   true,
		"2022-blake3-chacha20-poly1305": true,
		"lea-128-gcm":               true,
		"lea-192-gcm":               true,
		"lea-256-gcm":               true,
		"rabbit128-poly1305":        true,
		"aegis-128l":                true,
		"aegis-256":                 true,
		"aez-384":                   true,
		"deoxys-ii-256-128":         true,
		"rc4-md5":                   true,
		"none":                      true,
	}

	// Supported VMess ciphers for ClashMeta
	supportedVMessCiphers := map[string]bool{
		"auto":             true,
		"none":             true,
		"zero":             true,
		"aes-128-gcm":      true,
		"chacha20-poly1305": true,
	}

	// Filter proxies
	filtered := make([]Proxy, 0)
	for _, proxy := range proxies {
		proxyType := p.helper.GetProxyType(proxy)

		// Skip if include-unsupported-proxy is not set
		if !opts.IncludeUnsupportedProxy {
			// Skip Snell v4+
			if proxyType == "snell" {
				version := GetInt(proxy, "version")
				if version >= 4 {
					continue
				}
			}

			// Skip juicity
			if proxyType == "juicity" {
				continue
			}

			// Check SS cipher
			if proxyType == "ss" {
				cipher := GetString(proxy, "cipher")
				if !supportedSSCiphers[cipher] {
					continue
				}
			}

			// Check anytls with reality or unsupported network
			if proxyType == "anytls" {
				network := GetString(proxy, "network")
				if network != "" && network != "tcp" {
					continue
				}
				if network == "tcp" && IsPresent(proxy, "reality-opts") {
					continue
				}
			}

			// Skip xhttp network
			if GetString(proxy, "network") == "xhttp" {
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

		case "tuic":
			// Handle alpn
			if IsPresent(transformed, "alpn") {
				if alpn, ok := transformed["alpn"].(string); ok {
					transformed["alpn"] = []string{alpn}
				}
			}

			// Handle tfo -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
			}

			// Set default version if token is empty
			token := GetString(transformed, "token")
			if token == "" && !IsPresent(transformed, "version") {
				transformed["version"] = 5
			}

		case "hysteria":
			// Handle auth_str -> auth-str
			if IsPresent(transformed, "auth_str") && !IsPresent(transformed, "auth-str") {
				transformed["auth-str"] = GetString(transformed, "auth_str")
			}

			// Handle alpn
			if IsPresent(transformed, "alpn") {
				if alpn, ok := transformed["alpn"].(string); ok {
					transformed["alpn"] = []string{alpn}
				}
			}

			// Handle tfo -> fast-open
			if IsPresent(transformed, "tfo") && !IsPresent(transformed, "fast-open") {
				transformed["fast-open"] = GetBool(transformed, "tfo")
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

		case "ss":
			// Handle shadow-tls plugin
			if IsPresent(transformed, "shadow-tls-password") && !IsPresent(transformed, "plugin") {
				transformed["plugin"] = "shadow-tls"
				pluginOpts := make(map[string]interface{})
				pluginOpts["host"] = GetString(transformed, "shadow-tls-sni")
				pluginOpts["password"] = GetString(transformed, "shadow-tls-password")
				pluginOpts["version"] = GetInt(transformed, "shadow-tls-version")
				transformed["plugin-opts"] = pluginOpts

				delete(transformed, "shadow-tls-password")
				delete(transformed, "shadow-tls-sni")
				delete(transformed, "shadow-tls-version")
			}
		}

		// Handle HTTP network options
		network := GetString(transformed, "network")
		if (proxyType == "vmess" || proxyType == "vless") && network == "http" {
			if httpOpts := GetMap(transformed, "http-opts"); httpOpts != nil {
				// Ensure path is array
				if IsPresent(httpOpts, "path") {
					if path, ok := httpOpts["path"].(string); ok {
						httpOpts["path"] = []string{path}
					}
				}

				// Ensure headers.Host is array
				if headers := GetMap(httpOpts, "headers"); headers != nil {
					if IsPresent(headers, "Host") {
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
				if IsPresent(h2Opts, "path") {
					if pathSlice, ok := h2Opts["path"].([]interface{}); ok && len(pathSlice) > 0 {
						h2Opts["path"] = pathSlice[0]
					}
				}

				// Ensure host is array
				if headers := GetMap(h2Opts, "headers"); headers != nil {
					if IsPresent(headers, "Host") {
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

		// Handle underlying-proxy -> dialer-proxy
		if IsPresent(transformed, "underlying-proxy") {
			transformed["dialer-proxy"] = GetString(transformed, "underlying-proxy")
		}
		delete(transformed, "underlying-proxy")

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

		// Handle IP version mapping
		if IsPresent(transformed, "ip-version") {
			ipVersion := GetString(transformed, "ip-version")
			if mapped, ok := ipVersionMapping[ipVersion]; ok {
				transformed["ip-version"] = mapped
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
