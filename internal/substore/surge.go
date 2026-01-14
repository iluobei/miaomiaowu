package substore

import (
	"fmt"
	"regexp"
	"strings"
)

// SurgeProducer implements Surge format converter
type SurgeProducer struct {
	producerType string
	helper       *ProxyHelper
}

var ipVersions = map[string]string{
	"dual":        "dual",
	"ipv4":        "v4-only",
	"ipv6":        "v6-only",
	"ipv4-prefer": "prefer-v4",
	"ipv6-prefer": "prefer-v6",
}

// NewSurgeProducer creates a new Surge producer
func NewSurgeProducer() *SurgeProducer {
	return &SurgeProducer{
		producerType: "surge",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *SurgeProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to Surge format
func (p *SurgeProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	var result []string
	for _, proxy := range proxies {
		line, err := p.ProduceOne(proxy, outputType, opts)

		// convert dailer-proxy to underlying-proxy
		dailerProxy := GetString(proxy, "dialer-proxy")
		if dailerProxy != "" {
			line += fmt.Sprintf(", underlying-proxy=%s", dailerProxy)
		}

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

// ProduceOne converts a single proxy to Surge format
func (p *SurgeProducer) ProduceOne(proxy Proxy, outputType string, opts *ProduceOptions) (string, error) {
	// Check for unsupported ws network with v2ray-http-upgrade
	if GetString(proxy, "network") == "ws" {
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if GetBool(wsOpts, "v2ray-http-upgrade") {
				return "", fmt.Errorf("platform Surge does not support network ws with http upgrade")
			}
		}
	}

	// Clean proxy name
	name := p.helper.GetProxyName(proxy)
	name = strings.ReplaceAll(name, "=", "")
	name = strings.ReplaceAll(name, ",", "")
	proxy["name"] = name

	// Convert ports to string if present
	if IsPresent(proxy, "ports") {
		proxy["ports"] = fmt.Sprintf("%v", proxy["ports"])
	}

	proxyType := p.helper.GetProxyType(proxy)
	includeUnsupported := opts != nil && opts.IncludeUnsupportedProxy

	switch proxyType {
	case "ss":
		return p.shadowsocks(proxy, includeUnsupported)
	case "trojan":
		return p.trojan(proxy)
	case "vmess":
		return p.vmess(proxy, includeUnsupported)
	case "http":
		return p.http(proxy)
	case "direct":
		return p.direct(proxy)
	case "socks5":
		return p.socks5(proxy)
	case "snell":
		return p.snell(proxy)
	case "tuic":
		return p.tuic(proxy)
	case "wireguard-surge":
		return p.wireguardSurge(proxy)
	case "hysteria2":
		return p.hysteria2(proxy, includeUnsupported)
	case "ssh":
		return p.ssh(proxy)
	case "wireguard":
		if includeUnsupported {
			return p.wireguard(proxy)
		}
		return "", fmt.Errorf("platform Surge does not support proxy type: %s", proxyType)
	case "anytls":
		if includeUnsupported {
			network := GetString(proxy, "network")
			if network != "" && network != "tcp" {
				return "", fmt.Errorf("platform Surge does not support proxy type %s with network or reality", proxyType)
			}
			if network == "tcp" && IsPresent(proxy, "reality-opts") {
				return "", fmt.Errorf("platform Surge does not support proxy type %s with network or reality", proxyType)
			}
			return p.anytls(proxy)
		}
		return "", fmt.Errorf("platform Surge does not support proxy type: %s", proxyType)
	default:
		return "", fmt.Errorf("platform Surge does not support proxy type: %s", proxyType)
	}
}

func (p *SurgeProducer) shadowsocks(proxy Proxy, _ bool) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "type"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	cipher := GetString(proxy, "cipher")
	if cipher == "" {
		cipher = "none"
	}

	supportedCiphers := []string{
		"aes-128-gcm", "aes-192-gcm", "aes-256-gcm",
		"chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
		"rc4", "rc4-md5",
		"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
		"aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
		"bf-cfb", "camellia-128-cfb", "camellia-192-cfb", "camellia-256-cfb",
		"cast5-cfb", "des-cfb", "idea-cfb", "rc2-cfb", "seed-cfb",
		"salsa20", "chacha20", "chacha20-ietf", "none",
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

	result.Append(fmt.Sprintf(",encrypt-method=%s", cipher))
	result.AppendIfPresent(`,password="%s"`, "password")

	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)

	// obfs
	if IsPresent(proxy, "plugin") {
		plugin := GetString(proxy, "plugin")
		if plugin == "obfs" {
			pluginOpts := GetMap(proxy, "plugin-opts")
			if pluginOpts != nil {
				result.Append(fmt.Sprintf(",obfs=%s", GetString(pluginOpts, "mode")))
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

	p.appendShadowTLS(result, proxy)
	return result.String(), nil
}

func (p *SurgeProducer) trojan(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "type"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,password="%s"`, "password")
	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)
	p.handleTransport(result, proxy, false)
	p.appendTLS(result, proxy)
	p.appendShadowTLS(result, proxy)

	return result.String(), nil
}

func (p *SurgeProducer) anytls(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "type"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,password="%s"`, "password")
	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)
	p.appendTLS(result, proxy)
	result.AppendIfPresent(`,reuse=%v`, "reuse")

	return result.String(), nil
}

func (p *SurgeProducer) vmess(proxy Proxy, includeUnsupported bool) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "type"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,username=%s`, "uuid")
	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)
	p.handleTransport(result, proxy, includeUnsupported)

	// AEAD
	if IsPresent(proxy, "aead") {
		result.Append(fmt.Sprintf(",vmess-aead=%v", GetBool(proxy, "aead")))
	} else {
		result.Append(fmt.Sprintf(",vmess-aead=%v", GetInt(proxy, "alterId") == 0))
	}

	p.appendTLS(result, proxy)
	p.appendShadowTLS(result, proxy)

	return result.String(), nil
}

func (p *SurgeProducer) http(proxy Proxy) (string, error) {
	if headers := GetMap(proxy, "headers"); len(headers) > 0 {
		return "", fmt.Errorf("headers is unsupported")
	}

	result := &Result{Proxy: proxy}
	proxyType := "http"
	if GetBool(proxy, "tls") {
		proxyType = "https"
	}

	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		proxyType,
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,username="%s"`, "username")
	result.AppendIfPresent(`,password="%s"`, "password")
	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)
	p.appendTLS(result, proxy)
	p.appendShadowTLS(result, proxy)

	return result.String(), nil
}

func (p *SurgeProducer) direct(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=direct", GetString(proxy, "name")))

	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)

	return result.String(), nil
}

func (p *SurgeProducer) socks5(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}
	proxyType := "socks5"
	if GetBool(proxy, "tls") {
		proxyType = "socks5-tls"
	}

	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		proxyType,
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,username="%s"`, "username")
	result.AppendIfPresent(`,password="%s"`, "password")
	p.appendIPVersion(result, proxy)

	// Note: tfo is not supported by Surge for socks5, so we skip it here
	result.AppendIfPresent(`,no-error-alert=%v`, "no-error-alert")
	result.AppendIfPresent(`,udp-relay=%v`, "udp")
	result.AppendIfPresent(`,test-url=%s`, "test-url")
	result.AppendIfPresent(`,test-timeout=%d`, "test-timeout")
	result.AppendIfPresent(`,test-udp=%s`, "test-udp")
	result.AppendIfPresent(`,hybrid=%s`, "hybrid")
	result.AppendIfPresent(`,tos=%s`, "tos")
	result.AppendIfPresent(`,allow-other-interface=%s`, "allow-other-interface")
	result.AppendIfPresent(`,interface=%s`, "interface-name")
	result.AppendIfPresent(`,interface=%s`, "interface")
	result.AppendIfPresent(`,block-quic=%s`, "block-quic")
	result.AppendIfPresent(`,underlying-proxy=%s`, "underlying-proxy")

	p.appendTLS(result, proxy)
	p.appendShadowTLS(result, proxy)

	return result.String(), nil
}

func (p *SurgeProducer) snell(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "type"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,version=%d`, "version")
	result.AppendIfPresent(`,psk=%s`, "psk")
	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)

	// obfs
	if obfsOpts := GetMap(proxy, "obfs-opts"); obfsOpts != nil {
		if mode := GetString(obfsOpts, "mode"); mode != "" {
			result.Append(fmt.Sprintf(",obfs=%s", mode))
		}
		if host := GetString(obfsOpts, "host"); host != "" {
			result.Append(fmt.Sprintf(",obfs-host=%s", host))
		}
		if path := GetString(obfsOpts, "path"); path != "" {
			result.Append(fmt.Sprintf(",obfs-uri=%s", path))
		}
	}

	p.appendShadowTLS(result, proxy)
	result.AppendIfPresent(`,reuse=%v`, "reuse")

	return result.String(), nil
}

func (p *SurgeProducer) tuic(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}

	proxyType := GetString(proxy, "type")
	token := GetString(proxy, "token")
	if token == "" {
		proxyType = "tuic-v5"
	}

	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		proxyType,
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,uuid=%s`, "uuid")
	result.AppendIfPresent(`,password="%s"`, "password")
	result.AppendIfPresent(`,token=%s`, "token")

	// alpn
	if IsPresent(proxy, "alpn") {
		alpn := proxy["alpn"]
		if alpnSlice, ok := alpn.([]interface{}); ok && len(alpnSlice) > 0 {
			result.Append(fmt.Sprintf(",alpn=%v", alpnSlice[0]))
		} else if alpnStr, ok := alpn.(string); ok {
			result.Append(fmt.Sprintf(",alpn=%s", alpnStr))
		}
	}

	// port hopping
	if IsPresent(proxy, "ports") {
		ports := strings.ReplaceAll(GetString(proxy, "ports"), ",", ";")
		result.Append(fmt.Sprintf(`,port-hopping="%s"`, ports))
	}
	result.AppendIfPresent(`,port-hopping-interval=%s`, "hop-interval")

	p.appendIPVersion(result, proxy)

	// Common options except tfo (we handle it separately below)
	result.AppendIfPresent(`,no-error-alert=%v`, "no-error-alert")
	result.AppendIfPresent(`,udp-relay=%v`, "udp")
	result.AppendIfPresent(`,test-url=%s`, "test-url")
	result.AppendIfPresent(`,test-timeout=%d`, "test-timeout")
	result.AppendIfPresent(`,test-udp=%s`, "test-udp")
	result.AppendIfPresent(`,hybrid=%s`, "hybrid")
	result.AppendIfPresent(`,tos=%s`, "tos")
	result.AppendIfPresent(`,allow-other-interface=%s`, "allow-other-interface")
	result.AppendIfPresent(`,interface=%s`, "interface-name")
	result.AppendIfPresent(`,interface=%s`, "interface")
	result.AppendIfPresent(`,block-quic=%s`, "block-quic")
	result.AppendIfPresent(`,underlying-proxy=%s`, "underlying-proxy")

	p.appendTLS(result, proxy)
	p.appendShadowTLS(result, proxy)

	// tfo: prefer tfo, fallback to fast-open
	if IsPresent(proxy, "tfo") {
		result.Append(fmt.Sprintf(",tfo=%v", GetBool(proxy, "tfo")))
	} else if IsPresent(proxy, "fast-open") {
		result.Append(fmt.Sprintf(",tfo=%v", GetBool(proxy, "fast-open")))
	}

	result.AppendIfPresent(`,ecn=%v`, "ecn")

	return result.String(), nil
}

func (p *SurgeProducer) wireguard(proxy Proxy) (string, error) {
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

	result := &Result{Proxy: proxy}
	name := GetString(proxy, "name")

	result.Append(fmt.Sprintf("# > WireGuard Proxy %s\n# %s=wireguard", name, name))

	sectionName := GetString(proxy, "section-name")
	if sectionName == "" {
		sectionName = name
		proxy["section-name"] = sectionName
	}

	result.AppendIfPresent(`,section-name=%s`, "section-name")
	p.appendCommonOptions(result, proxy)

	ipVersion := ipVersions[GetString(proxy, "ip-version")]
	if ipVersion == "" {
		ipVersion = GetString(proxy, "ip-version")
	}

	result.Append(fmt.Sprintf("\n# > WireGuard Section %s\n[WireGuard %s]\nprivate-key = %s",
		name, sectionName, GetString(proxy, "private-key")))

	result.AppendIfPresent(`\nself-ip = %s`, "ip")
	result.AppendIfPresent(`\nself-ip-v6 = %s`, "ipv6")

	// DNS
	if IsPresent(proxy, "dns") {
		dns := proxy["dns"]
		if dnsSlice, ok := dns.([]interface{}); ok {
			var dnsStrs []string
			for _, d := range dnsSlice {
				dnsStrs = append(dnsStrs, fmt.Sprintf("%v", d))
			}
			result.Append(fmt.Sprintf("\ndns-server = %s", strings.Join(dnsStrs, ", ")))
		} else if dnsStr, ok := dns.(string); ok {
			result.Append(fmt.Sprintf("\ndns-server = %s", dnsStr))
		}
	}

	result.AppendIfPresent(`\nmtu = %d`, "mtu")

	if ipVersion == "prefer-v6" {
		result.Append("\nprefer-ipv6 = true")
	}

	// allowed-ips
	var allowedIps string
	if ips, ok := proxy["allowed-ips"].([]interface{}); ok {
		var ipStrs []string
		for _, ip := range ips {
			ipStrs = append(ipStrs, fmt.Sprintf("%v", ip))
		}
		allowedIps = strings.Join(ipStrs, ",")
	} else if ips, ok := proxy["allowed-ips"].(string); ok {
		allowedIps = ips
	}

	// reserved
	var reserved string
	if res, ok := proxy["reserved"].([]interface{}); ok {
		var resStrs []string
		for _, r := range res {
			resStrs = append(resStrs, fmt.Sprintf("%v", r))
		}
		reserved = strings.Join(resStrs, "/")
	} else if res, ok := proxy["reserved"].(string); ok {
		reserved = res
	}

	// preshared-key
	presharedKey := GetString(proxy, "preshared-key")
	if presharedKey == "" {
		presharedKey = GetString(proxy, "pre-shared-key")
	}

	// Build peer
	peer := make(map[string]string)
	peer["public-key"] = GetString(proxy, "public-key")
	if allowedIps != "" {
		peer["allowed-ips"] = fmt.Sprintf(`"%s"`, allowedIps)
	}
	peer["endpoint"] = fmt.Sprintf("%s:%d", GetString(proxy, "server"), GetInt(proxy, "port"))

	keepalive := GetInt(proxy, "persistent-keepalive")
	if keepalive == 0 {
		keepalive = GetInt(proxy, "keepalive")
	}
	if keepalive > 0 {
		peer["keepalive"] = fmt.Sprintf("%d", keepalive)
	}
	if reserved != "" {
		peer["client-id"] = reserved
	}
	if presharedKey != "" {
		peer["preshared-key"] = presharedKey
	}

	var peerParts []string
	for k, v := range peer {
		if v != "" {
			peerParts = append(peerParts, fmt.Sprintf("%s = %s", k, v))
		}
	}
	result.Append(fmt.Sprintf("\npeer = (%s)", strings.Join(peerParts, ", ")))

	return result.String(), nil
}

func (p *SurgeProducer) wireguardSurge(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=wireguard", GetString(proxy, "name")))

	result.AppendIfPresent(`,section-name=%s`, "section-name")
	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)

	return result.String(), nil
}

func (p *SurgeProducer) hysteria2(proxy Proxy, includeUnsupported bool) (string, error) {
	// Check obfs support
	if includeUnsupported {
		if IsPresent(proxy, "obfs-password") && GetString(proxy, "obfs") != "salamander" {
			return "", fmt.Errorf("only salamander obfs is supported")
		}
	} else {
		if IsPresent(proxy, "obfs") || IsPresent(proxy, "obfs-password") {
			return "", fmt.Errorf("obfs is unsupported")
		}
	}

	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=%s,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "type"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,password="%s"`, "password")

	// port hopping
	if IsPresent(proxy, "ports") {
		ports := strings.ReplaceAll(GetString(proxy, "ports"), ",", ";")
		result.Append(fmt.Sprintf(`,port-hopping="%s"`, ports))
	}
	result.AppendIfPresent(`,port-hopping-interval=%s`, "hop-interval")

	// salamander obfs
	if IsPresent(proxy, "obfs-password") && GetString(proxy, "obfs") == "salamander" {
		result.Append(fmt.Sprintf(`,salamander-password="%s"`, GetString(proxy, "obfs-password")))
	}

	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)
	p.appendTLS(result, proxy)
	p.appendShadowTLS(result, proxy)

	// download-bandwidth
	if down := GetString(proxy, "down"); down != "" {
		re := regexp.MustCompile(`\d+`)
		if match := re.FindString(down); match != "" {
			result.Append(fmt.Sprintf(",download-bandwidth=%s", match))
		}
	}

	result.AppendIfPresent(`,ecn=%v`, "ecn")

	return result.String(), nil
}

func (p *SurgeProducer) ssh(proxy Proxy) (string, error) {
	result := &Result{Proxy: proxy}
	result.Append(fmt.Sprintf("%s=ssh,%s,%d",
		GetString(proxy, "name"),
		GetString(proxy, "server"),
		GetInt(proxy, "port")))

	result.AppendIfPresent(`,username="%s"`, "username")
	result.AppendIfPresent(`,password="%s"`, "password")
	result.AppendIfPresent(`,private-key=%s`, "keystore-private-key")
	result.AppendIfPresent(`,idle-timeout=%s`, "idle-timeout")
	result.AppendIfPresent(`,server-fingerprint="%s"`, "server-fingerprint")

	p.appendIPVersion(result, proxy)
	p.appendCommonOptions(result, proxy)

	return result.String(), nil
}

// Helper methods

func (p *SurgeProducer) appendIPVersion(result *Result, proxy Proxy) {
	if ipVer := GetString(proxy, "ip-version"); ipVer != "" {
		mappedVersion := ipVersions[ipVer]
		if mappedVersion == "" {
			mappedVersion = ipVer
		}
		result.Append(fmt.Sprintf(",ip-version=%s", mappedVersion))
	}
}

func (p *SurgeProducer) appendCommonOptions(result *Result, _ Proxy) {
	result.AppendIfPresent(`,no-error-alert=%v`, "no-error-alert")
	result.AppendIfPresent(`,tfo=%v`, "tfo")
	result.AppendIfPresent(`,udp-relay=%v`, "udp")
	result.AppendIfPresent(`,test-url=%s`, "test-url")
	result.AppendIfPresent(`,test-timeout=%d`, "test-timeout")
	result.AppendIfPresent(`,test-udp=%s`, "test-udp")
	result.AppendIfPresent(`,hybrid=%s`, "hybrid")
	result.AppendIfPresent(`,tos=%s`, "tos")
	result.AppendIfPresent(`,allow-other-interface=%s`, "allow-other-interface")
	result.AppendIfPresent(`,interface=%s`, "interface-name")
	result.AppendIfPresent(`,interface=%s`, "interface")
	result.AppendIfPresent(`,block-quic=%s`, "block-quic")
	result.AppendIfPresent(`,underlying-proxy=%s`, "underlying-proxy")
}

func (p *SurgeProducer) appendTLS(result *Result, _ Proxy) {
	result.AppendIfPresent(`,tls=%v`, "tls")
	result.AppendIfPresent(`,server-cert-fingerprint-sha256=%s`, "tls-fingerprint")
	result.AppendIfPresent(`,sni=%s`, "sni")
	result.AppendIfPresent(`,skip-cert-verify=%v`, "skip-cert-verify")
}

func (p *SurgeProducer) appendShadowTLS(result *Result, proxy Proxy) {
	if IsPresent(proxy, "shadow-tls-password") {
		result.Append(fmt.Sprintf(",shadow-tls-password=%s", GetString(proxy, "shadow-tls-password")))
		result.AppendIfPresent(`,shadow-tls-version=%d`, "shadow-tls-version")
		result.AppendIfPresent(`,shadow-tls-sni=%s`, "shadow-tls-sni")
		result.AppendIfPresent(`,udp-port=%d`, "udp-port")
	} else if GetString(proxy, "plugin") == "shadow-tls" {
		if pluginOpts := GetMap(proxy, "plugin-opts"); pluginOpts != nil {
			if password := GetString(pluginOpts, "password"); password != "" {
				result.Append(fmt.Sprintf(",shadow-tls-password=%s", password))
				if host := GetString(pluginOpts, "host"); host != "" {
					result.Append(fmt.Sprintf(",shadow-tls-sni=%s", host))
				}
				if version := GetInt(pluginOpts, "version"); version > 0 {
					if version < 2 {
						// Note: version < 2 is not supported, but we don't error here
						// to match JS behavior which just doesn't add the version field
					} else {
						result.Append(fmt.Sprintf(",shadow-tls-version=%d", version))
					}
				}
				result.AppendIfPresent(`,udp-port=%d`, "udp-port")
			}
		}
	}
}

func (p *SurgeProducer) handleTransport(result *Result, proxy Proxy, includeUnsupported bool) error {
	if !IsPresent(proxy, "network") {
		return nil
	}

	network := GetString(proxy, "network")
	if network == "ws" {
		result.Append(",ws=true")
		if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
			if path := GetString(wsOpts, "path"); path != "" {
				result.Append(fmt.Sprintf(",ws-path=%s", path))
			}
			if headers := GetMap(wsOpts, "headers"); headers != nil {
				var headerParts []string
				for k, v := range headers {
					headerParts = append(headerParts, fmt.Sprintf("%s:\"%v\"", k, v))
				}
				if len(headerParts) > 0 {
					result.Append(fmt.Sprintf(",ws-headers=%s", strings.Join(headerParts, "|")))
				}
			}
		}
	} else if network == "http" && includeUnsupported {
		// Include unsupported: network http -> tcp
	} else if network == "tcp" && IsPresent(proxy, "reality-opts") {
		return fmt.Errorf("reality is unsupported")
	} else if network != "" && network != "tcp" {
		return fmt.Errorf("network %s is unsupported", network)
	}

	return nil
}
