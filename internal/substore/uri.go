package substore

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// Produce converts all proxies to URI format, one per line
func (p *URIProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if len(proxies) == 0 {
		return "", nil
	}

	// Process all proxies and collect URIs
	var uris []string
	for _, proxy := range proxies {
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

// encodeVMess encodes VMess proxy to vmess:// URI
func (p *URIProducer) encodeVMess(proxy Proxy) (string, error) {
	config := map[string]interface{}{
		"v":    "2",
		"ps":   GetString(proxy, "name"),
		"add":  GetString(proxy, "server"),
		"port": fmt.Sprintf("%d", GetInt(proxy, "port")),
		"id":   GetString(proxy, "uuid"),
		"aid":  fmt.Sprintf("%d", GetInt(proxy, "alterId")),
		"net":  GetString(proxy, "network"),
		"type": "none",
		"tls":  "",
	}

	// TLS
	if GetBool(proxy, "tls") {
		config["tls"] = "tls"
	}

	// SNI
	if sni := GetString(proxy, "sni"); sni != "" {
		config["sni"] = sni
	} else if servername := GetString(proxy, "servername"); servername != "" {
		config["sni"] = servername
	}

	// Cipher
	if cipher := GetString(proxy, "cipher"); cipher != "" {
		config["scy"] = cipher
	}

	// ALPN
	if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
		config["alpn"] = strings.Join(alpn, ",")
	}

	// Fingerprint
	if fp := GetString(proxy, "client-fingerprint"); fp != "" {
		config["fp"] = fp
	}

	// Network specific options
	network := GetString(proxy, "network")
	switch network {
	case "ws":
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			config["path"] = GetString(wsOpts, "path")
			if headers := GetMap(wsOpts, "headers"); headers != nil {
				config["host"] = GetString(headers, "Host")
			}
		}
	case "grpc":
		if grpcOpts := GetMap(proxy, "grpc-opts"); grpcOpts != nil {
			config["path"] = GetString(grpcOpts, "grpc-service-name")
		}
	case "h2", "http":
		if h2Opts := GetMap(proxy, "h2-opts"); h2Opts != nil {
			if path := h2Opts["path"]; path != nil {
				if pathSlice, ok := path.([]interface{}); ok && len(pathSlice) > 0 {
					config["path"] = fmt.Sprintf("%v", pathSlice[0])
				} else if pathStr, ok := path.(string); ok {
					config["path"] = pathStr
				}
			}
			if host := h2Opts["host"]; host != nil {
				if hostSlice, ok := host.([]interface{}); ok && len(hostSlice) > 0 {
					config["host"] = fmt.Sprintf("%v", hostSlice[0])
				} else if hostStr, ok := host.(string); ok {
					config["host"] = hostStr
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
		if shortID := GetString(realityOpts, "short-id"); shortID != "" {
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

	// Network type
	network := GetString(proxy, "network")
	if network != "" {
		params.Set("type", network)
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
		}
	}

	uri := fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		uuid, server, port, params.Encode(), url.QueryEscape(name))
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

	// Network type
	network := GetString(proxy, "network")
	if network != "" {
		params.Set("type", network)

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
			}
		}
	}

	uri := fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		password, server, port, params.Encode(), url.QueryEscape(name))
	return uri, nil
}

// encodeShadowsocks encodes Shadowsocks proxy to ss:// URI
func (p *URIProducer) encodeShadowsocks(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	cipher := GetString(proxy, "cipher")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	// Format: method:password
	userInfo := fmt.Sprintf("%s:%s", cipher, password)
	encoded := base64.URLEncoding.EncodeToString([]byte(userInfo))
	// Remove padding
	encoded = strings.TrimRight(encoded, "=")

	uri := fmt.Sprintf("ss://%s@%s:%d#%s", encoded, server, port, url.QueryEscape(name))
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
	if sni := GetString(proxy, "sni"); sni != "" {
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

	uri := fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s",
		password, server, port, params.Encode(), url.QueryEscape(name))
	return uri, nil
}

// encodeHysteria encodes Hysteria proxy to hysteria:// URI
func (p *URIProducer) encodeHysteria(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	params := url.Values{}

	// Auth
	if auth := GetString(proxy, "auth"); auth != "" {
		params.Set("auth", auth)
	}

	// Peer/SNI
	if peer := GetString(proxy, "peer"); peer != "" {
		params.Set("peer", peer)
	} else if sni := GetString(proxy, "sni"); sni != "" {
		params.Set("peer", sni)
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
	}

	// Up/Down speed
	if up := GetString(proxy, "up"); up != "" {
		params.Set("upmbps", up)
	}
	if down := GetString(proxy, "down"); down != "" {
		params.Set("downmbps", down)
	}

	uri := fmt.Sprintf("hysteria://%s@%s:%d?%s#%s",
		password, server, port, params.Encode(), url.QueryEscape(name))
	return uri, nil
}

// encodeTUIC encodes TUIC proxy to tuic:// URI
func (p *URIProducer) encodeTUIC(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	uuid := GetString(proxy, "uuid")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	params := url.Values{}

	// SNI
	if sni := GetString(proxy, "sni"); sni != "" {
		params.Set("sni", sni)
	}

	// Skip cert verify
	if GetBool(proxy, "skip-cert-verify") {
		params.Set("allow_insecure", "1")
	}

	// ALPN
	if alpn := GetStringSlice(proxy, "alpn"); len(alpn) > 0 {
		params.Set("alpn", strings.Join(alpn, ","))
	}

	// Congestion controller
	if cc := GetString(proxy, "congestion-controller"); cc != "" {
		params.Set("congestion_control", cc)
	}

	// UDP relay mode
	if udpMode := GetString(proxy, "udp-relay-mode"); udpMode != "" {
		params.Set("udp_relay_mode", udpMode)
	}

	// Password
	if password != "" {
		params.Set("password", password)
	}

	uri := fmt.Sprintf("tuic://%s@%s:%d?%s#%s",
		uuid, server, port, params.Encode(), url.QueryEscape(name))
	return uri, nil
}

// encodeSOCKS5 encodes SOCKS5 proxy to socks5:// URI
func (p *URIProducer) encodeSOCKS5(proxy Proxy) (string, error) {
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")
	username := GetString(proxy, "username")
	password := GetString(proxy, "password")
	name := GetString(proxy, "name")

	var auth string
	if username != "" && password != "" {
		auth = fmt.Sprintf("%s:%s@", username, password)
	}

	uri := fmt.Sprintf("socks5://%s%s:%d#%s", auth, server, port, url.QueryEscape(name))
	return uri, nil
}
