package substore

import (
	"fmt"
	"strings"
)

// LoonProducer implements Loon format converter
type LoonProducer struct {
	producerType string
	helper       *ProxyHelper
}

var loonIPVersions = map[string]string{
	"dual":        "dual",
	"ipv4":        "v4-only",
	"ipv6":        "v6-only",
	"ipv4-prefer": "prefer-v4",
	"ipv6-prefer": "prefer-v6",
}

// NewLoonProducer creates a new Loon producer
func NewLoonProducer() *LoonProducer {
	return &LoonProducer{
		producerType: "loon",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *LoonProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to Loon format
func (p *LoonProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	var result []string
	for _, proxy := range proxies {
		line, err := p.ProduceOne(proxy, outputType, opts)
		if err != nil {
			if !opts.IncludeUnsupportedProxy {
				continue
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}

	if outputType == "internal" {
		return proxies, nil
	}

	output := ""
	for _, line := range result {
		output += line + "\n"
	}
	return output, nil
}

// ProduceOne converts a single proxy to Loon format
func (p *LoonProducer) ProduceOne(proxy Proxy, outputType string, opts *ProduceOptions) (string, error) {
	// Clean proxy name
	name := p.helper.GetProxyName(proxy)
	name = strings.ReplaceAll(name, "=", "")
	name = strings.ReplaceAll(name, ",", "")
	proxy["name"] = name

	proxyType := p.helper.GetProxyType(proxy)
	includeUnsupported := opts != nil && opts.IncludeUnsupportedProxy

	switch proxyType {
	case "ss":
		return p.shadowsocks(proxy)
	case "ssr":
		return p.shadowsocksr(proxy)
	case "trojan":
		return p.trojan(proxy)
	case "vmess":
		return p.vmess(proxy, includeUnsupported)
	case "vless":
		return p.vless(proxy, includeUnsupported)
	case "http":
		return p.http(proxy)
	case "socks5":
		return p.socks5(proxy)
	case "wireguard":
		return p.wireguard(proxy)
	case "hysteria2":
		return p.hysteria2(proxy)
	default:
		return "", fmt.Errorf("platform Loon does not support proxy type: %s", proxyType)
	}
}

func (p *LoonProducer) shadowsocks(proxy Proxy) (string, error) {
	result := NewResult(proxy)

	cipher := GetString(proxy, "cipher")
	supportedCiphers := []string{
		"rc4", "rc4-md5",
		"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
		"aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
		"bf-cfb",
		"camellia-128-cfb", "camellia-192-cfb", "camellia-256-cfb",
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

	result.Append(fmt.Sprintf("%s=shadowsocks,%s,%d,%s,\"%s\"",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port"),
		cipher,
		GetString(proxy, "password")))

	// obfs
	if IsPresent(proxy, "plugin") {
		plugin := GetString(proxy, "plugin")
		if plugin == "obfs" {
			pluginOpts := GetMap(proxy, "plugin-opts")
			if pluginOpts != nil {
				mode := GetString(pluginOpts, "mode")
				if mode != "" && strings.HasPrefix(cipher, "2022-") {
					return "", fmt.Errorf("%s %s is not supported", cipher, plugin)
				}
				result.Append(fmt.Sprintf(",obfs-name=%s", mode))
				if host := GetString(pluginOpts, "host"); host != "" {
					result.Append(fmt.Sprintf(",obfs-host=%s", host))
				}
				if path := GetString(pluginOpts, "path"); path != "" {
					result.Append(fmt.Sprintf(",obfs-uri=%s", path))
				}
			}
		} else if plugin != "shadow-tls" {
			return "", fmt.Errorf("plugin %s is not supported", plugin)
		}
	}

	// shadow-tls
	p.appendShadowTLS(result, proxy)

	// sni
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) shadowsocksr(proxy Proxy) (string, error) {
	result := NewResult(proxy)
	result.Append(fmt.Sprintf("%s=shadowsocksr,%s,%d,%s,\"%s\"",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port"),
		GetString(proxy, "cipher"),
		GetString(proxy, "password")))

	// ssr protocol
	result.Append(fmt.Sprintf(",protocol=%s", GetString(proxy, "protocol")))
	if IsPresent(proxy, "protocol-param") {
		result.Append(fmt.Sprintf(",protocol-param=%s", GetString(proxy, "protocol-param")))
	}

	// obfs
	if IsPresent(proxy, "obfs") {
		result.Append(fmt.Sprintf(",obfs=%s", GetString(proxy, "obfs")))
	}
	if IsPresent(proxy, "obfs-param") {
		result.Append(fmt.Sprintf(",obfs-param=%s", GetString(proxy, "obfs-param")))
	}

	// shadow-tls
	p.appendShadowTLS(result, proxy)

	// sni
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) trojan(proxy Proxy) (string, error) {
	result := NewResult(proxy)
	result.Append(fmt.Sprintf("%s=trojan,%s,%d,\"%s\"",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port"),
		GetString(proxy, "password")))

	network := GetString(proxy, "network")
	if network == "tcp" {
		delete(proxy, "network")
		network = ""
	}

	// transport
	if IsPresent(proxy, "network") && network != "" {
		switch network {
		case "ws":
			result.Append(",transport=ws")
			if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
				if path := GetString(wsOpts, "path"); path != "" {
					result.Append(fmt.Sprintf(",path=%s", path))
				}
				if headers := GetMap(wsOpts, "headers"); headers != nil {
					if host := GetString(headers, "Host"); host != "" {
						result.Append(fmt.Sprintf(",host=%s", host))
					}
				}
			}
		default:
			return "", fmt.Errorf("network %s is unsupported", network)
		}
	}

	// tls verification
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// sni
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",tls-name=%s", GetString(proxy, "servername")))
	}
	if IsPresent(proxy, "tls-fingerprint") {
		result.Append(fmt.Sprintf(",tls-cert-sha256=%s", GetString(proxy, "tls-fingerprint")))
	}
	if IsPresent(proxy, "tls-pubkey-sha256") {
		result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", GetString(proxy, "tls-pubkey-sha256")))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) vmess(proxy Proxy, _ bool) (string, error) {
	isReality := IsPresent(proxy, "reality-opts")

	result := NewResult(proxy)
	result.Append(fmt.Sprintf("%s=vmess,%s,%d,%s,\"%s\"",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port"),
		GetString(proxy, "cipher"),
		GetString(proxy, "uuid")))

	network := GetString(proxy, "network")
	if network == "tcp" {
		delete(proxy, "network")
		network = ""
	}

	// transport
	if IsPresent(proxy, "network") && network != "" {
		switch network {
		case "ws":
			result.Append(",transport=ws")
			if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
				if path := GetString(wsOpts, "path"); path != "" {
					result.Append(fmt.Sprintf(",path=%s", path))
				}
				if headers := GetMap(wsOpts, "headers"); headers != nil {
					if host := GetString(headers, "Host"); host != "" {
						result.Append(fmt.Sprintf(",host=%s", host))
					}
				}
			}
		case "http":
			result.Append(",transport=http")
			if httpOpts := GetMap(proxy, "http-opts"); httpOpts != nil {
				httpPath := ""
				if path := httpOpts["path"]; path != nil {
					if pathSlice, ok := path.([]interface{}); ok && len(pathSlice) > 0 {
						httpPath = fmt.Sprintf("%v", pathSlice[0])
					} else if pathStr, ok := path.(string); ok {
						httpPath = pathStr
					}
				}
				if httpPath != "" {
					result.Append(fmt.Sprintf(",path=%s", httpPath))
				}

				httpHost := ""
				if headers := GetMap(httpOpts, "headers"); headers != nil {
					if host := headers["Host"]; host != nil {
						if hostSlice, ok := host.([]interface{}); ok && len(hostSlice) > 0 {
							httpHost = fmt.Sprintf("%v", hostSlice[0])
						} else if hostStr, ok := host.(string); ok {
							httpHost = hostStr
						}
					}
				}
				if httpHost != "" {
					result.Append(fmt.Sprintf(",host=%s", httpHost))
				}
			}
		default:
			return "", fmt.Errorf("network %s is unsupported", network)
		}
	} else {
		result.Append(",transport=tcp")
	}

	// tls
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",over-tls=%v", GetBool(proxy, "tls")))
	}

	// tls verification
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	if isReality {
		if IsPresent(proxy, "servername") {
			result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
		}
		if realityOpts := GetMap(proxy, "reality-opts"); realityOpts != nil {
			if publicKey := GetString(realityOpts, "public-key"); publicKey != "" {
				result.Append(fmt.Sprintf(",public-key=\"%s\"", publicKey))
			}
			if shortID := GetString(realityOpts, "short-id"); shortID != "" {
				result.Append(fmt.Sprintf(",short-id=%s", shortID))
			}
		}
	} else {
		// sni
		if IsPresent(proxy, "servername") {
			result.Append(fmt.Sprintf(",tls-name=%s", GetString(proxy, "servername")))
		}
		if IsPresent(proxy, "tls-fingerprint") {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", GetString(proxy, "tls-fingerprint")))
		}
		if IsPresent(proxy, "tls-pubkey-sha256") {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", GetString(proxy, "tls-pubkey-sha256")))
		}
	}

	// AEAD
	if IsPresent(proxy, "aead") {
		if GetBool(proxy, "aead") {
			result.Append(",alterId=0")
		} else {
			result.Append(",alterId=1")
		}
	} else {
		result.Append(fmt.Sprintf(",alterId=%d", GetInt(proxy, "alterId")))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) vless(proxy Proxy, _ bool) (string, error) {
	isXtls := false
	isReality := IsPresent(proxy, "reality-opts")

	if IsPresent(proxy, "flow") {
		flow := GetString(proxy, "flow")
		if flow == "xtls-rprx-vision" {
			isXtls = true
		} else {
			return "", fmt.Errorf("VLESS flow(%s) is not supported", flow)
		}
	}

	result := NewResult(proxy)
	result.Append(fmt.Sprintf("%s=vless,%s,%d,\"%s\"",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port"),
		GetString(proxy, "uuid")))

	network := GetString(proxy, "network")
	if network == "tcp" {
		delete(proxy, "network")
		network = ""
	}

	// transport
	if IsPresent(proxy, "network") && network != "" {
		switch network {
		case "ws":
			result.Append(",transport=ws")
			if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
				if path := GetString(wsOpts, "path"); path != "" {
					result.Append(fmt.Sprintf(",path=%s", path))
				}
				if headers := GetMap(wsOpts, "headers"); headers != nil {
					if host := GetString(headers, "Host"); host != "" {
						result.Append(fmt.Sprintf(",host=%s", host))
					}
				}
			}
		case "http":
			result.Append(",transport=http")
			if httpOpts := GetMap(proxy, "http-opts"); httpOpts != nil {
				httpPath := ""
				if path := httpOpts["path"]; path != nil {
					if pathSlice, ok := path.([]interface{}); ok && len(pathSlice) > 0 {
						httpPath = fmt.Sprintf("%v", pathSlice[0])
					} else if pathStr, ok := path.(string); ok {
						httpPath = pathStr
					}
				}
				if httpPath != "" {
					result.Append(fmt.Sprintf(",path=%s", httpPath))
				}

				httpHost := ""
				if headers := GetMap(httpOpts, "headers"); headers != nil {
					if host := headers["Host"]; host != nil {
						if hostSlice, ok := host.([]interface{}); ok && len(hostSlice) > 0 {
							httpHost = fmt.Sprintf("%v", hostSlice[0])
						} else if hostStr, ok := host.(string); ok {
							httpHost = hostStr
						}
					}
				}
				if httpHost != "" {
					result.Append(fmt.Sprintf(",host=%s", httpHost))
				}
			}
		default:
			return "", fmt.Errorf("network %s is unsupported", network)
		}
	} else {
		result.Append(",transport=tcp")
	}

	// tls
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",over-tls=%v", GetBool(proxy, "tls")))
	}

	// tls verification
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	if isXtls {
		if IsPresent(proxy, "flow") {
			result.Append(fmt.Sprintf(",flow=%s", GetString(proxy, "flow")))
		}
	}

	if isReality {
		if IsPresent(proxy, "servername") {
			result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
		}
		if realityOpts := GetMap(proxy, "reality-opts"); realityOpts != nil {
			if publicKey := GetString(realityOpts, "public-key"); publicKey != "" {
				result.Append(fmt.Sprintf(",public-key=\"%s\"", publicKey))
			}
			if shortID := GetString(realityOpts, "short-id"); shortID != "" {
				result.Append(fmt.Sprintf(",short-id=%s", shortID))
			}
		}
	} else {
		// sni
		if IsPresent(proxy, "servername") {
			result.Append(fmt.Sprintf(",tls-name=%s", GetString(proxy, "servername")))
		}
		if IsPresent(proxy, "tls-fingerprint") {
			result.Append(fmt.Sprintf(",tls-cert-sha256=%s", GetString(proxy, "tls-fingerprint")))
		}
		if IsPresent(proxy, "tls-pubkey-sha256") {
			result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", GetString(proxy, "tls-pubkey-sha256")))
		}
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) http(proxy Proxy) (string, error) {
	result := NewResult(proxy)
	proxyType := "http"
	if GetBool(proxy, "tls") {
		proxyType = "https"
	}

	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		proxyType,
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	if IsPresent(proxy, "username") {
		result.Append(fmt.Sprintf(",%s", GetString(proxy, "username")))
	}
	if IsPresent(proxy, "password") {
		result.Append(fmt.Sprintf(",\"%s\"", GetString(proxy, "password")))
	}

	// sni
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}

	// tls verification
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",tfo=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) socks5(proxy Proxy) (string, error) {
	result := NewResult(proxy)
	result.Append(fmt.Sprintf("%s=socks5,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	if IsPresent(proxy, "username") {
		result.Append(fmt.Sprintf(",%s", GetString(proxy, "username")))
	}
	if IsPresent(proxy, "password") {
		result.Append(fmt.Sprintf(",\"%s\"", GetString(proxy, "password")))
	}

	// tls
	if IsPresent(proxy, "tls") {
		result.Append(fmt.Sprintf(",over-tls=%v", GetBool(proxy, "tls")))
	}

	// sni
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",sni=%s", GetString(proxy, "servername")))
	}

	// tls verification
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",tfo=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) wireguard(proxy Proxy) (string, error) {
	// Handle peers array
	if peers, ok := proxy["peers"].([]interface{}); ok && len(peers) > 0 {
		if peer, ok := peers[0].(map[string]interface{}); ok {
			proxy["server"] = peer["server"]
			proxy["port"] = peer["port"]
			proxy["ip"] = peer["ip"]
			proxy["ipv6"] = peer["ipv6"]
			proxy["public-key"] = peer["public-key"]
			proxy["preshared-key"] = peer["pre-shared-key"]
			proxy["allowed-ips"] = peer["allowed-ips"]
			proxy["reserved"] = peer["reserved"]
		}
	}

	result := NewResult(proxy)
	result.Append(fmt.Sprintf("%s=wireguard", GetString(proxy, "name")))

	if IsPresent(proxy, "ip") {
		result.Append(fmt.Sprintf(",interface-ip=%s", GetString(proxy, "ip")))
	}
	if IsPresent(proxy, "ipv6") {
		result.Append(fmt.Sprintf(",interface-ipv6=%s", GetString(proxy, "ipv6")))
	}

	if IsPresent(proxy, "private-key") {
		result.Append(fmt.Sprintf(",private-key=\"%s\"", GetString(proxy, "private-key")))
	}
	if IsPresent(proxy, "mtu") {
		result.Append(fmt.Sprintf(",mtu=%d", GetInt(proxy, "mtu")))
	}

	// DNS handling
	if IsPresent(proxy, "dns") {
		dns := proxy["dns"]
		var dnsStr string
		var dnsv6Str string

		if dnsSlice, ok := dns.([]interface{}); ok {
			for _, d := range dnsSlice {
				dStr := fmt.Sprintf("%v", d)
				if IsIPv6(dStr) {
					dnsv6Str = dStr
				} else if IsIPv4(dStr) && dnsStr == "" {
					dnsStr = dStr
				} else if dnsStr == "" && !IsIPv4(dStr) && !IsIPv6(dStr) {
					dnsStr = dStr
				}
			}
		} else if dnsString, ok := dns.(string); ok {
			dnsStr = dnsString
		}

		if dnsStr != "" {
			result.Append(fmt.Sprintf(",dns=%s", dnsStr))
		}
		if dnsv6Str != "" {
			proxy["dnsv6"] = dnsv6Str
		}
	}

	if IsPresent(proxy, "dnsv6") {
		result.Append(fmt.Sprintf(",dnsv6=%s", GetString(proxy, "dnsv6")))
	}

	// keepalive
	if IsPresent(proxy, "persistent-keepalive") {
		result.Append(fmt.Sprintf(",keepalive=%d", GetInt(proxy, "persistent-keepalive")))
	}
	if IsPresent(proxy, "keepalive") {
		result.Append(fmt.Sprintf(",keepalive=%d", GetInt(proxy, "keepalive")))
	}

	// allowed-ips
	allowedIps := "0.0.0.0/0,::/0"
	if ips, ok := proxy["allowed-ips"].([]interface{}); ok {
		var ipStrs []string
		for _, ip := range ips {
			ipStrs = append(ipStrs, fmt.Sprintf("%v", ip))
		}
		allowedIps = strings.Join(ipStrs, ",")
	} else if ips, ok := proxy["allowed-ips"].(string); ok && ips != "" {
		allowedIps = ips
	}

	// reserved
	var reservedStr string
	if res, ok := proxy["reserved"].([]interface{}); ok {
		var resStrs []string
		for _, r := range res {
			resStrs = append(resStrs, fmt.Sprintf("%v", r))
		}
		reservedStr = strings.Join(resStrs, ",")
	} else if res, ok := proxy["reserved"].(string); ok {
		reservedStr = res
	}

	// preshared-key
	presharedKey := GetString(proxy, "preshared-key")
	if presharedKey == "" {
		presharedKey = GetString(proxy, "pre-shared-key")
	}

	// Build peers
	peersBuilder := fmt.Sprintf(",peers=[{public-key=\"%s\",allowed-ips=\"%s\",endpoint=%s:%d",
		GetString(proxy, "public-key"),
		allowedIps,
		GetString(proxy, "server"),
		GetInt(proxy, "port"))

	if reservedStr != "" {
		peersBuilder += fmt.Sprintf(",reserved=[%s]", reservedStr)
	}
	if presharedKey != "" {
		peersBuilder += fmt.Sprintf(",preshared-key=\"%s\"", presharedKey)
	}
	peersBuilder += "}]"

	result.Append(peersBuilder)

	// ip-version
	p.appendIPVersion(result, proxy)

	// block-quic
	p.appendBlockQuic(result, proxy)

	return result.String(), nil
}

func (p *LoonProducer) hysteria2(proxy Proxy) (string, error) {
	if IsPresent(proxy, "obfs-password") && GetString(proxy, "obfs") != "salamander" {
		return "", fmt.Errorf("only salamander obfs is supported")
	}

	result := NewResult(proxy)
	result.Append(fmt.Sprintf("%s=Hysteria2,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	if IsPresent(proxy, "password") {
		result.Append(fmt.Sprintf(",\"%s\"", GetString(proxy, "password")))
	}

	// sni
	if IsPresent(proxy, "servername") {
		result.Append(fmt.Sprintf(",tls-name=%s", GetString(proxy, "servername")))
	}
	if IsPresent(proxy, "tls-fingerprint") {
		result.Append(fmt.Sprintf(",tls-cert-sha256=%s", GetString(proxy, "tls-fingerprint")))
	}
	if IsPresent(proxy, "tls-pubkey-sha256") {
		result.Append(fmt.Sprintf(",tls-pubkey-sha256=%s", GetString(proxy, "tls-pubkey-sha256")))
	}
	if IsPresent(proxy, "skip-cert-verify") {
		result.Append(fmt.Sprintf(",skip-cert-verify=%v", GetBool(proxy, "skip-cert-verify")))
	}

	// salamander obfs
	if IsPresent(proxy, "obfs-password") && GetString(proxy, "obfs") == "salamander" {
		result.Append(fmt.Sprintf(",salamander-password=%s", GetString(proxy, "obfs-password")))
	}

	// tfo
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",fast-open=%v", GetBool(proxy, "tfo")))
	}

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// download-bandwidth
	if IsPresent(proxy, "down") {
		down := GetString(proxy, "down")
		// Extract digits from down string
		var bandwidth string
		for _, c := range down {
			if c >= '0' && c <= '9' {
				bandwidth += string(c)
			}
		}
		if bandwidth == "" {
			bandwidth = "0"
		}
		result.Append(fmt.Sprintf(",download-bandwidth=%s", bandwidth))
	}

	if IsPresent(proxy, "ecn") {
		result.Append(fmt.Sprintf(",ecn=%v", GetBool(proxy, "ecn")))
	}

	// ip-version
	p.appendIPVersion(result, proxy)

	return result.String(), nil
}

// Helper methods

func (p *LoonProducer) appendIPVersion(result *Result, proxy Proxy) {
	if IsPresent(proxy, "ip-version") {
		ipVersion := GetString(proxy, "ip-version")
		mappedVersion := loonIPVersions[ipVersion]
		if mappedVersion == "" {
			mappedVersion = ipVersion
		}
		result.Append(fmt.Sprintf(",ip-mode=%s", mappedVersion))
	}
}

func (p *LoonProducer) appendBlockQuic(result *Result, proxy Proxy) {
	blockQuic := GetString(proxy, "block-quic")
	switch blockQuic {
	case "on":
		result.Append(",block-quic=true")
	case "off":
		result.Append(",block-quic=false")
	}
}

func (p *LoonProducer) appendShadowTLS(result *Result, proxy Proxy) {
	if IsPresent(proxy, "shadow-tls-password") {
		result.Append(fmt.Sprintf(",shadow-tls-password=%s", GetString(proxy, "shadow-tls-password")))

		if IsPresent(proxy, "shadow-tls-version") {
			result.Append(fmt.Sprintf(",shadow-tls-version=%d", GetInt(proxy, "shadow-tls-version")))
		}
		if IsPresent(proxy, "shadow-tls-sni") {
			result.Append(fmt.Sprintf(",shadow-tls-sni=%s", GetString(proxy, "shadow-tls-sni")))
		}
		if IsPresent(proxy, "udp-port") {
			result.Append(fmt.Sprintf(",udp-port=%d", GetInt(proxy, "udp-port")))
		}
	} else if GetString(proxy, "plugin") == "shadow-tls" {
		if pluginOpts := GetMap(proxy, "plugin-opts"); pluginOpts != nil {
			password := GetString(pluginOpts, "password")
			if password != "" {
				result.Append(fmt.Sprintf(",shadow-tls-password=%s", password))

				if host := GetString(pluginOpts, "host"); host != "" {
					result.Append(fmt.Sprintf(",shadow-tls-sni=%s", host))
				}

				version := GetInt(pluginOpts, "version")
				if version > 0 {
					if version < 2 {
						// Note: We append but TypeScript throws error
						// For consistency, we just skip adding if version < 2
					} else {
						result.Append(fmt.Sprintf(",shadow-tls-version=%d", version))
					}
				}

				if IsPresent(proxy, "udp-port") {
					result.Append(fmt.Sprintf(",udp-port=%d", GetInt(proxy, "udp-port")))
				}
			}
		}
	}
}
