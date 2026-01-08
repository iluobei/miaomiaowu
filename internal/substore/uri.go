package substore

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// URIProducer implements URI scheme encoding for various proxy protocols
type URIProducer struct {
	producerType string
}

// NewURIProducer creates a new URI producer
func NewURIProducer() *URIProducer {
	return &URIProducer{
		producerType: "uri",
	}
}

// GetType returns the producer type
func (p *URIProducer) GetType() string {
	return p.producerType
}

// isIPv6 checks if a string is an IPv6 address
func isIPv6(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}

// preprocessProxy performs preprocessing on proxy before encoding
// This matches the frontend logic in uri.ts lines 268-291
func preprocessProxy(proxy Proxy) Proxy {
	// Clone proxy to avoid modifying original
	processed := make(Proxy)
	for k, v := range proxy {
		processed[k] = v
	}

	// Delete metadata fields
	delete(processed, "subName")
	delete(processed, "collectionName")
	delete(processed, "id")
	delete(processed, "resolved")
	delete(processed, "no-resolve")

	// Convert servername to sni if sni doesn't exist
	if servername := GetString(processed, "servername"); servername != "" && GetString(processed, "sni") == "" {
		processed["sni"] = servername
	}

	// Remove null/empty values
	for key := range processed {
		if processed[key] == nil || processed[key] == "" {
			delete(processed, key)
		}
	}

	// Delete tls field for certain protocols
	proxyType := GetString(processed, "type")
	if proxyType == "trojan" || proxyType == "tuic" || proxyType == "hysteria" ||
	   proxyType == "hysteria2" || proxyType == "juicity" {
		delete(processed, "tls")
	}

	// Add brackets for IPv6 addresses (except vmess)
	if proxyType != "vmess" {
		if server := GetString(processed, "server"); server != "" && isIPv6(server) {
			processed["server"] = "[" + server + "]"
		}
	}

	return processed
}

// Produce converts all proxies to URI format, one per line
func (p *URIProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if len(proxies) == 0 {
		return "", nil
	}

	// Process all proxies and collect URIs
	var uris []string
	for _, proxy := range proxies {
		// Preprocess proxy (matches frontend logic)
		proxy = preprocessProxy(proxy)
		proxyType := GetString(proxy, "type")

		var uri string
		var err error

		switch proxyType {
		case "vmess":
			uri, err = p.encodeVMess(proxy)
		case "vless":
			uri, err = p.encodeVLESS(proxy)
		case "trojan":
			uri, err = p.encodeTrojan(proxy)
		case "ss":
			uri, err = p.encodeShadowsocks(proxy)
		case "ssr":
			uri, err = p.encodeShadowsocksR(proxy)
		case "hysteria2":
			uri, err = p.encodeHysteria2(proxy)
		case "hysteria":
			uri, err = p.encodeHysteria(proxy)
		case "tuic":
			uri, err = p.encodeTUIC(proxy)
		case "socks5":
			uri, err = p.encodeSOCKS5(proxy)
		case "wireguard":
			uri, err = p.encodeWireGuard(proxy)
		case "anytls":
			uri, err = p.encodeAnyTLS(proxy)
		default:
			// Skip unsupported proxy types instead of returning error
			continue
		}

		if err != nil {
			// Skip proxies that fail to encode
			continue
		}

		uris = append(uris, uri)
	}

	// Join all URIs with newline
	return strings.Join(uris, "\n"), nil
}

// ProduceOne is a helper to encode a single proxy
func (p *URIProducer) ProduceOne(proxy Proxy) (string, error) {
	result, err := p.Produce([]Proxy{proxy}, "", nil)
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

// encodeVMess encodes VMess proxy to vmess:// URI (matches frontend)
func (p *URIProducer) encodeVMess(proxy Proxy) (string, error) {
	// Handle network type conversion (frontend line 376-386)
	network := GetString(proxy, "network")
	net := network
	typeField := ""

	if network == "http" {
		net = "tcp"
		typeField = "http"
	} else if network == "ws" {
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if GetBool(wsOpts, "v2ray-http-upgrade") {
				net = "httpupgrade"
			}
		}
	}

	config := map[string]interface{}{
		"v":    "2",
		"ps":   GetString(proxy, "name"),
		"add":  GetString(proxy, "server"),
		"port": fmt.Sprintf("%d", GetInt(proxy, "port")),
		"id":   GetString(proxy, "uuid"),
		"aid":  fmt.Sprintf("%d", GetInt(proxy, "alterId")),
		"scy":  GetString(proxy, "cipher"),
		"net":  net,
		"type": typeField,
		"tls":  "",
	}

	// TLS
	if GetBool(proxy, "tls") {
		config["tls"] = "tls"
		if sni := GetString(proxy, "sni"); sni != "" {
			config["sni"] = sni
		}
	}

	// ALPN
	if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
		config["alpn"] = strings.Join(alpn, ",")
	}

	// Fingerprint
	if fp := GetString(proxy, "client-fingerprint"); fp != "" {
		config["fp"] = fp
	}

	// UDP (frontend line 407-409)
	if udp, ok := proxy["udp"]; ok {
		config["udp"] = udp
	}

	// Network specific options (frontend line 411-454)
	if network != "" {
		switch network {
		case "grpc":
			if grpcOpts := GetMap(proxy, "grpc-opts"); grpcOpts != nil {
				config["path"] = GetString(grpcOpts, "grpc-service-name")
				// https://github.com/XTLS/Xray-core/issues/91
				config["type"] = GetString(grpcOpts, "_grpc-type")
				if config["type"] == "" {
					config["type"] = "gun"
				}
				if authority := GetString(grpcOpts, "_grpc-authority"); authority != "" {
					config["host"] = authority
				}
			}
		case "kcp", "quic":
			if opts := GetMap(proxy, network+"-opts"); opts != nil {
				typeKey := "_" + network + "-type"
				hostKey := "_" + network + "-host"
				pathKey := "_" + network + "-path"
				config["type"] = GetString(opts, typeKey)
				if config["type"] == "" {
					config["type"] = "none"
				}
				if host := GetString(opts, hostKey); host != "" {
					config["host"] = host
				}
				if path := GetString(opts, pathKey); path != "" {
					config["path"] = path
				}
			}
		default:
			// ws, http, h2, etc.
			if opts := GetMap(proxy, network+"-opts"); opts != nil {
				if path := opts["path"]; path != nil {
					if pathSlice, ok := path.([]interface{}); ok && len(pathSlice) > 0 {
						config["path"] = fmt.Sprintf("%v", pathSlice[0])
					} else if pathStr, ok := path.(string); ok {
						config["path"] = pathStr
					}
				}
				if headers := GetMap(opts, "headers"); headers != nil {
					if host := headers["Host"]; host != nil {
						if hostSlice, ok := host.([]interface{}); ok && len(hostSlice) > 0 {
							config["host"] = fmt.Sprintf("%v", hostSlice[0])
						} else if hostStr, ok := host.(string); ok {
							config["host"] = hostStr
						}
					}
				}
			}
		}
	}

	jsonBytes, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	return "vmess://" + encoded, nil
}

// encodeVLESS encodes VLESS proxy to vless:// URI
func (p *URIProducer) encodeVLESS(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	uuid := GetString(proxy, "uuid")
	name := GetString(proxy, "name")

	params := url.Values{}

	// Security
	security := "none"
	if GetBool(proxy, "tls") {
		security = "tls"
	}
	if realityOpts := GetMap(proxy, "reality-opts"); realityOpts != nil {
		security = "reality"
		if pubKey := GetString(realityOpts, "public-key"); pubKey != "" {
			params.Set("pbk", pubKey)
		}
		if shortID := GetAnyString(realityOpts, "short-id"); shortID != "" {
			params.Set("sid", shortID)
		}
		if spiderX := GetString(realityOpts, "_spider-x"); spiderX != "" {
			params.Set("spx", spiderX)
		}
	}
	params.Set("security", security)

	// SNI
	if sni := GetString(proxy, "servername"); sni != "" {
		params.Set("sni", sni)
	}

	// ALPN
	if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
		params.Set("alpn", strings.Join(alpn, ","))
	}

	// Fingerprint
	if fp := GetString(proxy, "client-fingerprint"); fp != "" {
		params.Set("fp", fp)
	}

	// Flow
	if flow := GetString(proxy, "flow"); flow != "" {
		params.Set("flow", flow)
	}

	// Skip cert verify
	if GetBool(proxy, "skip-cert-verify") {
		params.Set("allowInsecure", "1")
	}

	// Encryption
	if encryption := GetString(proxy, "encryption"); encryption != "" {
		params.Set("encryption", encryption)
	}

	// Network type (frontend line 187-190)
	network := GetString(proxy, "network")
	vlessType := network
	if network == "splithttp" {
		vlessType = "xhttp"
	} else if network == "ws" {
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if GetBool(wsOpts, "v2ray-http-upgrade") {
				vlessType = "httpupgrade"
			}
		}
	}

	if vlessType != "" {
		params.Set("type", vlessType)
	}

	// Mode, extra, pqv parameters (frontend line 175-182)
	if mode := GetString(proxy, "_mode"); mode != "" {
		params.Set("mode", mode)
	}
	if extra := GetString(proxy, "_extra"); extra != "" {
		params.Set("extra", extra)
	}
	if pqv := GetString(proxy, "_pqv"); pqv != "" {
		params.Set("pqv", pqv)
	}

	// Network-specific options
	switch network {
	case "ws":
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if path := GetString(wsOpts, "path"); path != "" {
				params.Set("path", path)
			}
			if headers := GetMap(wsOpts, "headers"); headers != nil {
				if host := GetString(headers, "Host"); host != "" {
					params.Set("host", host)
				}
			}
		}
	case "grpc":
		if grpcOpts := GetMap(proxy, "grpc-opts"); grpcOpts != nil {
			if serviceName := GetString(grpcOpts, "grpc-service-name"); serviceName != "" {
				params.Set("serviceName", serviceName)
			}
			// Frontend line 195-201: grpc mode and authority
			grpcType := GetString(grpcOpts, "_grpc-type")
			if grpcType == "" {
				grpcType = "gun"
			}
			params.Set("mode", grpcType)
			if authority := GetString(grpcOpts, "_grpc-authority"); authority != "" {
				params.Set("authority", authority)
			}
		}
	case "xhttp":
		// Frontend line 204-206: splithttp/xhttp mode
		if mode := GetString(proxy, "_mode"); mode != "" {
			params.Set("mode", mode)
		} else {
			params.Set("mode", "auto")
		}
		if xhttpOpts := GetMap(proxy, "xhttp-opts"); xhttpOpts != nil {
			if path := GetString(xhttpOpts, "path"); path != "" {
				params.Set("path", path)
			}
			if headers := GetMap(xhttpOpts, "headers"); headers != nil {
				if host := GetString(headers, "Host"); host != "" {
					params.Set("host", host)
				}
			}
		}

	case "splithttp":
		params.Set("type", "xhttp")
		// Frontend line 204-206: splithttp mode
		if mode := GetString(proxy, "_mode"); mode != "" {
			params.Set("mode", mode)
		} else {
			params.Set("mode", "auto")
		}
		if splithttpOpts := GetMap(proxy, "splithttp-opts"); splithttpOpts != nil {
			if path := GetString(splithttpOpts, "path"); path != "" {
				params.Set("path", path)
			}
			if headers := GetMap(splithttpOpts, "headers"); headers != nil {
				if host := GetString(headers, "Host"); host != "" {
					params.Set("host", host)
				}
			}
		}
	case "kcp":
		if kcpOpts := GetMap(proxy, "kcp-opts"); kcpOpts != nil {
			if seed := GetString(kcpOpts, "seed"); seed != "" {
				params.Set("seed", seed)
			}
			if headerType := GetString(kcpOpts, "headerType"); headerType != "" {
				params.Set("headerType", headerType)
			}
		}
	}

	// UDP parameter (frontend line 244-247)
	if udp, ok := proxy["udp"]; ok {
		if udpBool, ok := udp.(bool); ok {
			if udpBool {
				params.Set("udp", "1")
			} else {
				params.Set("udp", "0")
			}
		}
	}

	uri := fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		uuid, server, port, params.Encode(), url.PathEscape(name))
	return uri, nil
}

// encodeTrojan encodes Trojan proxy to trojan:// URI
func (p *URIProducer) encodeTrojan(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	params := url.Values{}

	// SNI
	sni := GetString(proxy, "sni")
	if sni == "" {
		sni = GetString(proxy, "servername")
	}
	if sni == "" {
		sni = server
	}
	params.Set("sni", sni)

	// Skip cert verify
	if GetBool(proxy, "skip-cert-verify") {
		params.Set("allowInsecure", "1")
	}

	// ALPN
	if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
		params.Set("alpn", strings.Join(alpn, ","))
	}

	// Fingerprint
	if fp := GetString(proxy, "client-fingerprint"); fp != "" {
		params.Set("fp", fp)
	}

	// Reality support (frontend line 526-555)
	if realityOpts := GetMap(proxy, "reality-opts"); realityOpts != nil {
		params.Set("security", "reality")
		if pubKey := GetString(realityOpts, "public-key"); pubKey != "" {
			params.Set("pbk", pubKey)
		}
		if shortID := GetAnyString(realityOpts, "short-id"); shortID != "" {
			params.Set("sid", shortID)
		}
		if spiderX := GetString(realityOpts, "_spider-x"); spiderX != "" {
			params.Set("spx", spiderX)
		}
		if extra := GetString(proxy, "_extra"); extra != "" {
			params.Set("extra", extra)
		}
		if mode := GetString(proxy, "_mode"); mode != "" {
			params.Set("mode", mode)
		}
	}

	// Network type (frontend line 461-470)
	network := GetString(proxy, "network")
	trojanType := network
	if network == "ws" {
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if GetBool(wsOpts, "v2ray-http-upgrade") {
				trojanType = "httpupgrade"
			}
		}
	}
	if trojanType != "" {
		params.Set("type", trojanType)

		// Network-specific options
		switch network {
		case "ws":
			if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
				if path := GetString(wsOpts, "path"); path != "" {
					params.Set("path", path)
				}
				if headers := GetMap(wsOpts, "headers"); headers != nil {
					if host := GetString(headers, "Host"); host != "" {
						params.Set("host", host)
					}
				}
			}
		case "grpc":
			if grpcOpts := GetMap(proxy, "grpc-opts"); grpcOpts != nil {
				if serviceName := GetString(grpcOpts, "grpc-service-name"); serviceName != "" {
					params.Set("serviceName", serviceName)
				}
				// Frontend line 476-491: grpc authority and mode
				if authority := GetString(grpcOpts, "_grpc-authority"); authority != "" {
					params.Set("authority", authority)
				}
				grpcType := GetString(grpcOpts, "_grpc-type")
				if grpcType == "" {
					grpcType = "gun"
				}
				params.Set("mode", grpcType)
			}
		}
	}

	// UDP parameter (frontend line 557-560)
	if udp, ok := proxy["udp"]; ok {
		if udpBool, ok := udp.(bool); ok {
			if udpBool {
				params.Set("udp", "1")
			} else {
				params.Set("udp", "0")
			}
		}
	}

	uri := fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		password, server, port, params.Encode(), url.PathEscape(name))
	return uri, nil
}

// encodeShadowsocks encodes Shadowsocks proxy to ss:// URI (matches frontend)
func (p *URIProducer) encodeShadowsocks(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	cipher := GetString(proxy, "cipher")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	// Format: method:password (frontend line 305-313)
	userInfo := fmt.Sprintf("%s:%s", cipher, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(userInfo))

	uri := fmt.Sprintf("ss://%s@%s:%d", encoded, server, port)

	// Plugin support (frontend line 314-342)
	plugin := GetString(proxy, "plugin")
	if plugin != "" {
		uri += "/"
	}

	params := url.Values{}
	hasPlugin := false

	if plugin == "obfs" {
		opts := GetMap(proxy, "plugin-opts")
		mode := GetString(opts, "mode")
		host := GetString(opts, "host")
		pluginStr := fmt.Sprintf("simple-obfs;obfs=%s", mode)
		if host != "" {
			pluginStr += ";obfs-host=" + host
		}
		params.Set("plugin", pluginStr)
		hasPlugin = true
	} else if plugin == "v2ray-plugin" {
		opts := GetMap(proxy, "plugin-opts")
		mode := GetString(opts, "mode")
		host := GetString(opts, "host")
		tls := GetBool(opts, "tls")
		pluginStr := fmt.Sprintf("v2ray-plugin;obfs=%s", mode)
		if host != "" {
			pluginStr += ";obfs-host=" + host
		}
		if tls {
			pluginStr += ";tls"
		}
		params.Set("plugin", pluginStr)
		hasPlugin = true
	} else if plugin == "shadow-tls" {
		opts := GetMap(proxy, "plugin-opts")
		host := GetString(opts, "host")
		pass := GetString(opts, "password")
		version := GetInt(opts, "version")
		pluginStr := fmt.Sprintf("shadow-tls;host=%s;password=%s;version=%d", host, pass, version)
		params.Set("plugin", pluginStr)
		hasPlugin = true
	}

	// UDP over TCP (frontend line 343-345)
	if GetBool(proxy, "udp-over-tcp") {
		params.Set("uot", "1")
	}

	// TFO (frontend line 346-350)
	if GetBool(proxy, "tfo") {
		params.Set("tfo", "1")
	}

	// UDP (frontend line 351-355)
	if udp, ok := proxy["udp"]; ok {
		if udpBool, ok := udp.(bool); ok {
			if udpBool {
				params.Set("udp", "1")
			} else {
				params.Set("udp", "0")
			}
		}
	}

	// Append parameters
	if hasPlugin || len(params) > 0 {
		paramStr := params.Encode()
		if paramStr != "" {
			if !hasPlugin {
				uri += "?" + paramStr
			} else {
				uri += "?" + paramStr
			}
		}
	}

	uri += "#" + url.PathEscape(name)
	return uri, nil
}

// encodeShadowsocksR encodes ShadowsocksR proxy to ssr:// URI
func (p *URIProducer) encodeShadowsocksR(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	protocol := GetString(proxy, "protocol")
	cipher := GetString(proxy, "cipher")
	obfs := GetString(proxy, "obfs")
	password := GetString(proxy, "password")

	params := url.Values{}
	if obfsParam := GetString(proxy, "obfs-param"); obfsParam != "" {
		params.Set("obfsparam", base64.URLEncoding.EncodeToString([]byte(obfsParam)))
	}
	if protocolParam := GetString(proxy, "protocol-param"); protocolParam != "" {
		params.Set("protoparam", base64.URLEncoding.EncodeToString([]byte(protocolParam)))
	}
	if name := GetString(proxy, "name"); name != "" {
		params.Set("remarks", base64.URLEncoding.EncodeToString([]byte(name)))
	}

	// Format: server:port:protocol:cipher:obfs:password_base64/?params
	passwordB64 := base64.URLEncoding.EncodeToString([]byte(password))
	main := fmt.Sprintf("%s:%d:%s:%s:%s:%s", server, port, protocol, cipher, obfs, passwordB64)

	encoded := base64.URLEncoding.EncodeToString([]byte(main + "/?" + params.Encode()))
	return "ssr://" + strings.TrimRight(encoded, "="), nil
}

// encodeHysteria2 encodes Hysteria2 proxy to hysteria2:// or hy2:// URI
func (p *URIProducer) encodeHysteria2(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	params := url.Values{}

	// SNI
	if sni := GetString(proxy, "servername"); sni != "" {
		params.Set("sni", sni)
	}

	// Skip cert verify
	if GetBool(proxy, "skip-cert-verify") {
		params.Set("insecure", "1")
	}

	// ALPN
	if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
		params.Set("alpn", strings.Join(alpn, ","))
	}

	// Obfuscation
	if obfs := GetString(proxy, "obfs"); obfs != "" {
		params.Set("obfs", obfs)
		if obfsPassword := GetString(proxy, "obfs-password"); obfsPassword != "" {
			params.Set("obfs-password", obfsPassword)
		}
	}

	// hop-interval (frontend line 571-575)
	if hopInterval := GetString(proxy, "hop-interval"); hopInterval != "" {
		params.Set("hop-interval", hopInterval)
	}

	// keepalive (frontend line 576-578)
	if keepalive := proxy["keepalive"]; keepalive != nil {
		params.Set("keepalive", fmt.Sprintf("%v", keepalive))
	}

	// ports → mport (frontend line 599-601)
	if ports := GetString(proxy, "ports"); ports != "" {
		params.Set("mport", ports)
	}

	// tls-fingerprint → pinSHA256 (frontend line 602-608)
	if tlsFingerprint := GetString(proxy, "tls-fingerprint"); tlsFingerprint != "" {
		params.Set("pinSHA256", tlsFingerprint)
	}

	// tfo → fastopen (frontend line 609-611)
	if GetBool(proxy, "tfo") {
		params.Set("fastopen", "1")
	}

	// UDP (frontend line 612-615)
	if udp, ok := proxy["udp"]; ok {
		if udpBool, ok := udp.(bool); ok {
			if udpBool {
				params.Set("udp", "1")
			} else {
				params.Set("udp", "0")
			}
		}
	}

	uri := fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s",
		url.PathEscape(password), server, port, params.Encode(), url.PathEscape(name))
	return uri, nil
}

// encodeHysteria encodes Hysteria proxy to hysteria:// URI (completely rewritten to match frontend)
func (p *URIProducer) encodeHysteria(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	name := GetString(proxy, "name")

	hysteriaParams := []string{}

	// Frontend line 624-671: Iterate through all keys with complex mapping
	for key := range proxy {
		if key == "name" || key == "type" || key == "server" || key == "port" {
			continue
		}

		val := proxy[key]
		if val == nil {
			continue
		}

		// Skip keys starting with underscore
		if strings.HasPrefix(key, "_") {
			continue
		}

		// Special mappings (frontend line 626-669)
		switch key {
		case "alpn":
			if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
				hysteriaParams = append(hysteriaParams, fmt.Sprintf("alpn=%s", url.QueryEscape(alpn[0])))
			}
		case "skip-cert-verify":
			if GetBool(proxy, "skip-cert-verify") {
				hysteriaParams = append(hysteriaParams, "insecure=1")
			}
		case "tfo", "fast-open":
			if GetBool(proxy, key) {
				// Only add once
				hasParam := false
				for _, p := range hysteriaParams {
					if p == "fastopen=1" {
						hasParam = true
						break
					}
				}
				if !hasParam {
					hysteriaParams = append(hysteriaParams, "fastopen=1")
				}
			}
		case "ports":
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("mport=%v", val))
		case "auth-str":
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("auth=%v", val))
		case "up":
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("upmbps=%v", val))
		case "down":
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("downmbps=%v", val))
		case "_obfs":
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("obfs=%v", val))
		case "obfs":
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("obfsParam=%v", val))
		case "sni":
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("peer=%v", val))
		case "udp":
			if udpBool, ok := val.(bool); ok {
				if udpBool {
					hysteriaParams = append(hysteriaParams, "udp=1")
				} else {
					hysteriaParams = append(hysteriaParams, "udp=0")
				}
			}
		default:
			// Other parameters: replace - with _
			paramKey := strings.ReplaceAll(key, "-", "_")
			hysteriaParams = append(hysteriaParams, fmt.Sprintf("%s=%s", paramKey, url.QueryEscape(fmt.Sprintf("%v", val))))
		}
	}

	uri := fmt.Sprintf("hysteria://%s:%d?%s#%s",
		server, port, strings.Join(hysteriaParams, "&"), url.PathEscape(name))
	return uri, nil
}

// encodeTUIC encodes TUIC proxy to tuic:// URI (completely rewritten to match frontend)
// Frontend line 680-749: Only process when token is not present
func (p *URIProducer) encodeTUIC(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	uuid := GetString(proxy, "uuid")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	// Frontend line 681: Only process if no token
	token := GetString(proxy, "token")
	if token != "" && len(token) > 0 {
		// Skip if token exists
		return "", fmt.Errorf("TUIC with token is not supported in URI format")
	}

	tuicParams := []string{}

	// Frontend line 683-740: Iterate through all keys with complex mapping
	for key := range proxy {
		if key == "name" || key == "type" || key == "uuid" || key == "password" ||
			key == "server" || key == "port" || key == "tls" {
			continue
		}

		val := proxy[key]
		if val == nil {
			continue
		}

		// Skip keys starting with underscore
		if strings.HasPrefix(key, "_") {
			continue
		}

		// Special mappings (frontend line 695-738)
		paramKey := strings.ReplaceAll(key, "-", "_")
		switch key {
		case "alpn":
			if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
				tuicParams = append(tuicParams, fmt.Sprintf("alpn=%s", url.QueryEscape(alpn[0])))
			}
		case "skip-cert-verify":
			if GetBool(proxy, "skip-cert-verify") {
				tuicParams = append(tuicParams, "allow_insecure=1")
			}
		case "tfo", "fast-open":
			if GetBool(proxy, key) {
				// Only add once
				hasParam := false
				for _, p := range tuicParams {
					if p == "fast_open=1" {
						hasParam = true
						break
					}
				}
				if !hasParam {
					tuicParams = append(tuicParams, "fast_open=1")
				}
			}
		case "disable-sni", "reduce-rtt":
			if GetBool(proxy, key) {
				tuicParams = append(tuicParams, fmt.Sprintf("%s=1", strings.ReplaceAll(key, "-", "_")))
			}
		case "congestion-controller":
			tuicParams = append(tuicParams, fmt.Sprintf("congestion_control=%v", val))
		case "udp":
			if udpBool, ok := val.(bool); ok {
				if udpBool {
					tuicParams = append(tuicParams, "udp=1")
				} else {
					tuicParams = append(tuicParams, "udp=0")
				}
			}
		default:
			// Other parameters: replace - with _ and encode
			tuicParams = append(tuicParams, fmt.Sprintf("%s=%s", strings.ReplaceAll(paramKey, "-", "_"), url.QueryEscape(fmt.Sprintf("%v", val))))
		}
	}

	// Build auth part: uuid:password (frontend line 742-744)
	auth := url.PathEscape(uuid)
	if password != "" {
		auth = url.PathEscape(uuid) + ":" + url.PathEscape(password)
	}

	uri := fmt.Sprintf("tuic://%s@%s:%d?%s#%s",
		auth, server, port, strings.Join(tuicParams, "&"), url.PathEscape(name))
	return uri, nil
}

// encodeSOCKS5 encodes SOCKS5 proxy to socks:// URI (matches frontend)
func (p *URIProducer) encodeSOCKS5(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	username := GetString(proxy, "username")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	// Encode username:password in Base64 (matches frontend line 298-302)
	userInfo := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(userInfo))

	// UDP parameter
	params := ""
	if udp, ok := proxy["udp"]; ok {
		if udpBool, ok := udp.(bool); ok {
			if udpBool {
				params = "?udp=1"
			} else {
				params = "?udp=0"
			}
		}
	}

	uri := fmt.Sprintf("socks://%s@%s:%d%s#%s",
		url.PathEscape(encoded), server, port, params, url.PathEscape(name))
	return uri, nil
}

// encodeWireGuard encodes WireGuard proxy to wireguard:// URI (matches frontend)
func (p *URIProducer) encodeWireGuard(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	privateKey := GetString(proxy, "private-key")
	name := GetString(proxy, "name")
	ip := GetString(proxy, "ip")
	ipv6 := GetString(proxy, "ipv6")

	params := url.Values{}

	// Public key (frontend line 811-812)
	if publicKey := GetString(proxy, "public-key"); publicKey != "" {
		params.Set("publickey", publicKey)
	}

	// Address (frontend line 823-831)
	if ip != "" && ipv6 != "" {
		params.Set("address", fmt.Sprintf("%s/32,%s/128", ip, ipv6))
	} else if ip != "" {
		params.Set("address", fmt.Sprintf("%s/32", ip))
	} else if ipv6 != "" {
		params.Set("address", fmt.Sprintf("%s/128", ipv6))
	}

	// Other parameters
	for key, val := range proxy {
		if key == "name" || key == "type" || key == "server" || key == "port" ||
			key == "ip" || key == "ipv6" || key == "private-key" || key == "public-key" {
			continue
		}
		if strings.HasPrefix(key, "_") {
			continue
		}
		if key == "udp" {
			if udpBool, ok := val.(bool); ok {
				if udpBool {
					params.Set("udp", "1")
				} else {
					params.Set("udp", "0")
				}
			}
		} else if val != nil && val != "" {
			params.Set(key, fmt.Sprintf("%v", val))
		}
	}

	uri := fmt.Sprintf("wireguard://%s@%s:%d/?%s#%s",
		url.PathEscape(privateKey), server, port, params.Encode(), url.PathEscape(name))
	return uri, nil
}

// encodeAnyTLS encodes AnyTLS proxy to anytls:// URI (matches frontend)
func (p *URIProducer) encodeAnyTLS(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	params := url.Values{}

	// skip-cert-verify (frontend line 756-758)
	if GetBool(proxy, "skip-cert-verify") {
		params.Set("insecure", "1")
	}

	// SNI (frontend line 761-763)
	if sni := GetString(proxy, "sni"); sni != "" {
		params.Set("sni", sni)
	}

	// client-fingerprint (frontend line 766-768)
	if fp := GetString(proxy, "client-fingerprint"); fp != "" {
		params.Set("fp", fp)
	}

	// ALPN (frontend line 771-773)
	if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
		alpnStrs := make([]string, len(alpn))
		for i, a := range alpn {
			alpnStrs[i] = url.QueryEscape(a)
		}
		params.Set("alpn", strings.Join(alpnStrs, ","))
	}

	// UDP (frontend line 776-778)
	if udp, ok := proxy["udp"]; ok {
		if udpBool, ok := udp.(bool); ok {
			if udpBool {
				params.Set("udp", "1")
			} else {
				params.Set("udp", "0")
			}
		}
	}

	// idle session parameters (frontend line 781-789)
	if val := GetString(proxy, "idle-session-check-interval"); val != "" {
		params.Set("idleSessionCheckInterval", val)
	}
	if val := GetString(proxy, "idle-session-timeout"); val != "" {
		params.Set("idleSessionTimeout", val)
	}
	if val := proxy["min-idle-session"]; val != nil {
		params.Set("minIdleSession", fmt.Sprintf("%v", val))
	}

	// Build URI (frontend line 792-794)
	uri := fmt.Sprintf("anytls://%s@%s:%d", url.PathEscape(password), server, port)
	if len(params) > 0 {
		uri += "/?" + params.Encode()
	}
	uri += "#" + url.PathEscape(name)
	return uri, nil
}
