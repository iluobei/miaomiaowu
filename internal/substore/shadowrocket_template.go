package substore

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ShadowrocketTemplateProducer implements Shadowrocket format converter
type ShadowrocketTemplateProducer struct {
	producerType string
	helper       *ProxyHelper
}

// NewShadowrocketTemplateProducer creates a new Shadowrocket template producer
func NewShadowrocketTemplateProducer() *ShadowrocketTemplateProducer {
	return &ShadowrocketTemplateProducer{
		producerType: "shadowrocket",
		helper:       NewProxyHelper(),
	}
}

// GetType returns the producer type
func (p *ShadowrocketTemplateProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to Shadowrocket format
func (p *ShadowrocketTemplateProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	if opts == nil {
		opts = &ProduceOptions{}
	}

	// 如果只需要内部格式或没有完整配置，使用原有的简化逻辑
	if outputType == "internal" || opts.FullConfig == nil {
		return p.produceProxiesOnly(proxies, outputType, opts)
	}

	// 生成完整的 Shadowrocket 配置
	return p.produceFullConfig(proxies, opts)
}

// produceProxiesOnly 只生成节点部分（向后兼容）
func (p *ShadowrocketTemplateProducer) produceProxiesOnly(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	result := p.transformProxies(proxies, opts)

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

// produceFullConfig 生成完整的 Shadowrocket 配置文件
func (p *ShadowrocketTemplateProducer) produceFullConfig(proxies []Proxy, opts *ProduceOptions) (string, error) {
	var sb strings.Builder

	// [General]
	sb.WriteString(p.generateGeneral(opts))
	sb.WriteString("\n")

	// [Proxy]
	sb.WriteString(p.generateProxies(proxies, opts))
	sb.WriteString("\n")

	// [Proxy Group]
	sb.WriteString(p.generateProxyGroups(opts))
	sb.WriteString("\n")

	// [Rule]
	sb.WriteString(p.generateRules(opts))
	sb.WriteString("\n")

	// [Host]
	sb.WriteString(p.generateHost())
	sb.WriteString("\n")

	// [URL Rewrite]
	sb.WriteString("[URL Rewrite]\n")
	sb.WriteString("# Google搜索引擎防跳转\n")
	sb.WriteString("^https?://(www.)?g.cn https://www.google.com 302\n")
	sb.WriteString("^https?://(www.)?google.cn https://www.google.com 302\n")
	sb.WriteString("\n")

	// [MITM]
	sb.WriteString("[MITM]\n")
	sb.WriteString("hostname = www.google.cn\n")

	return sb.String(), nil
}

// generateGeneral 生成 [General] 部分
func (p *ShadowrocketTemplateProducer) generateGeneral(opts *ProduceOptions) string {
	var sb strings.Builder
	sb.WriteString("[General]\n")
	sb.WriteString("bypass-system = true\n")
	sb.WriteString("skip-proxy = 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, localhost, *.local, captive.apple.com\n")
	sb.WriteString("tun-excluded-routes = 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.0.0.0/24, 192.0.2.0/24, 192.88.99.0/24, 192.168.0.0/16, 198.51.100.0/24, 203.0.113.0/24, 224.0.0.0/4, 255.255.255.255/32\n")

	// DNS 配置
	dnsServers := p.extractDNSServers(opts.FullConfig)
	if len(dnsServers) > 0 {
		sb.WriteString("dns-server = ")
		sb.WriteString(strings.Join(dnsServers, ","))
		sb.WriteString("\n")
	} else {
		sb.WriteString("dns-server = https://doh.pub/dns-query,https://dns.alidns.com/dns-query,223.5.5.5,119.29.29.29\n")
	}

	// IPv6 配置
	ipv6 := p.extractIPv6Config(opts.FullConfig)
	sb.WriteString(fmt.Sprintf("ipv6 = %v\n", ipv6))

	sb.WriteString("fallback-dns-server = system\n")
	sb.WriteString("prefer-ipv6 = false\n")
	sb.WriteString("dns-direct-system = false\n")
	sb.WriteString("icmp-auto-reply = true\n")
	sb.WriteString("always-reject-url-rewrite = false\n")
	sb.WriteString("private-ip-answer = true\n")
	sb.WriteString("dns-direct-fallback-proxy = false\n")
	sb.WriteString("udp-policy-not-supported-behaviour = REJECT\n")

	return sb.String()
}

// extractDNSServers 从 Clash 配置中提取 DNS 服务器
func (p *ShadowrocketTemplateProducer) extractDNSServers(fullConfig map[string]interface{}) []string {
	var servers []string

	if dnsConfig, ok := fullConfig["dns"].(map[string]interface{}); ok {
		// 提取 nameserver
		if nameserver, ok := dnsConfig["nameserver"].([]interface{}); ok {
			for _, ns := range nameserver {
				if nsStr, ok := ns.(string); ok {
					servers = append(servers, nsStr)
				}
			}
		}
	}

	return servers
}

// extractIPv6Config 从 Clash 配置中提取 IPv6 配置
func (p *ShadowrocketTemplateProducer) extractIPv6Config(fullConfig map[string]interface{}) bool {
	if dnsConfig, ok := fullConfig["dns"].(map[string]interface{}); ok {
		if ipv6, ok := dnsConfig["ipv6"].(bool); ok {
			return ipv6
		}
	}
	return false
}

// generateProxies 生成 [Proxy] 部分
func (p *ShadowrocketTemplateProducer) generateProxies(proxies []Proxy, opts *ProduceOptions) string {
	var sb strings.Builder
	sb.WriteString("[Proxy]\n")
	sb.WriteString("# 节点配置\n")

	transformed := p.transformProxies(proxies, opts)

	for _, proxy := range transformed {
		proxyType := GetString(proxy, "type")
		name := GetString(proxy, "name")

		// 在兼容模式下,过滤不兼容的节点
		if opts != nil && opts.ClientCompatibilityMode {
			if proxyType == "wireguard" {
				log.Printf("[兼容模式] 已过滤不支持的节点类型 '%s': %s", proxyType, name)
				continue
			}
			// 可以添加更多不兼容的节点类型
		}

		proxyLine := p.formatProxyLine(proxy)
		if proxyLine != "" {
			sb.WriteString(proxyLine)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatProxyLine 将节点转换为 Shadowrocket 单行格式
func (p *ShadowrocketTemplateProducer) formatProxyLine(proxy Proxy) string {
	proxyType := GetString(proxy, "type")
	name := GetString(proxy, "name")
	server := GetString(proxy, "server")
	port := GetInt(proxy, "port")

	if name == "" || server == "" || port == 0 {
		return ""
	}

	// 构建参数列表
	var params []string
	params = append(params, name)
	params = append(params, proxyType)
	params = append(params, server)
	params = append(params, fmt.Sprintf("%d", port))

	switch proxyType {
	case "ss":
		// Shadowsocks: name=ss,server,port,encrypt-method=xxx,password=xxx,obfs=xxx,obfs-host=xxx,tfo=1
		if method := GetString(proxy, "cipher"); method != "" {
			params = append(params, fmt.Sprintf("encrypt-method=%s", method))
		}
		if password := GetString(proxy, "password"); password != "" {
			params = append(params, fmt.Sprintf("password=%s", password))
		}
		if plugin := GetString(proxy, "plugin"); plugin != "" {
			if pluginOpts := GetMap(proxy, "plugin-opts"); pluginOpts != nil {
				if mode := GetString(pluginOpts, "mode"); mode != "" {
					params = append(params, fmt.Sprintf("obfs=%s", mode))
				}
				if host := GetString(pluginOpts, "host"); host != "" {
					params = append(params, fmt.Sprintf("obfs-host=%s", host))
				}
			}
		}
		if udp := GetBool(proxy, "udp"); udp {
			params = append(params, "udp-relay=true")
		}
		if tfo := GetBool(proxy, "tfo"); tfo {
			params = append(params, "tfo=1")
		}

	case "vmess":
		// VMess: name=vmess,server,port,username=uuid,alterId=0,method=auto,tls=true,skip-cert-verify=true,obfs=websocket,obfs-path=/,obfs-header=Host:xxx,tfo=1
		if uuid := GetString(proxy, "uuid"); uuid != "" {
			params = append(params, fmt.Sprintf("username=%s", uuid))
		}
		alterId := GetInt(proxy, "alterId")
		params = append(params, fmt.Sprintf("alterId=%d", alterId))

		cipher := GetString(proxy, "cipher")
		if cipher == "" {
			cipher = "auto"
		}
		params = append(params, fmt.Sprintf("method=%s", cipher))

		if tls := GetBool(proxy, "tls"); tls {
			params = append(params, "tls=true")
			if skipCertVerify := GetBool(proxy, "skip-cert-verify"); skipCertVerify {
				params = append(params, "skip-cert-verify=true")
			}
			// transformProxies 已将 sni 转换为 servername
			if servername := GetString(proxy, "servername"); servername != "" {
				params = append(params, fmt.Sprintf("sni=%s", servername))
			}
		}

		network := GetString(proxy, "network")
		if network != "" {
			switch network {
			case "ws":
				params = append(params, "obfs=websocket")
				if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
					if path := GetString(wsOpts, "path"); path != "" {
						params = append(params, fmt.Sprintf("obfs-path=%s", path))
					}
					if headers := GetMap(wsOpts, "headers"); headers != nil {
						if host := GetString(headers, "Host"); host != "" {
							params = append(params, fmt.Sprintf("obfs-header=Host:%s", host))
						}
					}
				}
			case "h2", "http":
				params = append(params, "obfs=http")
				if h2Opts := GetMap(proxy, "h2-opts"); h2Opts != nil {
					if path := GetString(h2Opts, "path"); path != "" {
						params = append(params, fmt.Sprintf("obfs-path=%s", path))
					}
					if host := GetString(h2Opts, "host"); host != "" {
						params = append(params, fmt.Sprintf("obfs-header=Host:%s", host))
					}
				}
			case "grpc":
				params = append(params, "obfs=grpc")
				if grpcOpts := GetMap(proxy, "grpc-opts"); grpcOpts != nil {
					if serviceName := GetString(grpcOpts, "grpc-service-name"); serviceName != "" {
						params = append(params, fmt.Sprintf("obfs-path=%s", serviceName))
					}
				}
			}
		}

		if tfo := GetBool(proxy, "tfo"); tfo {
			params = append(params, "tfo=1")
		}

	case "vless":
		// VLESS: name=vless,server,port,username=uuid,tls=true,skip-cert-verify=true,sni=xxx,obfs=websocket,obfs-path=/,obfs-header=Host:xxx,tfo=1
		if uuid := GetString(proxy, "uuid"); uuid != "" {
			params = append(params, fmt.Sprintf("username=%s", uuid))
		}

		if tls := GetBool(proxy, "tls"); tls {
			params = append(params, "tls=true")
			if skipCertVerify := GetBool(proxy, "skip-cert-verify"); skipCertVerify {
				params = append(params, "skip-cert-verify=true")
			}
			// transformProxies 已将 sni 转换为 servername
			if servername := GetString(proxy, "servername"); servername != "" {
				params = append(params, fmt.Sprintf("sni=%s", servername))
			}
		}

		// Flow
		if flow := GetString(proxy, "flow"); flow != "" {
			params = append(params, fmt.Sprintf("flow=%s", flow))
		}

		// 处理 xhttp/splithttp (transformProxies 已设置 obfs, path, obfsParam)
		if obfs := GetString(proxy, "obfs"); obfs == "xhttp" {
			params = append(params, "obfs=xhttp")
			if path := GetString(proxy, "path"); path != "" {
				params = append(params, fmt.Sprintf("obfs-path=%s", path))
			}
			if obfsParam := GetString(proxy, "obfsParam"); obfsParam != "" {
				params = append(params, fmt.Sprintf("obfs-header=Host:%s", obfsParam))
			}
		} else {
			// 处理其他 network 类型
			network := GetString(proxy, "network")
			if network != "" {
				switch network {
				case "ws":
					params = append(params, "obfs=websocket")
					if wsOpts := GetMap(proxy, "ws-opts"); wsOpts != nil {
						if path := GetString(wsOpts, "path"); path != "" {
							params = append(params, fmt.Sprintf("obfs-path=%s", path))
						}
						if headers := GetMap(wsOpts, "headers"); headers != nil {
							if host := GetString(headers, "Host"); host != "" {
								params = append(params, fmt.Sprintf("obfs-header=Host:%s", host))
							}
						}
					}
				case "h2", "http":
					params = append(params, "obfs=http")
					if h2Opts := GetMap(proxy, "h2-opts"); h2Opts != nil {
						if path := GetString(h2Opts, "path"); path != "" {
							params = append(params, fmt.Sprintf("obfs-path=%s", path))
						}
						if host := GetString(h2Opts, "host"); host != "" {
							params = append(params, fmt.Sprintf("obfs-header=Host:%s", host))
						}
					}
				case "grpc":
					params = append(params, "obfs=grpc")
					if grpcOpts := GetMap(proxy, "grpc-opts"); grpcOpts != nil {
						if serviceName := GetString(grpcOpts, "grpc-service-name"); serviceName != "" {
							params = append(params, fmt.Sprintf("obfs-path=%s", serviceName))
						}
					}
				}
			}
		}

		if tfo := GetBool(proxy, "tfo"); tfo {
			params = append(params, "tfo=1")
		}

	case "trojan":
		// Trojan: name=trojan,server,port,password=xxx,skip-cert-verify=true,sni=xxx,tfo=1
		if password := GetString(proxy, "password"); password != "" {
			params = append(params, fmt.Sprintf("password=%s", password))
		}
		if skipCertVerify := GetBool(proxy, "skip-cert-verify"); skipCertVerify {
			params = append(params, "skip-cert-verify=true")
		}
		if sni := GetString(proxy, "sni"); sni != "" {
			params = append(params, fmt.Sprintf("sni=%s", sni))
		}
		if tfo := GetBool(proxy, "tfo"); tfo {
			params = append(params, "tfo=1")
		}
		if udp := GetBool(proxy, "udp"); udp {
			params = append(params, "udp-relay=true")
		}

	case "http", "https":
		// HTTP/HTTPS: name=http,server,port,username=xxx,password=xxx,tls=true,skip-cert-verify=true,tfo=1
		if username := GetString(proxy, "username"); username != "" {
			params = append(params, fmt.Sprintf("username=%s", username))
		}
		if password := GetString(proxy, "password"); password != "" {
			params = append(params, fmt.Sprintf("password=%s", password))
		}
		if proxyType == "https" || GetBool(proxy, "tls") {
			params = append(params, "tls=true")
			if skipCertVerify := GetBool(proxy, "skip-cert-verify"); skipCertVerify {
				params = append(params, "skip-cert-verify=true")
			}
		}
		if tfo := GetBool(proxy, "tfo"); tfo {
			params = append(params, "tfo=1")
		}

	case "socks5", "socks":
		// SOCKS5: name=socks5,server,port,username=xxx,password=xxx,tls=true,skip-cert-verify=true,tfo=1
		if username := GetString(proxy, "username"); username != "" {
			params = append(params, fmt.Sprintf("username=%s", username))
		}
		if password := GetString(proxy, "password"); password != "" {
			params = append(params, fmt.Sprintf("password=%s", password))
		}
		if tls := GetBool(proxy, "tls"); tls {
			params = append(params, "tls=true")
			if skipCertVerify := GetBool(proxy, "skip-cert-verify"); skipCertVerify {
				params = append(params, "skip-cert-verify=true")
			}
		}
		if tfo := GetBool(proxy, "tfo"); tfo {
			params = append(params, "tfo=1")
		}
		if udp := GetBool(proxy, "udp"); udp {
			params = append(params, "udp-relay=true")
		}

	case "wireguard":
		// WireGuard 配置较复杂，记录警告
		log.Printf("[Shadowrocket] WireGuard节点 '%s' 可能需要手动配置", name)
		return fmt.Sprintf("# WireGuard节点需要手动配置: %s", name)

	default:
		log.Printf("[Shadowrocket] 不支持的节点类型: %s (%s)", proxyType, name)
		return fmt.Sprintf("# 不支持的节点类型 %s: %s", proxyType, name)
	}

	return strings.Join(params, ",")
}

// generateProxyGroups 生成 [Proxy Group] 部分
func (p *ShadowrocketTemplateProducer) generateProxyGroups(opts *ProduceOptions) string {
	var sb strings.Builder
	sb.WriteString("[Proxy Group]\n")
	sb.WriteString("# 代理分组\n")

	if proxyGroups, ok := opts.FullConfig["proxy-groups"].([]interface{}); ok {
		for _, group := range proxyGroups {
			if groupMap, ok := group.(map[string]interface{}); ok {
				groupLine := p.formatProxyGroupLine(groupMap)
				if groupLine != "" {
					sb.WriteString(groupLine)
					sb.WriteString("\n")
				}
			}
		}
	}

	return sb.String()
}

// formatProxyGroupLine 格式化代理组行
func (p *ShadowrocketTemplateProducer) formatProxyGroupLine(group map[string]interface{}) string {
	name := GetString(group, "name")
	groupType := GetString(group, "type")

	if name == "" || groupType == "" {
		return ""
	}

	// 不支持的类型转换
	if groupType == "relay" {
		log.Printf("[Shadowrocket] relay类型不支持，跳过: %s", name)
		return fmt.Sprintf("# relay类型不支持: %s", name)
	}

	// 获取代理列表
	var proxies []string
	if proxyList, ok := group["proxies"].([]interface{}); ok {
		for _, p := range proxyList {
			if pStr, ok := p.(string); ok {
				proxies = append(proxies, pStr)
			}
		}
	}

	if len(proxies) == 0 {
		return ""
	}

	// 构建基础配置
	parts := []string{name, groupType}
	parts = append(parts, proxies...)

	// 添加测试参数（仅 url-test, fallback, load-balance）
	if groupType == "url-test" || groupType == "fallback" || groupType == "load-balance" {
		if url := GetString(group, "url"); url != "" {
			parts = append(parts, fmt.Sprintf("url=%s", url))
		} else {
			parts = append(parts, "url=http://www.gstatic.com/generate_204")
		}

		if interval := GetInt(group, "interval"); interval > 0 {
			parts = append(parts, fmt.Sprintf("interval=%d", interval))
		}

		if timeout := GetInt(group, "timeout"); timeout > 0 {
			parts = append(parts, fmt.Sprintf("timeout=%d", timeout))
		} else {
			parts = append(parts, "timeout=5")
		}

		if tolerance := GetInt(group, "tolerance"); tolerance > 0 {
			parts = append(parts, fmt.Sprintf("tolerance=%d", tolerance))
		}
	}

	return strings.Join(parts, ",")
}

// generateRules 生成 [Rule] 部分
func (p *ShadowrocketTemplateProducer) generateRules(opts *ProduceOptions) string {
	var sb strings.Builder
	sb.WriteString("[Rule]\n")
	sb.WriteString("# 规则配置\n")

	// 处理 rule-providers（需要先处理，因为规则中会引用）
	ruleProviders := make(map[string]string)
	if providers, ok := opts.FullConfig["rule-providers"].(map[string]interface{}); ok {
		for name, provider := range providers {
			if providerMap, ok := provider.(map[string]interface{}); ok {
				if url := GetString(providerMap, "url"); url != "" {
					// 将 .mrs 转换为 .list
					convertedURL := strings.Replace(url, ".mrs", ".list", 1)
					ruleProviders[name] = convertedURL
				}
			}
		}
	}

	// 处理 rules
	if rules, ok := opts.FullConfig["rules"].([]interface{}); ok {
		for _, rule := range rules {
			if ruleStr, ok := rule.(string); ok {
				ruleLine := p.formatRuleLine(ruleStr, ruleProviders)
				if ruleLine != "" {
					sb.WriteString(ruleLine)
					sb.WriteString("\n")
				}
			}
		}
	}

	// 如果没有规则，添加默认规则
	if len(sb.String()) < 50 {
		sb.WriteString("GEOIP,CN,DIRECT\n")
		sb.WriteString("FINAL,PROXY\n")
	}

	return sb.String()
}

// formatRuleLine 格式化规则行
func (p *ShadowrocketTemplateProducer) formatRuleLine(rule string, ruleProviders map[string]string) string {
	parts := strings.Split(rule, ",")
	if len(parts) < 2 {
		return ""
	}

	ruleType := strings.TrimSpace(parts[0])

	// RULE-SET 特殊处理
	if ruleType == "RULE-SET" && len(parts) >= 3 {
		ruleSetName := strings.TrimSpace(parts[1])
		policy := strings.TrimSpace(parts[2])

		// 查找对应的 URL
		if url, ok := ruleProviders[ruleSetName]; ok {
			return fmt.Sprintf("RULE-SET,%s,%s", url, policy)
		}

		log.Printf("[Shadowrocket] 找不到规则集: %s", ruleSetName)
		return fmt.Sprintf("# 规则集未找到: %s", rule)
	}

	// MATCH -> FINAL
	if ruleType == "MATCH" {
		if len(parts) >= 2 {
			return fmt.Sprintf("FINAL,%s", strings.TrimSpace(parts[1]))
		}
		return "FINAL,PROXY"
	}

	// 其他规则直接返回
	return rule
}

// generateHost 生成 [Host] 部分
func (p *ShadowrocketTemplateProducer) generateHost() string {
	return "[Host]\nlocalhost = 127.0.0.1"
}

// transformProxies 转换节点（完整的旧版逻辑）
func (p *ShadowrocketTemplateProducer) transformProxies(proxies []Proxy, opts *ProduceOptions) []Proxy {
	supportedVMessCiphers := map[string]bool{
		"auto": true, "none": true, "zero": true,
		"aes-128-gcm": true, "chacha20-poly1305": true,
	}

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
				if IsPresent(httpOpts, "path") {
					if path, ok := httpOpts["path"].(string); ok {
						httpOpts["path"] = []string{path}
					}
				}

				// Ensure headers.Host is array
				if headers := GetMap(httpOpts, "headers"); headers != nil {
					if host, ok := headers["Host"].(string); ok {
						headers["Host"] = []string{host}
					}
				}
			}
		}

		// 处理xhttp参数
		if proxyType == "vless" && network == "xhttp" {
			if xhttpOpts := GetMap(transformed, "xhttp-opts"); xhttpOpts != nil {
				transformed["obfs"] = network
				if path, ok := xhttpOpts["path"].(string); ok {
					transformed["path"] = path
				}

				if headers := GetMap(xhttpOpts, "headers"); headers != nil {
					if host, ok := headers["Host"].(string); ok {
						transformed["obfsParam"] = host
					}
				}
			}
		}

		// 兼容0.3.7后续版本xhttp改为splithttp的情况
		if proxyType == "vless" && (network == "splithttp" || network == "xhttp") {
			transformed["network"] = "xhttp"
			transformed["obfs"] = "xhttp"
			if splithttpOpts := GetMap(transformed, "splithttp-opts"); splithttpOpts != nil {
				if path, ok := splithttpOpts["path"].(string); ok {
					transformed["path"] = path
				}

				if headers := GetMap(splithttpOpts, "headers"); headers != nil {
					if host, ok := headers["Host"].(string); ok {
						transformed["obfsParam"] = host
					}
				}
			} else if xhttpOpts := GetMap(transformed, "xhttp-opts"); xhttpOpts != nil {
				if path, ok := xhttpOpts["path"].(string); ok {
					transformed["path"] = path
				}

				if headers := GetMap(xhttpOpts, "headers"); headers != nil {
					if host, ok := headers["Host"].(string); ok {
						transformed["obfsParam"] = host
					}
				}
			}
		}

		// Handle H2 network options
		if (proxyType == "vmess" || proxyType == "vless") && network == "h2" {
			if h2Opts := GetMap(transformed, "h2-opts"); h2Opts != nil {
				// Ensure path is string (take first element if array)
				if pathSlice, ok := h2Opts["path"].([]interface{}); ok && len(pathSlice) > 0 {
					h2Opts["path"] = pathSlice[0]
				}

				// Ensure host is array
				if headers := GetMap(h2Opts, "headers"); headers != nil {
					if host, ok := headers["Host"].(string); ok {
						headers["host"] = []string{host}
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

		// Remove null and underscore-prefixed fields
		for key := range transformed {
			if transformed[key] == nil || strings.HasPrefix(key, "_") {
				delete(transformed, key)
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

	return result
}
