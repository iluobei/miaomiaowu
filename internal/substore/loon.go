package substore

import (
	"fmt"
	"regexp"
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
	// Check for unsupported ws network with v2ray-http-upgrade
	if GetString(proxy, "network") == "ws" {
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if GetBool(wsOpts, "v2ray-http-upgrade") {
				return "", fmt.Errorf("platform Loon does not support network ws with http upgrade")
			}
		}
	}

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
				result.AppendIfPresent(",obfs-host=%s", "plugin-opts.host")
				result.AppendIfPresent(",obfs-uri=%s", "plugin-opts.path")
			}
		} else if plugin != "shadow-tls" {
			return "", fmt.Errorf("plugin %s is not supported", plugin)
		}
	}

	// shadow-tls
	if err := p.appendShadowTLS(result, proxy); err != nil {
		return "", err
	}

	// tfo
	result.AppendIfPresent(",fast-open=%v", "tfo")

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
	result.AppendIfPresent(",protocol-param=%s", "protocol-param")

	// obfs
	result.AppendIfPresent(",obfs=%s", "obfs")
	result.AppendIfPresent(",obfs-param=%s", "obfs-param")

	// shadow-tls
	if err := p.appendShadowTLS(result, proxy); err != nil {
		return "", err
	}

	// tfo
	result.AppendIfPresent(",fast-open=%v", "tfo")

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
	}

	// transport
	if IsPresent(proxy, "network") {
		if proxy["network"] == "ws" {
			result.Append(",transport=ws")
			result.AppendIfPresent(",path=%s", "ws-opts.path")
			result.AppendIfPresent(",host=%s", "ws-opts.headers.Host")
		} else {
			return "", fmt.Errorf("network %s is unsupported", GetString(proxy, "network"))
		}
	}

	// tls verification
	result.AppendIfPresent(",skip-cert-verify=%v", "skip-cert-verify")

	// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
	if sni := GetSNI(proxy); sni != "" {
		result.Append(fmt.Sprintf(",tls-name=%s", sni))
	}
	result.AppendIfPresent(",tls-cert-sha256=%s", "tls-fingerprint")
	result.AppendIfPresent(",tls-pubkey-sha256=%s", "tls-pubkey-sha256")

	// tfo
	result.AppendIfPresent(",fast-open=%v", "tfo")

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
	}

	// transport
	if IsPresent(proxy, "network") {
		switch proxy["network"] {
		case "ws":
			result.Append(",transport=ws")
			result.AppendIfPresent(",path=%s", "ws-opts.path")
			result.AppendIfPresent(",host=%s", "ws-opts.headers.Host")
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
			return "", fmt.Errorf("network %s is unsupported", GetString(proxy, "network"))
		}
	} else {
		result.Append(",transport=tcp")
	}

	// tls
	result.AppendIfPresent(",over-tls=%v", "tls")

	// tls verification
	result.AppendIfPresent(",skip-cert-verify=%v", "skip-cert-verify")

	if isReality {
		// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
		if sni := GetSNI(proxy); sni != "" {
			result.Append(fmt.Sprintf(",sni=%s", sni))
		}
		result.AppendIfPresent(",public-key=\"%s\"", "reality-opts.public-key")
		result.AppendIfPresent(",short-id=%s", "reality-opts.short-id")
	} else {
		// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
		if sni := GetSNI(proxy); sni != "" {
			result.Append(fmt.Sprintf(",tls-name=%s", sni))
		}
		result.AppendIfPresent(",tls-cert-sha256=%s", "tls-fingerprint")
		result.AppendIfPresent(",tls-pubkey-sha256=%s", "tls-pubkey-sha256")
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
	result.AppendIfPresent(",fast-open=%v", "tfo")

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
	// Check encryption
	if IsPresent(proxy, "encryption") && GetString(proxy, "encryption") != "none" {
		return "", fmt.Errorf("VLESS encryption is not supported")
	}

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
	}

	// transport
	if IsPresent(proxy, "network") {
		switch proxy["network"] {
		case "ws":
			result.Append(",transport=ws")
			result.AppendIfPresent(",path=%s", "ws-opts.path")
			result.AppendIfPresent(",host=%s", "ws-opts.headers.Host")
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
			return "", fmt.Errorf("network %s is unsupported", GetString(proxy, "network"))
		}
	} else {
		result.Append(",transport=tcp")
	}

	// tls
	result.AppendIfPresent(",over-tls=%v", "tls")

	// tls verification
	result.AppendIfPresent(",skip-cert-verify=%v", "skip-cert-verify")

	if isXtls {
		result.AppendIfPresent(",flow=%s", "flow")
	}

	if isReality {
		// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
		if sni := GetSNI(proxy); sni != "" {
			result.Append(fmt.Sprintf(",sni=%s", sni))
		}
		result.AppendIfPresent(",public-key=\"%s\"", "reality-opts.public-key")
		result.AppendIfPresent(",short-id=%s", "reality-opts.short-id")
	} else {
		// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
		if sni := GetSNI(proxy); sni != "" {
			result.Append(fmt.Sprintf(",tls-name=%s", sni))
		}
		result.AppendIfPresent(",tls-cert-sha256=%s", "tls-fingerprint")
		result.AppendIfPresent(",tls-pubkey-sha256=%s", "tls-pubkey-sha256")
	}

	// tfo
	result.AppendIfPresent(",fast-open=%v", "tfo")

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

	result.AppendIfPresent(",%s", "username")
	result.AppendIfPresent(",\"%s\"", "password")

	// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
	if sni := GetSNI(proxy); sni != "" {
		result.Append(fmt.Sprintf(",sni=%s", sni))
	}

	// tls verification
	result.AppendIfPresent(",skip-cert-verify=%v", "skip-cert-verify")

	// tfo
	result.AppendIfPresent(",tfo=%v", "tfo")

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

	result.AppendIfPresent(",%s", "username")
	result.AppendIfPresent(",\"%s\"", "password")

	// tls
	result.AppendIfPresent(",over-tls=%v", "tls")

	// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
	if sni := GetSNI(proxy); sni != "" {
		result.Append(fmt.Sprintf(",sni=%s", sni))
	}

	// tls verification
	result.AppendIfPresent(",skip-cert-verify=%v", "skip-cert-verify")

	// tfo
	result.AppendIfPresent(",tfo=%v", "tfo")

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

	result.AppendIfPresent(",interface-ip=%s", "ip")
	result.AppendIfPresent(",interface-ipv6=%s", "ipv6")

	result.AppendIfPresent(",private-key=\"%s\"", "private-key")
	result.AppendIfPresent(",mtu=%d", "mtu")

	// DNS handling
	if IsPresent(proxy, "dns") {
		dns := proxy["dns"]
		if dnsSlice, ok := dns.([]interface{}); ok {
			var dnsv6 string
			var dnsStr string
			for _, d := range dnsSlice {
				dStr := fmt.Sprintf("%v", d)
				if IsIPv6(dStr) {
					dnsv6 = dStr
				} else if IsIPv4(dStr) {
					if dnsStr == "" {
						dnsStr = dStr
					}
				} else {
					if dnsStr == "" {
						dnsStr = dStr
					}
				}
			}
			if dnsStr != "" {
				proxy["dns"] = dnsStr
			}
			if dnsv6 != "" {
				proxy["dnsv6"] = dnsv6
			}
		}
	}
	result.AppendIfPresent(",dns=%s", "dns")
	result.AppendIfPresent(",dnsv6=%s", "dnsv6")

	// keepalive
	result.AppendIfPresent(",keepalive=%d", "persistent-keepalive")
	result.AppendIfPresent(",keepalive=%d", "keepalive")

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

	result.AppendIfPresent(",\"%s\"", "password")

	// SNI - compatible with both SubStore's "sni" and miaomiaowu's "servername"
	if sni := GetSNI(proxy); sni != "" {
		result.Append(fmt.Sprintf(",tls-name=%s", sni))
	}
	result.AppendIfPresent(",tls-cert-sha256=%s", "tls-fingerprint")
	result.AppendIfPresent(",tls-pubkey-sha256=%s", "tls-pubkey-sha256")
	result.AppendIfPresent(",skip-cert-verify=%v", "skip-cert-verify")

	// salamander obfs
	if IsPresent(proxy, "obfs-password") && GetString(proxy, "obfs") == "salamander" {
		result.Append(fmt.Sprintf(",salamander-password=%s", GetString(proxy, "obfs-password")))
	}

	// tfo
	result.AppendIfPresent(",fast-open=%v", "tfo")

	// block-quic
	p.appendBlockQuic(result, proxy)

	// udp
	if GetBool(proxy, "udp") {
		result.Append(",udp=true")
	}

	// download-bandwidth
	if IsPresent(proxy, "down") {
		down := GetString(proxy, "down")
		// Extract digits from down string (matches /\d+/ in JS)
		re := regexp.MustCompile(`\d+`)
		matches := re.FindString(down)
		if matches == "" {
			matches = "0"
		}
		result.Append(fmt.Sprintf(",download-bandwidth=%s", matches))
	}

	result.AppendIfPresent(",ecn=%v", "ecn")

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

func (p *LoonProducer) appendShadowTLS(result *Result, proxy Proxy) error {
	if IsPresent(proxy, "shadow-tls-password") {
		result.Append(fmt.Sprintf(",shadow-tls-password=%s", GetString(proxy, "shadow-tls-password")))
		result.AppendIfPresent(",shadow-tls-version=%d", "shadow-tls-version")
		result.AppendIfPresent(",shadow-tls-sni=%s", "shadow-tls-sni")
		result.AppendIfPresent(",udp-port=%d", "udp-port")
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
						return fmt.Errorf("shadow-tls version %d is not supported", version)
					}
					result.Append(fmt.Sprintf(",shadow-tls-version=%d", version))
				}

				result.AppendIfPresent(",udp-port=%d", "udp-port")
			}
		}
	}
	return nil
}
