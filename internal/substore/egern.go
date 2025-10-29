package substore

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EgernProducer implements the Producer interface for Egern format
// https://egernapp.com/zh-CN/docs/configuration/proxies
type EgernProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewEgernProducer creates a new Egern producer
func NewEgernProducer() *EgernProducer {
	return &EgernProducer{
		producerType: "egern",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *EgernProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to Egern format
func (p *EgernProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	// Supported Shadowsocks ciphers for Egern
	supportedSSCiphers := map[string]bool{
		"chacha20-ietf-poly1305": true,
		"chacha20-poly1305":      true,
		"aes-256-gcm":            true,
		"aes-128-gcm":            true,
		"none":                   true,
		"tbale":                  true,
		"rc4":                    true,
		"rc4-md5":                true,
		"aes-128-cfb":            true,
		"aes-192-cfb":            true,
		"aes-256-cfb":            true,
		"aes-128-ctr":            true,
		"aes-192-ctr":            true,
		"aes-256-ctr":            true,
		"bf-cfb":                 true,
		"camellia-128-cfb":       true,
		"camellia-192-cfb":       true,
		"camellia-256-cfb":       true,
		"cast5-cfb":              true,
		"des-cfb":                true,
		"idea-cfb":               true,
		"rc2-cfb":                true,
		"seed-cfb":               true,
		"salsa20":                true,
		"chacha20":               true,
		"chacha20-ietf":          true,
		"2022-blake3-aes-128-gcm": true,
		"2022-blake3-aes-256-gcm": true,
	}

	// Filter and transform proxies
	result := make([]map[string]interface{}, 0)

	for _, proxy := range proxies {
		original := p.helper.CloneProxy(proxy)
		proxyType := p.helper.GetProxyType(proxy)

		// Filter unsupported proxy types
		if !p.isSupportedType(proxyType) {
			continue
		}

		// Check Shadowsocks cipher and plugin
		if proxyType == "ss" {
			cipher := GetString(proxy, "cipher")
			plugin := GetString(proxy, "plugin")

			// Check plugin mode
			if plugin == "obfs" {
				if pluginOpts := GetMap(proxy, "plugin-opts"); pluginOpts != nil {
					mode := GetString(pluginOpts, "mode")
					if mode != "" && mode != "http" && mode != "tls" {
						continue
					}
				}
			}

			// Check cipher
			if !supportedSSCiphers[cipher] {
				continue
			}
		}

		// Check VMess network
		if proxyType == "vmess" {
			network := GetString(proxy, "network")
			if network != "" && network != "http" && network != "ws" && network != "tcp" {
				continue
			}
		}

		// Check Trojan network
		if proxyType == "trojan" {
			network := GetString(proxy, "network")
			if network != "" && network != "http" && network != "ws" && network != "tcp" {
				continue
			}
		}

		// Check VLESS network and flow
		if proxyType == "vless" {
			network := GetString(proxy, "network")
			flow := GetString(proxy, "flow")

			// Check flow support
			if !opts.IncludeUnsupportedProxy {
				if IsPresent(proxy, "flow") || IsPresent(proxy, "reality-opts") {
					continue
				}
			} else {
				if flow != "" && flow != "xtls-rprx-vision" {
					continue
				}
			}

			// Check network
			if network != "" && network != "http" && network != "ws" && network != "tcp" {
				continue
			}
		}

		// Check TUIC token
		if proxyType == "tuic" {
			token := GetString(proxy, "token")
			if token != "" {
				continue
			}
		}

		// Set default SNI
		if GetBool(proxy, "tls") && !IsPresent(proxy, "sni") {
			proxy["sni"] = GetString(proxy, "server")
		}

		// Get prev_hop (underlying proxy)
		prevHop := ""
		if IsPresent(original, "prev_hop") {
			prevHop = GetString(original, "prev_hop")
		} else if IsPresent(original, "underlying-proxy") {
			prevHop = GetString(original, "underlying-proxy")
		} else if IsPresent(original, "dialer-proxy") {
			prevHop = GetString(original, "dialer-proxy")
		} else if IsPresent(original, "detour") {
			prevHop = GetString(original, "detour")
		}

		var transformed Proxy
		var flow string

		// Transform based on proxy type
		switch proxyType {
		case "http":
			transformed = p.transformHTTP(proxy, original)
		case "socks5":
			transformed = p.transformSOCKS5(proxy, original)
		case "ss":
			transformed = p.transformShadowsocks(proxy, original)
		case "hysteria2":
			transformed = p.transformHysteria2(proxy, original)
		case "tuic":
			transformed = p.transformTUIC(proxy, original)
		case "trojan":
			transformed = p.transformTrojan(proxy, original)
		case "vmess":
			transformed = p.transformVMess(proxy, original)
		case "vless":
			transformed, flow = p.transformVLess(proxy, original)
		default:
			continue
		}

		// Add flow if present
		if flow != "" {
			transformed["flow"] = flow
		}

		// Handle shadow-tls for supported types
		if p.supportsShadowTLS(GetString(original, "type")) {
			if IsPresent(original, "shadow-tls-password") {
				version := GetInt(original, "shadow-tls-version")
				if version != 3 {
					return nil, fmt.Errorf("shadow-tls version %d is not supported", version)
				}
				transformed["shadow_tls"] = map[string]interface{}{
					"password": GetString(original, "shadow-tls-password"),
					"sni":      GetString(original, "shadow-tls-sni"),
				}
			} else if GetString(original, "plugin") == "shadow-tls" {
				if pluginOpts := GetMap(original, "plugin-opts"); pluginOpts != nil {
					version := GetInt(pluginOpts, "version")
					if version != 3 {
						return nil, fmt.Errorf("shadow-tls version %d is not supported", version)
					}
					transformed["shadow_tls"] = map[string]interface{}{
						"password": GetString(pluginOpts, "password"),
						"sni":      GetString(pluginOpts, "host"),
					}
				}
			}
		}

		// Handle UDP port for Shadowsocks with shadow-tls
		if GetString(original, "type") == "ss" && IsPresent(transformed, "shadow_tls") {
			udpPort := GetInt(original, "udp-port")
			if udpPort > 0 && udpPort <= 65535 {
				transformed["udp_port"] = udpPort
			}
		}

		// Clean up metadata fields
		delete(transformed, "subName")
		delete(transformed, "collectionName")
		delete(transformed, "id")
		delete(transformed, "resolved")
		delete(transformed, "no-resolve")

		// Clean up empty transport objects
		if transport := GetMap(transformed, "transport"); transport != nil {
			for key, val := range transport {
				if subMap, ok := val.(map[string]interface{}); ok {
					isEmpty := true
					for _, v := range subMap {
						if v != nil {
							isEmpty = false
							break
						}
					}
					if isEmpty {
						delete(transport, key)
					}
				}
			}
			if len(transport) == 0 {
				delete(transformed, "transport")
			}
		}

		// Remove null and underscore-prefixed fields for non-internal output
		if outputType != "internal" {
			for key := range transformed {
				if transformed[key] == nil || strings.HasPrefix(key, "_") {
					delete(transformed, key)
				}
			}
		}

		// Build final proxy object with type as key
		proxyObj := map[string]interface{}{
			GetString(transformed, "type"): transformed,
		}

		// Remove type from nested object and add prev_hop
		delete(transformed, "type")
		if prevHop != "" {
			transformed["prev_hop"] = prevHop
		}

		result = append(result, proxyObj)
	}

	// Return based on output type
	if outputType == "internal" {
		return result, nil
	}

	// Generate YAML string with JSON representation
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

// isSupportedType checks if a proxy type is supported by Egern
func (p *EgernProducer) isSupportedType(proxyType string) bool {
	supportedTypes := []string{
		"http", "socks5", "ss", "trojan", "hysteria2", "vless", "vmess", "tuic",
	}
	for _, t := range supportedTypes {
		if t == proxyType {
			return true
		}
	}
	return false
}

// supportsShadowTLS checks if a proxy type supports shadow-tls
func (p *EgernProducer) supportsShadowTLS(proxyType string) bool {
	supportedTypes := []string{
		"http", "socks5", "ss", "trojan", "vless", "vmess",
	}
	for _, t := range supportedTypes {
		if t == proxyType {
			return true
		}
	}
	return false
}

// transformHTTP transforms HTTP proxy
func (p *EgernProducer) transformHTTP(proxy, _ Proxy) Proxy {
	result := make(Proxy)
	result["type"] = "http"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")

	if IsPresent(proxy, "username") {
		result["username"] = GetString(proxy, "username")
	}
	if IsPresent(proxy, "password") {
		result["password"] = GetString(proxy, "password")
	}

	tfo := GetBool(proxy, "tfo") || GetBool(proxy, "fast-open")
	if tfo {
		result["tfo"] = tfo
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	return result
}

// transformSOCKS5 transforms SOCKS5 proxy
func (p *EgernProducer) transformSOCKS5(proxy, _ Proxy) Proxy {
	result := make(Proxy)
	result["type"] = "socks5"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")

	if IsPresent(proxy, "username") {
		result["username"] = GetString(proxy, "username")
	}
	if IsPresent(proxy, "password") {
		result["password"] = GetString(proxy, "password")
	}

	tfo := GetBool(proxy, "tfo") || GetBool(proxy, "fast-open")
	if tfo {
		result["tfo"] = tfo
	}

	udpRelay := GetBool(proxy, "udp") || GetBool(proxy, "udp_relay")
	if udpRelay {
		result["udp_relay"] = udpRelay
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	return result
}

// transformShadowsocks transforms Shadowsocks proxy
func (p *EgernProducer) transformShadowsocks(proxy, original Proxy) Proxy {
	result := make(Proxy)
	result["type"] = "shadowsocks"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")
	result["password"] = GetString(proxy, "password")

	// Handle cipher conversion
	cipher := GetString(proxy, "cipher")
	if cipher == "chacha20-ietf-poly1305" {
		result["method"] = "chacha20-poly1305"
	} else {
		result["method"] = cipher
	}

	tfo := GetBool(proxy, "tfo") || GetBool(proxy, "fast-open")
	if tfo {
		result["tfo"] = tfo
	}

	udpRelay := GetBool(proxy, "udp") || GetBool(proxy, "udp_relay")
	if udpRelay {
		result["udp_relay"] = udpRelay
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	// Handle obfs plugin
	if GetString(original, "plugin") == "obfs" {
		if pluginOpts := GetMap(original, "plugin-opts"); pluginOpts != nil {
			if IsPresent(pluginOpts, "mode") {
				result["obfs"] = GetString(pluginOpts, "mode")
			}
			if IsPresent(pluginOpts, "host") {
				result["obfs_host"] = GetString(pluginOpts, "host")
			}
			if IsPresent(pluginOpts, "path") {
				result["obfs_uri"] = GetString(pluginOpts, "path")
			}
		}
	}

	return result
}

// transformHysteria2 transforms Hysteria2 proxy
func (p *EgernProducer) transformHysteria2(proxy, original Proxy) Proxy {
	result := make(Proxy)
	result["type"] = "hysteria2"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")

	if IsPresent(proxy, "password") {
		result["auth"] = GetString(proxy, "password")
	}

	tfo := GetBool(proxy, "tfo") || GetBool(proxy, "fast-open")
	if tfo {
		result["tfo"] = tfo
	}

	udpRelay := GetBool(proxy, "udp") || GetBool(proxy, "udp_relay")
	if udpRelay {
		result["udp_relay"] = udpRelay
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	if IsPresent(proxy, "sni") {
		result["sni"] = GetString(proxy, "sni")
	}

	if IsPresent(proxy, "skip-cert-verify") {
		result["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
	}

	if IsPresent(proxy, "ports") {
		result["port_hopping"] = GetString(proxy, "ports")
	}

	if IsPresent(proxy, "hop-interval") {
		result["port_hopping_interval"] = GetString(proxy, "hop-interval")
	}

	// Handle obfs
	if IsPresent(original, "obfs-password") && GetString(original, "obfs") == "salamander" {
		result["obfs"] = "salamander"
		result["obfs_password"] = GetString(original, "obfs-password")
	}

	return result
}

// transformTUIC transforms TUIC proxy
func (p *EgernProducer) transformTUIC(proxy, _ Proxy) Proxy {
	result := make(Proxy)
	result["type"] = "tuic"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")

	if IsPresent(proxy, "uuid") {
		result["uuid"] = GetString(proxy, "uuid")
	}
	if IsPresent(proxy, "password") {
		result["password"] = GetString(proxy, "password")
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	if IsPresent(proxy, "sni") {
		result["sni"] = GetString(proxy, "sni")
	}

	// Handle alpn
	if IsPresent(proxy, "alpn") {
		alpnVal := proxy["alpn"]
		if alpnSlice, ok := alpnVal.([]interface{}); ok {
			result["alpn"] = alpnSlice
		} else if alpnStr, ok := alpnVal.(string); ok {
			result["alpn"] = []string{alpnStr}
		} else {
			result["alpn"] = []string{"h3"}
		}
	} else {
		result["alpn"] = []string{"h3"}
	}

	if IsPresent(proxy, "skip-cert-verify") {
		result["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
	}

	if IsPresent(proxy, "ports") {
		result["port_hopping"] = GetString(proxy, "ports")
	}

	if IsPresent(proxy, "hop-interval") {
		result["port_hopping_interval"] = GetString(proxy, "hop-interval")
	}

	return result
}

// transformTrojan transforms Trojan proxy
func (p *EgernProducer) transformTrojan(proxy, _ Proxy) Proxy {
	result := make(Proxy)
	result["type"] = "trojan"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")
	result["password"] = GetString(proxy, "password")

	tfo := GetBool(proxy, "tfo") || GetBool(proxy, "fast-open")
	if tfo {
		result["tfo"] = tfo
	}

	udpRelay := GetBool(proxy, "udp") || GetBool(proxy, "udp_relay")
	if udpRelay {
		result["udp_relay"] = udpRelay
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	if IsPresent(proxy, "sni") {
		result["sni"] = GetString(proxy, "sni")
	}

	if IsPresent(proxy, "skip-cert-verify") {
		result["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
	}

	// Handle WebSocket
	if GetString(proxy, "network") == "ws" {
		websocket := make(map[string]interface{})
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if IsPresent(wsOpts, "path") {
				websocket["path"] = GetString(wsOpts, "path")
			}
			if headers := GetMap(wsOpts, "headers"); headers != nil {
				if IsPresent(headers, "Host") {
					websocket["host"] = GetString(headers, "Host")
				}
			}
		}
		if len(websocket) > 0 {
			result["websocket"] = websocket
		}
	}

	return result
}

// transformVMess transforms VMess proxy
func (p *EgernProducer) transformVMess(proxy, _ Proxy) Proxy {
	result := make(Proxy)
	result["type"] = "vmess"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")
	result["user_id"] = GetString(proxy, "uuid")

	// Handle security/cipher
	security := GetString(proxy, "cipher")
	validSecurities := map[string]bool{
		"auto": true, "none": true, "zero": true,
		"aes-128-gcm": true, "chacha20-poly1305": true,
	}
	if !validSecurities[security] {
		security = "auto"
	}
	if security != "" {
		result["security"] = security
	}

	tfo := GetBool(proxy, "tfo") || GetBool(proxy, "fast-open")
	if tfo {
		result["tfo"] = tfo
	}

	// Handle legacy mode
	var legacy bool
	if IsPresent(proxy, "aead") && !GetBool(proxy, "aead") {
		legacy = true
	} else if GetInt(proxy, "alterId") != 0 {
		legacy = true
	}
	if legacy {
		result["legacy"] = legacy
	}

	udpRelay := GetBool(proxy, "udp") || GetBool(proxy, "udp_relay")
	if udpRelay {
		result["udp_relay"] = udpRelay
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	// Handle transport based on network type
	network := GetString(proxy, "network")
	transport := p.buildVMessTransport(proxy, network)
	if transport != nil {
		result["transport"] = transport
	}

	return result
}

// buildVMessTransport builds transport configuration for VMess
func (p *EgernProducer) buildVMessTransport(proxy Proxy, network string) map[string]interface{} {
	tls := GetBool(proxy, "tls")

	switch network {
	case "ws":
		transportKey := "ws"
		if tls {
			transportKey = "wss"
		}

		wsConfig := make(map[string]interface{})
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if IsPresent(wsOpts, "path") {
				wsConfig["path"] = GetString(wsOpts, "path")
			}
			if headers := GetMap(wsOpts, "headers"); headers != nil {
				headerMap := make(map[string]interface{})
				if IsPresent(headers, "Host") {
					headerMap["Host"] = GetString(headers, "Host")
				}
				if len(headerMap) > 0 {
					wsConfig["headers"] = headerMap
				}
			}
		}
		if tls {
			if IsPresent(proxy, "sni") {
				wsConfig["sni"] = GetString(proxy, "sni")
			}
			if IsPresent(proxy, "skip-cert-verify") {
				wsConfig["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
			}
		}
		return map[string]interface{}{transportKey: wsConfig}

	case "http":
		httpConfig := make(map[string]interface{})
		if httpOpts := GetMap(proxy, "http-opts"); httpOpts != nil {
			if IsPresent(httpOpts, "method") {
				httpConfig["method"] = GetString(httpOpts, "method")
			}
			if IsPresent(httpOpts, "path") {
				pathVal := httpOpts["path"]
				if pathSlice, ok := pathVal.([]interface{}); ok && len(pathSlice) > 0 {
					httpConfig["path"] = pathSlice[0]
				} else {
					httpConfig["path"] = pathVal
				}
			}
			if headers := GetMap(httpOpts, "headers"); headers != nil {
				headerMap := make(map[string]interface{})
				if IsPresent(headers, "Host") {
					hostVal := headers["Host"]
					if hostSlice, ok := hostVal.([]interface{}); ok && len(hostSlice) > 0 {
						headerMap["Host"] = hostSlice[0]
					} else {
						headerMap["Host"] = hostVal
					}
				}
				if len(headerMap) > 0 {
					httpConfig["headers"] = headerMap
				}
			}
		}
		if IsPresent(proxy, "skip-cert-verify") {
			httpConfig["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
		}
		return map[string]interface{}{"http1": httpConfig}

	case "h2":
		h2Config := make(map[string]interface{})
		if h2Opts := GetMap(proxy, "h2-opts"); h2Opts != nil {
			if IsPresent(h2Opts, "method") {
				h2Config["method"] = GetString(h2Opts, "method")
			}
			if IsPresent(h2Opts, "path") {
				pathVal := h2Opts["path"]
				if pathSlice, ok := pathVal.([]interface{}); ok && len(pathSlice) > 0 {
					h2Config["path"] = pathSlice[0]
				} else {
					h2Config["path"] = pathVal
				}
			}
			if headers := GetMap(h2Opts, "headers"); headers != nil {
				headerMap := make(map[string]interface{})
				if IsPresent(headers, "Host") {
					hostVal := headers["Host"]
					if hostSlice, ok := hostVal.([]interface{}); ok && len(hostSlice) > 0 {
						headerMap["Host"] = hostSlice[0]
					} else {
						headerMap["Host"] = hostVal
					}
				}
				if len(headerMap) > 0 {
					h2Config["headers"] = headerMap
				}
			}
		}
		if IsPresent(proxy, "skip-cert-verify") {
			h2Config["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
		}
		return map[string]interface{}{"http2": h2Config}

	case "tcp", "":
		if tls {
			tlsConfig := make(map[string]interface{})
			if IsPresent(proxy, "sni") {
				tlsConfig["sni"] = GetString(proxy, "sni")
			}
			if IsPresent(proxy, "skip-cert-verify") {
				tlsConfig["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
			}
			return map[string]interface{}{"tls": tlsConfig}
		}
	}

	return nil
}

// transformVLess transforms VLESS proxy
func (p *EgernProducer) transformVLess(proxy, _ Proxy) (Proxy, string) {
	result := make(Proxy)
	result["type"] = "vless"
	result["name"] = GetString(proxy, "name")
	result["server"] = GetString(proxy, "server")
	result["port"] = GetInt(proxy, "port")
	result["user_id"] = GetString(proxy, "uuid")

	if IsPresent(proxy, "cipher") {
		result["security"] = GetString(proxy, "cipher")
	}

	tfo := GetBool(proxy, "tfo") || GetBool(proxy, "fast-open")
	if tfo {
		result["tfo"] = tfo
	}

	udpRelay := GetBool(proxy, "udp") || GetBool(proxy, "udp_relay")
	if udpRelay {
		result["udp_relay"] = udpRelay
	}

	if IsPresent(proxy, "next_hop") {
		result["next_hop"] = GetString(proxy, "next_hop")
	}

	// Handle transport based on network type
	network := GetString(proxy, "network")
	flow := ""
	transport := p.buildVLessTransport(proxy, network, &flow)
	if transport != nil {
		result["transport"] = transport
	}

	return result, flow
}

// buildVLessTransport builds transport configuration for VLESS
func (p *EgernProducer) buildVLessTransport(proxy Proxy, network string, flow *string) map[string]interface{} {
	tls := GetBool(proxy, "tls")

	switch network {
	case "ws":
		transportKey := "ws"
		if tls {
			transportKey = "wss"
		}

		wsConfig := make(map[string]interface{})
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if IsPresent(wsOpts, "path") {
				wsConfig["path"] = GetString(wsOpts, "path")
			}
			if headers := GetMap(wsOpts, "headers"); headers != nil {
				headerMap := make(map[string]interface{})
				if IsPresent(headers, "Host") {
					headerMap["Host"] = GetString(headers, "Host")
				}
				if len(headerMap) > 0 {
					wsConfig["headers"] = headerMap
				}
			}
		}
		if tls {
			if IsPresent(proxy, "sni") {
				wsConfig["sni"] = GetString(proxy, "sni")
			}
			if IsPresent(proxy, "skip-cert-verify") {
				wsConfig["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
			}
		}
		return map[string]interface{}{transportKey: wsConfig}

	case "http":
		httpConfig := make(map[string]interface{})
		if httpOpts := GetMap(proxy, "http-opts"); httpOpts != nil {
			if IsPresent(httpOpts, "method") {
				httpConfig["method"] = GetString(httpOpts, "method")
			}
			if IsPresent(httpOpts, "path") {
				pathVal := httpOpts["path"]
				if pathSlice, ok := pathVal.([]interface{}); ok && len(pathSlice) > 0 {
					httpConfig["path"] = pathSlice[0]
				} else {
					httpConfig["path"] = pathVal
				}
			}
			if headers := GetMap(httpOpts, "headers"); headers != nil {
				headerMap := make(map[string]interface{})
				if IsPresent(headers, "Host") {
					hostVal := headers["Host"]
					if hostSlice, ok := hostVal.([]interface{}); ok && len(hostSlice) > 0 {
						headerMap["Host"] = hostSlice[0]
					} else {
						headerMap["Host"] = hostVal
					}
				}
				if len(headerMap) > 0 {
					httpConfig["headers"] = headerMap
				}
			}
		}
		if IsPresent(proxy, "skip-cert-verify") {
			httpConfig["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
		}
		return map[string]interface{}{"http": httpConfig}

	case "tcp", "":
		transportKey := "tcp"
		if tls {
			transportKey = "tls"
		}

		tcpConfig := make(map[string]interface{})
		if tls {
			if IsPresent(proxy, "sni") {
				tcpConfig["sni"] = GetString(proxy, "sni")
			}
			if IsPresent(proxy, "skip-cert-verify") {
				tcpConfig["skip_tls_verify"] = GetBool(proxy, "skip-cert-verify")
			}

			// Handle reality
			if realityOpts := GetMap(proxy, "reality-opts"); realityOpts != nil {
				if IsPresent(realityOpts, "short-id") || IsPresent(realityOpts, "public-key") {
					reality := make(map[string]interface{})
					if IsPresent(realityOpts, "short-id") {
						reality["short_id"] = GetString(realityOpts, "short-id")
					}
					if IsPresent(realityOpts, "public-key") {
						reality["public_key"] = GetString(realityOpts, "public-key")
					}
					tcpConfig["reality"] = reality
				}
			}
		}

		// Get flow for VLESS
		if flow != nil && IsPresent(proxy, "flow") {
			*flow = GetString(proxy, "flow")
		}

		return map[string]interface{}{transportKey: tcpConfig}
	}

	return nil
}
