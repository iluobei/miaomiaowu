package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"miaomiaowu/internal/logger"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/util"

	"gopkg.in/yaml.v3"
)

// GeoIP 缓存和 API 配置
const ipInfoToken = "cddae164b36656"

type geoIPResponse struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code"`
}

var geoIPCache = sync.Map{} // map[string]string (ip -> countryCode)

// 订阅内容缓存（5分钟过期）
const subscriptionCacheTTL = 5 * time.Minute

type subscriptionCacheEntry struct {
	content   []byte
	fetchedAt time.Time
}

var subscriptionCache = sync.Map{} // map[string]*subscriptionCacheEntry (url -> entry)

// InvalidateSubscriptionContentCache 失效指定URL的订阅内容缓存
func InvalidateSubscriptionContentCache(url string) {
	subscriptionCache.Delete(url)
}

// getGeoIPCountryCode 查询 IP 的国家代码
func getGeoIPCountryCode(ipOrHost string) string {
	if ipOrHost == "" {
		return ""
	}

	// 如果是域名，先解析为 IP
	ip := ipOrHost
	if net.ParseIP(ipOrHost) == nil {
		// 是域名，需要解析
		ips, err := net.LookupIP(ipOrHost)
		if err != nil || len(ips) == 0 {
			logger.Info("[GeoIP] 域名解析失败", "domain", ipOrHost, "error", err)
			return ""
		}
		ip = ips[0].String()
	}

	// 检查缓存
	if cached, ok := geoIPCache.Load(ip); ok {
		return cached.(string)
	}

	// 查询 API
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://api.ipinfo.io/lite/%s?token=%s", ip, ipInfoToken))
	if err != nil {
		logger.Info("[GeoIP] IP查询失败", "ip", ip, "error", err)
		// 缓存空结果避免重复查询
		geoIPCache.Store(ip, "")
		return ""
	}
	defer resp.Body.Close()

	var result geoIPResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Info("[GeoIP] 响应解析失败", "ip", ip, "error", err)
		geoIPCache.Store(ip, "")
		return ""
	}

	// 缓存结果
	countryCode := strings.ToUpper(result.CountryCode)
	geoIPCache.Store(ip, countryCode)
	logger.Info("[GeoIP] IP地理位置查询成功", "ip", ip, "country", countryCode)
	return countryCode
}

// NewProxyProviderServeHandler handles serving filtered proxies for "妙妙屋处理" mode
// URL: /api/proxy-provider/{config_id}?token={user_token}
func NewProxyProviderServeHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("proxy provider serve handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		// Extract config_id from URL path: /api/proxy-provider/{config_id}
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathParts) < 3 {
			writeError(w, http.StatusBadRequest, errors.New("invalid path"))
			return
		}

		configIDStr := pathParts[len(pathParts)-1]
		configID, err := strconv.ParseInt(configIDStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid config_id"))
			return
		}

		// Get token from query params or authorization header
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("Authorization")
			if after, ok := strings.CutPrefix(token, "Bearer "); ok {
				token = after
			}
		}

		// Validate user token and get username
		username, err := repo.ValidateUserToken(r.Context(), token)
		if err != nil || username == "" {
			writeError(w, http.StatusUnauthorized, errors.New("invalid token"))
			return
		}

		// Get proxy provider config
		config, err := repo.GetProxyProviderConfig(r.Context(), configID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if config == nil || config.Username != username {
			writeError(w, http.StatusNotFound, errors.New("proxy provider config not found"))
			return
		}

		// Only process if mode is "mmw"
		if config.ProcessMode != "mmw" {
			writeError(w, http.StatusBadRequest, errors.New("this config is set to client processing mode"))
			return
		}

		// Get external subscription
		sub, err := repo.GetExternalSubscription(r.Context(), config.ExternalSubscriptionID, username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if sub.ID == 0 {
			writeError(w, http.StatusNotFound, errors.New("external subscription not found"))
			return
		}

		// 检查缓存
		cache := GetProxyProviderCache()
		if entry, ok := cache.Get(configID); ok && !cache.IsExpired(entry) {
			logger.Info("[ProxyProviderServe] 使用缓存", "id", configID, "node_count", entry.NodeCount)
			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(entry.YAMLData)
			return
		}

		// 缓存未命中或过期，拉取新数据
		entry, err := RefreshProxyProviderCache(&sub, config)
		if err != nil {
			logger.Info("[ProxyProviderServe] 拉取代理节点失败", "config_id", configID, "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// Output directly without download
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(entry.YAMLData)
	})
}

// fetchSubscriptionContent fetches subscription content with caching (5 min TTL)
func fetchSubscriptionContent(sub *storage.ExternalSubscription) ([]byte, error) {
	cacheKey := sub.URL

	// 检查缓存
	if cached, ok := subscriptionCache.Load(cacheKey); ok {
		entry := cached.(*subscriptionCacheEntry)
		if time.Since(entry.fetchedAt) < subscriptionCacheTTL {
			logger.Info("[SubscriptionCache] 缓存命中", "url", sub.URL)
			return entry.content, nil
		}
		// 缓存过期，删除
		subscriptionCache.Delete(cacheKey)
	}

	logger.Info("[SubscriptionCache] 缓存未命中，正在拉取", "url", sub.URL)

	// 拉取订阅内容
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, sub.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// 存入缓存
	subscriptionCache.Store(cacheKey, &subscriptionCacheEntry{
		content:   body,
		fetchedAt: time.Now(),
	})

	return body, nil
}

// base64FeatureStrings 是用于检测 base64 编码订阅内容的特征字符串
// 这些是常见协议标识符的 base64 编码形式
var base64FeatureStrings = []string{
	"dm1lc3M",          // vmess
	"c3NyOi8v",         // ssr://
	"c29ja3M6Ly",       // socks://
	"dHJvamFu",         // trojan
	"c3M6Ly",           // ss:/
	"c3NkOi8v",         // ssd://
	"c2hhZG93",         // shadow
	"aHR0c",            // htt
	"dmxlc3M",          // vless
	"aHlzdGVyaWEy",     // hysteria2
	"aHkyOi8v",         // hy2://
	"d2lyZWd1YXJkOi8v", // wireguard://
	"d2c6Ly8",          // wg://
	"dHVpYzovLw",       // tuic://
}

// preprocessSubscriptionContent 预处理订阅内容
// 检测并解码可能是 base64 编码的订阅内容
// 返回处理后的内容和错误信息（如果需要）
func preprocessSubscriptionContent(content []byte) ([]byte, error) {
	text := string(content)
	trimmed := strings.TrimSpace(text)

	// 1. 检查是否是 HTML（直接返回空）
	if strings.HasPrefix(trimmed, "<!DOCTYPE html>") || strings.HasPrefix(trimmed, "<html") {
		logger.Info("[预处理] 检测到 HTML 内容，跳过")
		return content, nil
	}

	// 2. 检查是否已经是 YAML 格式（包含 proxies:）
	if strings.Contains(trimmed, "proxies:") {
		// 尝试解析，如果成功则直接返回
		var rootNode yaml.Node
		if err := yaml.Unmarshal(content, &rootNode); err == nil {
			logger.Info("[预处理] 内容已经是有效的 YAML 格式")
			return content, nil
		}
	}

	// 3. 检查是否是 URI 协议格式（非 base64，每行一个 URI）
	if isURIListFormat(trimmed) {
		logger.Info("[预处理] 检测到 URI 列表格式，尝试转换为 YAML")
		yamlContent, err := convertURIListToYAML(trimmed)
		if err != nil {
			return nil, fmt.Errorf("URI 列表格式转换失败: %w", err)
		}
		return yamlContent, nil
	}

	// 4. 尝试 base64 解码
	decoded := tryBase64Decode(trimmed)
	if decoded != nil {
		decodedStr := string(decoded)
		decodedTrimmed := strings.TrimSpace(decodedStr)

		// 检查解码后的内容类型
		if strings.Contains(decodedTrimmed, "proxies:") {
			// YAML 格式
			var rootNode yaml.Node
			if err := yaml.Unmarshal(decoded, &rootNode); err == nil {
				logger.Info("[预处理] base64 解码后是有效的 YAML 格式", "original_len", len(content), "decoded_len", len(decoded))
				return decoded, nil
			}
		}

		// 检查是否是 URI 列表格式
		if isURIListFormat(decodedTrimmed) {
			logger.Info("[预处理] base64 解码后是 URI 列表格式，尝试转换为 YAML")
			yamlContent, err := convertURIListToYAML(decodedTrimmed)
			if err != nil {
				return nil, fmt.Errorf("base64 解码后的 URI 列表格式转换失败: %w", err)
			}
			return yamlContent, nil
		}

		// 解码成功但格式不明确，尝试继续使用
		if len(decoded) > 0 {
			logger.Info("[预处理] base64 解码成功但格式未知", "original_len", len(content), "decoded_len", len(decoded))
			return decoded, nil
		}
	}

	// 5. 无法识别的格式，返回原内容
	return content, nil
}

// isURIListFormat 检查内容是否是 URI 列表格式
func isURIListFormat(content string) bool {
	// 检查第一个非空行是否是 URI 协议格式
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 检查是否是支持的协议
		supportedProtocols := []string{
			"vmess://", "vless://", "ss://", "ssr://", "trojan://",
			"hysteria://", "hysteria2://", "hy2://", "tuic://",
			"socks://", "socks5://", "http://", "https://",
			"wireguard://", "wg://", "anytls://",
		}
		for _, proto := range supportedProtocols {
			if strings.HasPrefix(line, proto) {
				return true
			}
		}
		// 第一个非空行不是 URI 格式，返回 false
		return false
	}
	return false
}

// tryBase64Decode 尝试多种 base64 编码方式解码
func tryBase64Decode(content string) []byte {
	// 移除可能的换行符和空白
	content = strings.TrimSpace(content)

	// 尝试标准 base64
	if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
		return decoded
	}

	// 尝试 URL-safe base64
	if decoded, err := base64.URLEncoding.DecodeString(content); err == nil {
		return decoded
	}

	// 尝试无填充的标准 base64
	if decoded, err := base64.RawStdEncoding.DecodeString(content); err == nil {
		return decoded
	}

	// 尝试无填充的 URL-safe base64
	if decoded, err := base64.RawURLEncoding.DecodeString(content); err == nil {
		return decoded
	}

	return nil
}

// convertURIListToYAML 将 URI 列表转换为 YAML 格式
// 注意：这是一个简化的实现，仅提取节点基本信息
func convertURIListToYAML(content string) ([]byte, error) {
	lines := strings.Split(content, "\n")
	var proxies []map[string]interface{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		proxy, err := parseProxyURI(line)
		if err != nil {
			logger.Info("[URI解析] 跳过无效的 URI", "line", truncateString(line, 50), "error", err)
			continue
		}
		if proxy != nil {
			proxies = append(proxies, proxy)
		}
	}

	if len(proxies) == 0 {
		return nil, fmt.Errorf("没有解析到有效的代理节点")
	}

	logger.Info("[URI解析] 成功解析代理节点", "count", len(proxies))

	// 构建 YAML 格式
	result := map[string]interface{}{
		"proxies": proxies,
	}

	return yaml.Marshal(result)
}

// truncateString 截断字符串用于日志
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseProxyURI 解析单个代理 URI
func parseProxyURI(uri string) (map[string]interface{}, error) {
	uri = strings.TrimSpace(uri)

	switch {
	case strings.HasPrefix(uri, "vmess://"):
		return parseVmessURI(uri)
	case strings.HasPrefix(uri, "vless://"):
		return parseVlessURI(uri)
	case strings.HasPrefix(uri, "ss://"):
		return parseShadowsocksURI(uri)
	case strings.HasPrefix(uri, "ssr://"):
		return parseShadowsocksRURI(uri)
	case strings.HasPrefix(uri, "trojan://"):
		return parseTrojanURI(uri)
	case strings.HasPrefix(uri, "hysteria://"):
		return parseHysteriaURI(uri, "hysteria")
	case strings.HasPrefix(uri, "hysteria2://"), strings.HasPrefix(uri, "hy2://"):
		u := uri
		if strings.HasPrefix(uri, "hy2://") {
			u = "hysteria2://" + uri[6:]
		}
		return parseHysteriaURI(u, "hysteria2")
	case strings.HasPrefix(uri, "tuic://"):
		return parseTuicURI(uri)
	case strings.HasPrefix(uri, "wireguard://"), strings.HasPrefix(uri, "wg://"):
		return parseWireguardURI(uri)
	case strings.HasPrefix(uri, "anytls://"):
		return parseAnytlsURI(uri)
	case strings.HasPrefix(uri, "socks://"), strings.HasPrefix(uri, "socks5://"):
		return parseSocksURI(uri)
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}

// parseVmessURI 解析 vmess:// URI
func parseVmessURI(uri string) (map[string]interface{}, error) {
	content := strings.TrimPrefix(uri, "vmess://")

	// VMess 格式: vmess://base64(json)
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("vmess base64 decode failed: %w", err)
		}
	}

	var config map[string]interface{}
	if err := json.Unmarshal(decoded, &config); err != nil {
		return nil, fmt.Errorf("vmess json parse failed: %w", err)
	}

	proxy := map[string]interface{}{
		"type":   "vmess",
		"name":   getStringValue(config, "ps", getStringValue(config, "name", "VMess Node")),
		"server": getStringValue(config, "add", getStringValue(config, "address", "")),
		"port":   getIntValue(config, "port"),
		"uuid":   getStringValue(config, "id", ""),
		"cipher": getStringValue(config, "scy", "auto"),
		"udp":    true,
	}

	// alterId
	if aid := getIntValue(config, "aid"); aid > 0 {
		proxy["alterId"] = aid
	} else {
		proxy["alterId"] = 0
	}

	// network
	network := getStringValue(config, "net", "tcp")
	if network != "" && network != "tcp" {
		proxy["network"] = network
	}

	// TLS
	if tls := getStringValue(config, "tls", ""); tls == "tls" {
		proxy["tls"] = true
	}

	// SNI
	if sni := getStringValue(config, "sni", ""); sni != "" {
		proxy["sni"] = sni
	} else if host := getStringValue(config, "host", ""); host != "" && proxy["tls"] == true {
		proxy["sni"] = host
	}

	// WebSocket options
	if network == "ws" {
		wsOpts := make(map[string]interface{})
		if path := getStringValue(config, "path", ""); path != "" {
			wsOpts["path"] = path
		} else {
			wsOpts["path"] = "/"
		}
		if host := getStringValue(config, "host", ""); host != "" {
			wsOpts["headers"] = map[string]interface{}{"Host": host}
		}
		proxy["ws-opts"] = wsOpts
	}

	// gRPC options
	if network == "grpc" {
		grpcOpts := make(map[string]interface{})
		if path := getStringValue(config, "path", ""); path != "" {
			grpcOpts["grpc-service-name"] = path
		}
		proxy["grpc-opts"] = grpcOpts
	}

	return proxy, nil
}

// parseVlessURI 解析 vless:// URI
func parseVlessURI(uri string) (map[string]interface{}, error) {
	// vless://uuid@server:port?params#name
	content := strings.TrimPrefix(uri, "vless://")

	// 提取名称
	name := "VLESS Node"
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 提取参数
	params := make(map[string]string)
	if idx := strings.Index(content, "?"); idx != -1 {
		paramStr := content[idx+1:]
		content = content[:idx]
		for _, kv := range strings.Split(paramStr, "&") {
			if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
				params[parts[0]] = urlDecode(parts[1])
			}
		}
	}

	// 解析 uuid@server:port
	atIdx := strings.LastIndex(content, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("invalid vless uri: missing @")
	}

	uuid := content[:atIdx]
	serverPort := content[atIdx+1:]

	server, port := parseServerPort(serverPort)
	if server == "" || port == 0 {
		return nil, fmt.Errorf("invalid vless uri: invalid server:port")
	}

	proxy := map[string]interface{}{
		"type":   "vless",
		"name":   name,
		"server": server,
		"port":   port,
		"uuid":   uuid,
		"udp":    true,
	}

	// Security
	security := params["security"]
	if security == "tls" || security == "reality" {
		proxy["tls"] = true
	}

	// SNI
	if sni := params["sni"]; sni != "" {
		proxy["sni"] = sni
	}

	// Flow
	if flow := params["flow"]; flow != "" {
		proxy["flow"] = flow
	}

	// Network type
	network := params["type"]
	if network == "" {
		network = "tcp"
	}
	if network != "tcp" {
		proxy["network"] = network
	}

	// Transport options
	switch network {
	case "ws":
		wsOpts := map[string]interface{}{"path": params["path"]}
		if params["path"] == "" {
			wsOpts["path"] = "/"
		}
		if host := params["host"]; host != "" {
			wsOpts["headers"] = map[string]interface{}{"Host": host}
		}
		proxy["ws-opts"] = wsOpts
	case "grpc":
		grpcOpts := make(map[string]interface{})
		if sn := params["serviceName"]; sn != "" {
			grpcOpts["grpc-service-name"] = sn
		} else if path := params["path"]; path != "" {
			grpcOpts["grpc-service-name"] = path
		}
		proxy["grpc-opts"] = grpcOpts
	}

	// Reality
	if security == "reality" {
		realityOpts := make(map[string]interface{})
		if pbk := params["pbk"]; pbk != "" {
			realityOpts["public-key"] = pbk
		}
		if sid := params["sid"]; sid != "" {
			realityOpts["short-id"] = sid
		}
		proxy["reality-opts"] = realityOpts

		if fp := params["fp"]; fp != "" {
			proxy["client-fingerprint"] = fp
		}
	}

	// Skip cert verify
	if params["allowInsecure"] == "1" {
		proxy["skip-cert-verify"] = true
	}

	return proxy, nil
}

// parseShadowsocksURI 解析 ss:// URI
func parseShadowsocksURI(uri string) (map[string]interface{}, error) {
	content := strings.TrimPrefix(uri, "ss://")

	// 提取名称
	name := "SS Node"
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 提取参数
	params := make(map[string]string)
	if idx := strings.Index(content, "?"); idx != -1 {
		paramStr := content[idx+1:]
		content = content[:idx]
		for _, kv := range strings.Split(paramStr, "&") {
			if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
				params[parts[0]] = urlDecode(parts[1])
			}
		}
	}

	// 去掉尾部的 /
	content = strings.TrimSuffix(content, "/")

	var server string
	var port int
	var method, password string

	if strings.Contains(content, "@") {
		// 格式: base64(method:password)@server:port 或 method:password@server:port
		atIdx := strings.LastIndex(content, "@")
		authPart := content[:atIdx]
		serverPart := content[atIdx+1:]

		server, port = parseServerPort(serverPart)

		// 尝试判断 authPart 是否已经是 method:password 格式
		knownCiphers := []string{
			"aes-128-gcm", "aes-192-gcm", "aes-256-gcm",
			"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
			"chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
			"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm",
			"rc4-md5", "none",
		}

		isPlainAuth := false
		for _, cipher := range knownCiphers {
			if strings.HasPrefix(authPart, cipher+":") {
				isPlainAuth = true
				method = cipher
				password = authPart[len(cipher)+1:]
				break
			}
		}

		if !isPlainAuth {
			// base64 解码
			decoded, err := base64.StdEncoding.DecodeString(urlDecode(authPart))
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(urlDecode(authPart))
			}
			if err != nil {
				return nil, fmt.Errorf("ss base64 decode failed: %w", err)
			}
			decodedStr := string(decoded)
			colonIdx := strings.Index(decodedStr, ":")
			if colonIdx == -1 {
				return nil, fmt.Errorf("invalid ss auth format")
			}
			method = decodedStr[:colonIdx]
			password = decodedStr[colonIdx+1:]
		}
	} else {
		// 格式: base64(method:password@server:port)
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(content)
		}
		if err != nil {
			return nil, fmt.Errorf("ss base64 decode failed: %w", err)
		}
		decodedStr := string(decoded)

		atIdx := strings.LastIndex(decodedStr, "@")
		if atIdx == -1 {
			return nil, fmt.Errorf("invalid ss uri format")
		}

		authPart := decodedStr[:atIdx]
		serverPart := decodedStr[atIdx+1:]

		colonIdx := strings.Index(authPart, ":")
		if colonIdx == -1 {
			return nil, fmt.Errorf("invalid ss auth format")
		}
		method = authPart[:colonIdx]
		password = authPart[colonIdx+1:]

		server, port = parseServerPort(serverPart)
	}

	if server == "" || port == 0 {
		return nil, fmt.Errorf("invalid ss uri: invalid server:port")
	}

	proxy := map[string]interface{}{
		"type":     "ss",
		"name":     name,
		"server":   server,
		"port":     port,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}

	// Plugin
	if plugin := params["plugin"]; plugin != "" {
		pluginParts := strings.SplitN(urlDecode(plugin), ";", 2)
		pluginName := pluginParts[0]
		if pluginName == "obfs-local" || pluginName == "simple-obfs" {
			pluginName = "obfs"
		}
		proxy["plugin"] = pluginName

		if len(pluginParts) > 1 {
			pluginOpts := make(map[string]interface{})
			for _, kv := range strings.Split(pluginParts[1], ";") {
				if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
					key := parts[0]
					value := parts[1]
					switch key {
					case "obfs":
						pluginOpts["mode"] = value
					case "obfs-host", "host":
						pluginOpts["host"] = value
					case "path":
						pluginOpts["path"] = value
					case "tls":
						pluginOpts["tls"] = value == "true" || value == "1"
					}
				}
			}
			if len(pluginOpts) > 0 {
				proxy["plugin-opts"] = pluginOpts
			}
		}
	}

	return proxy, nil
}

// parseShadowsocksRURI 解析 ssr:// URI
func parseShadowsocksRURI(uri string) (map[string]interface{}, error) {
	content := strings.TrimPrefix(uri, "ssr://")

	// SSR 格式: ssr://base64(server:port:protocol:method:obfs:base64(password)/?params)
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(content)
	}
	if err != nil {
		return nil, fmt.Errorf("ssr base64 decode failed: %w", err)
	}

	decodedStr := string(decoded)

	// 分离主体和参数
	parts := strings.SplitN(decodedStr, "/?", 2)
	mainPart := parts[0]

	// 解析参数
	params := make(map[string]string)
	if len(parts) > 1 {
		for _, kv := range strings.Split(parts[1], "&") {
			if p := strings.SplitN(kv, "=", 2); len(p) == 2 {
				params[p[0]] = p[1]
			}
		}
	}

	// 解析主体: server:port:protocol:method:obfs:base64(password)
	segments := strings.Split(mainPart, ":")
	if len(segments) < 6 {
		return nil, fmt.Errorf("invalid ssr format: not enough segments")
	}

	// 从右往左解析
	passwordBase64 := segments[len(segments)-1]
	obfs := segments[len(segments)-2]
	method := segments[len(segments)-3]
	protocol := segments[len(segments)-4]
	portStr := segments[len(segments)-5]
	server := strings.Join(segments[:len(segments)-5], ":")

	port, _ := strconv.Atoi(portStr)

	// 解码密码
	passwordDecoded, _ := base64.StdEncoding.DecodeString(passwordBase64)
	if len(passwordDecoded) == 0 {
		passwordDecoded, _ = base64.RawStdEncoding.DecodeString(passwordBase64)
	}
	password := string(passwordDecoded)

	// 解码名称
	name := "SSR Node"
	if remarks := params["remarks"]; remarks != "" {
		remarksDecoded, _ := base64.StdEncoding.DecodeString(remarks)
		if len(remarksDecoded) == 0 {
			remarksDecoded, _ = base64.RawStdEncoding.DecodeString(remarks)
		}
		if len(remarksDecoded) > 0 {
			name = string(remarksDecoded)
		}
	}

	proxy := map[string]interface{}{
		"type":     "ssr",
		"name":     name,
		"server":   server,
		"port":     port,
		"cipher":   method,
		"password": password,
		"protocol": protocol,
		"obfs":     obfs,
		"udp":      true,
	}

	// obfs-param
	if obfsParam := params["obfsparam"]; obfsParam != "" {
		obfsParamDecoded, _ := base64.StdEncoding.DecodeString(obfsParam)
		if len(obfsParamDecoded) == 0 {
			obfsParamDecoded, _ = base64.RawStdEncoding.DecodeString(obfsParam)
		}
		if len(obfsParamDecoded) > 0 {
			proxy["obfs-param"] = string(obfsParamDecoded)
		}
	}

	// protocol-param
	if protoParam := params["protoparam"]; protoParam != "" {
		protoParamDecoded, _ := base64.StdEncoding.DecodeString(protoParam)
		if len(protoParamDecoded) == 0 {
			protoParamDecoded, _ = base64.RawStdEncoding.DecodeString(protoParam)
		}
		if len(protoParamDecoded) > 0 {
			proxy["protocol-param"] = string(protoParamDecoded)
		}
	}

	return proxy, nil
}

// parseTrojanURI 解析 trojan:// URI
func parseTrojanURI(uri string) (map[string]interface{}, error) {
	// trojan://password@server:port?params#name
	content := strings.TrimPrefix(uri, "trojan://")

	// 提取名称
	name := "Trojan Node"
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 提取参数
	params := make(map[string]string)
	if idx := strings.Index(content, "?"); idx != -1 {
		paramStr := content[idx+1:]
		content = content[:idx]
		for _, kv := range strings.Split(paramStr, "&") {
			if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
				params[parts[0]] = urlDecode(parts[1])
			}
		}
	}

	// 解析 password@server:port
	atIdx := strings.LastIndex(content, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("invalid trojan uri: missing @")
	}

	password := content[:atIdx]
	serverPort := content[atIdx+1:]

	server, port := parseServerPort(serverPort)
	if server == "" || port == 0 {
		return nil, fmt.Errorf("invalid trojan uri: invalid server:port")
	}

	proxy := map[string]interface{}{
		"type":     "trojan",
		"name":     name,
		"server":   server,
		"port":     port,
		"password": password,
		"udp":      true,
	}

	// SNI
	if sni := params["sni"]; sni != "" {
		proxy["sni"] = sni
	} else if peer := params["peer"]; peer != "" {
		proxy["sni"] = peer
	} else if host := params["host"]; host != "" {
		proxy["sni"] = host
	}

	// Network type
	network := params["type"]
	if network != "" && network != "tcp" {
		proxy["network"] = network
	}

	// Transport options
	if network == "ws" {
		wsOpts := map[string]interface{}{"path": params["path"]}
		if params["path"] == "" {
			wsOpts["path"] = "/"
		}
		if host := params["host"]; host != "" {
			wsOpts["headers"] = map[string]interface{}{"Host": host}
		}
		proxy["ws-opts"] = wsOpts
	} else if network == "grpc" {
		grpcOpts := make(map[string]interface{})
		if sn := params["serviceName"]; sn != "" {
			grpcOpts["grpc-service-name"] = sn
		} else if path := params["path"]; path != "" {
			grpcOpts["grpc-service-name"] = path
		}
		proxy["grpc-opts"] = grpcOpts
	}

	// ALPN
	if alpn := params["alpn"]; alpn != "" {
		proxy["alpn"] = strings.Split(alpn, ",")
	}

	// Skip cert verify
	if params["allowInsecure"] == "1" || params["skip-cert-verify"] == "1" {
		proxy["skip-cert-verify"] = true
	}

	// Client fingerprint
	if fp := params["fp"]; fp != "" {
		proxy["client-fingerprint"] = fp
	}

	return proxy, nil
}

// parseHysteriaURI 解析 hysteria:// 或 hysteria2:// URI
func parseHysteriaURI(uri string, proxyType string) (map[string]interface{}, error) {
	var content string
	if proxyType == "hysteria2" {
		content = strings.TrimPrefix(uri, "hysteria2://")
	} else {
		content = strings.TrimPrefix(uri, "hysteria://")
	}

	// 提取名称
	name := strings.ToUpper(proxyType) + " Node"
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 提取参数
	params := make(map[string]string)
	if idx := strings.Index(content, "?"); idx != -1 {
		paramStr := content[idx+1:]
		content = content[:idx]
		for _, kv := range strings.Split(paramStr, "&") {
			if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
				params[parts[0]] = urlDecode(parts[1])
			}
		}
	}

	// 去掉尾部的 /
	content = strings.TrimSuffix(content, "/")

	// 解析 password@server:port
	atIdx := strings.LastIndex(content, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("invalid %s uri: missing @", proxyType)
	}

	password := content[:atIdx]
	serverPort := content[atIdx+1:]

	server, port := parseServerPort(serverPort)
	if server == "" || port == 0 {
		return nil, fmt.Errorf("invalid %s uri: invalid server:port", proxyType)
	}

	proxy := map[string]interface{}{
		"type":     proxyType,
		"name":     name,
		"server":   server,
		"port":     port,
		"password": password,
		"udp":      true,
	}

	// SNI
	if sni := params["sni"]; sni != "" {
		proxy["sni"] = sni
	} else if peer := params["peer"]; peer != "" {
		proxy["sni"] = peer
	}

	// ALPN
	if alpn := params["alpn"]; alpn != "" {
		proxy["alpn"] = strings.Split(alpn, ",")
	}

	// Skip cert verify
	if params["insecure"] == "1" || params["allowInsecure"] == "1" || params["skip-cert-verify"] == "1" {
		proxy["skip-cert-verify"] = true
	}

	// Obfs
	if obfs := params["obfs"]; obfs != "" {
		proxy["obfs"] = obfs
	}
	if obfsPassword := params["obfs-password"]; obfsPassword != "" {
		proxy["obfs-password"] = obfsPassword
	} else if obfsParam := params["obfsParam"]; obfsParam != "" {
		proxy["obfs-password"] = obfsParam
	}

	// Up/Down bandwidth
	if up := params["up"]; up != "" {
		proxy["up"] = up
	} else if upmbps := params["upmbps"]; upmbps != "" {
		proxy["up"] = upmbps
	}
	if down := params["down"]; down != "" {
		proxy["down"] = down
	} else if downmbps := params["downmbps"]; downmbps != "" {
		proxy["down"] = downmbps
	}

	// Client fingerprint
	if fp := params["fp"]; fp != "" {
		proxy["client-fingerprint"] = fp
	}

	return proxy, nil
}

// parseTuicURI 解析 tuic:// URI
func parseTuicURI(uri string) (map[string]interface{}, error) {
	content := strings.TrimPrefix(uri, "tuic://")

	// 提取名称
	name := "TUIC Node"
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 提取参数
	params := make(map[string]string)
	if idx := strings.Index(content, "?"); idx != -1 {
		paramStr := content[idx+1:]
		content = content[:idx]
		for _, kv := range strings.Split(paramStr, "&") {
			if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
				params[parts[0]] = urlDecode(parts[1])
			}
		}
	}

	// 解析 uuid:password@server:port
	atIdx := strings.LastIndex(content, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("invalid tuic uri: missing @")
	}

	auth := urlDecode(content[:atIdx])
	serverPort := content[atIdx+1:]

	// 解析 uuid:password
	var uuid, password string
	colonIdx := strings.Index(auth, ":")
	if colonIdx != -1 {
		uuid = auth[:colonIdx]
		password = auth[colonIdx+1:]
	} else {
		uuid = auth
		password = params["password"]
	}

	server, port := parseServerPort(serverPort)
	if server == "" || port == 0 {
		return nil, fmt.Errorf("invalid tuic uri: invalid server:port")
	}

	proxy := map[string]interface{}{
		"type":     "tuic",
		"name":     name,
		"server":   server,
		"port":     port,
		"uuid":     uuid,
		"password": password,
		"udp":      true,
	}

	// SNI
	if sni := params["sni"]; sni != "" {
		proxy["sni"] = sni
	}

	// ALPN
	if alpn := params["alpn"]; alpn != "" {
		proxy["alpn"] = strings.Split(alpn, ",")
	} else {
		proxy["alpn"] = []string{"h3"}
	}

	// Skip cert verify
	if params["allowInsecure"] == "1" || params["allow_insecure"] == "1" {
		proxy["skip-cert-verify"] = true
	}

	// Congestion controller
	if cc := params["congestion_control"]; cc != "" {
		proxy["congestion-controller"] = cc
	} else {
		proxy["congestion-controller"] = "bbr"
	}

	// UDP relay mode
	if urm := params["udp_relay_mode"]; urm != "" {
		proxy["udp-relay-mode"] = urm
	} else {
		proxy["udp-relay-mode"] = "native"
	}

	return proxy, nil
}

// parseWireguardURI 解析 wireguard:// 或 wg:// URI
func parseWireguardURI(uri string) (map[string]interface{}, error) {
	// 移除协议前缀
	content := strings.TrimPrefix(uri, "wireguard://")
	content = strings.TrimPrefix(content, "wg://")

	// 提取名称
	name := ""
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 提取参数
	params := make(map[string]string)
	if idx := strings.Index(content, "?"); idx != -1 {
		paramStr := content[idx+1:]
		content = content[:idx]
		for _, kv := range strings.Split(paramStr, "&") {
			if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
				key := strings.ReplaceAll(parts[0], "_", "-")
				params[key] = urlDecode(parts[1])
			}
		}
	}

	// 去掉尾部的 /
	content = strings.TrimSuffix(content, "/")

	// 解析 privateKey@server:port
	var privateKey, serverPort string
	if atIdx := strings.LastIndex(content, "@"); atIdx != -1 {
		privateKey = urlDecode(content[:atIdx])
		serverPort = content[atIdx+1:]
	} else {
		serverPort = content
	}

	server, port := parseServerPort(serverPort)
	if server == "" {
		return nil, fmt.Errorf("invalid wireguard uri: invalid server")
	}
	if port == 0 {
		port = 51820 // 默认端口
	}

	if name == "" {
		name = fmt.Sprintf("WireGuard %s:%d", server, port)
	}

	proxy := map[string]interface{}{
		"type":        "wireguard",
		"name":        name,
		"server":      server,
		"port":        port,
		"private-key": privateKey,
		"udp":         true,
	}

	// Public key
	if pk := params["publickey"]; pk != "" {
		proxy["public-key"] = pk
	}

	// Private key (from params if not in URI)
	if pk := params["privatekey"]; pk != "" && privateKey == "" {
		proxy["private-key"] = pk
	}

	// Reserved
	if reserved := params["reserved"]; reserved != "" {
		parts := strings.Split(reserved, ",")
		reservedInts := make([]int, 0, 3)
		for _, p := range parts {
			if v, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				reservedInts = append(reservedInts, v)
			}
		}
		if len(reservedInts) == 3 {
			proxy["reserved"] = reservedInts
		}
	}

	// Address/IP
	if addr := params["address"]; addr != "" {
		for _, a := range strings.Split(addr, ",") {
			a = strings.TrimSpace(a)
			a = strings.TrimPrefix(a, "[")
			a = strings.TrimSuffix(a, "]")
			// 移除 CIDR
			if idx := strings.Index(a, "/"); idx != -1 {
				a = a[:idx]
			}
			// 判断是 IPv4 还是 IPv6
			if strings.Contains(a, ":") {
				proxy["ipv6"] = a
			} else if strings.Count(a, ".") == 3 {
				proxy["ip"] = a
			}
		}
	} else if ip := params["ip"]; ip != "" {
		proxy["ip"] = ip
	}

	// MTU
	if mtu := params["mtu"]; mtu != "" {
		if v, err := strconv.Atoi(mtu); err == nil {
			proxy["mtu"] = v
		}
	}

	// Allowed IPs
	if allowedIPs := params["allowed-ips"]; allowedIPs != "" {
		// 处理可能的 JSON 数组格式
		allowedIPs = strings.TrimPrefix(allowedIPs, "[")
		allowedIPs = strings.TrimSuffix(allowedIPs, "]")
		ips := strings.Split(allowedIPs, ",")
		cleanIPs := make([]string, 0, len(ips))
		for _, ip := range ips {
			if trimmed := strings.TrimSpace(ip); trimmed != "" {
				cleanIPs = append(cleanIPs, trimmed)
			}
		}
		if len(cleanIPs) > 0 {
			proxy["allowed-ips"] = cleanIPs
		}
	}

	return proxy, nil
}

// parseAnytlsURI 解析 anytls:// URI
func parseAnytlsURI(uri string) (map[string]interface{}, error) {
	content := strings.TrimPrefix(uri, "anytls://")

	// 提取名称
	name := "AnyTLS Node"
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 提取参数
	params := make(map[string]string)
	if idx := strings.Index(content, "?"); idx != -1 {
		paramStr := content[idx+1:]
		content = content[:idx]
		for _, kv := range strings.Split(paramStr, "&") {
			if parts := strings.SplitN(kv, "=", 2); len(parts) == 2 {
				params[parts[0]] = urlDecode(parts[1])
			}
		}
	}

	// 去掉尾部的 /
	content = strings.TrimSuffix(content, "/")

	// 解析 password@server:port
	atIdx := strings.LastIndex(content, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("invalid anytls uri: missing @")
	}

	password := content[:atIdx]
	serverPort := content[atIdx+1:]

	server, port := parseServerPort(serverPort)
	if server == "" {
		return nil, fmt.Errorf("invalid anytls uri: invalid server")
	}
	if port == 0 {
		port = 443 // 默认端口
	}

	proxy := map[string]interface{}{
		"type":     "anytls",
		"name":     name,
		"server":   server,
		"port":     port,
		"password": password,
		"udp":      true,
	}

	// SNI
	if sni := params["sni"]; sni != "" {
		proxy["sni"] = sni
	} else if peer := params["peer"]; peer != "" {
		proxy["sni"] = peer
	}

	// ALPN
	if alpn := params["alpn"]; alpn != "" {
		proxy["alpn"] = strings.Split(alpn, ",")
	}

	// Skip cert verify
	if params["insecure"] == "1" || params["allowInsecure"] == "1" || params["skip-cert-verify"] == "1" {
		proxy["skip-cert-verify"] = true
	}

	// Client fingerprint
	if fp := params["fp"]; fp != "" {
		proxy["client-fingerprint"] = fp
	}

	return proxy, nil
}

// parseSocksURI 解析 socks:// 或 socks5:// URI
func parseSocksURI(uri string) (map[string]interface{}, error) {
	var content string
	isPlainAuth := false

	if strings.HasPrefix(uri, "socks5://") {
		content = strings.TrimPrefix(uri, "socks5://")
		isPlainAuth = true
	} else {
		content = strings.TrimPrefix(uri, "socks://")
	}

	// 提取名称
	name := ""
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		name = urlDecode(content[idx+1:])
		content = content[:idx]
	}

	// 移除查询参数
	if idx := strings.Index(content, "?"); idx != -1 {
		content = content[:idx]
	}

	var server string
	var port int
	var username, password string

	atIdx := strings.LastIndex(content, "@")
	if atIdx != -1 {
		authPart := content[:atIdx]
		serverPart := content[atIdx+1:]

		server, port = parseServerPort(serverPart)

		if isPlainAuth {
			// socks5:// 格式: user:password 是明文
			colonIdx := strings.Index(authPart, ":")
			if colonIdx != -1 {
				username = urlDecode(authPart[:colonIdx])
				password = urlDecode(authPart[colonIdx+1:])
			} else {
				username = urlDecode(authPart)
			}
		} else {
			// socks:// 格式: user:password 是 base64 编码的
			decoded, _ := base64.StdEncoding.DecodeString(authPart)
			if len(decoded) == 0 {
				decoded, _ = base64.RawStdEncoding.DecodeString(authPart)
			}
			if len(decoded) > 0 {
				decodedStr := string(decoded)
				colonIdx := strings.Index(decodedStr, ":")
				if colonIdx != -1 {
					username = decodedStr[:colonIdx]
					password = decodedStr[colonIdx+1:]
				} else {
					username = decodedStr
				}
			}
		}
	} else {
		server, port = parseServerPort(content)
	}

	if server == "" || port == 0 {
		return nil, fmt.Errorf("invalid socks uri: invalid server:port")
	}

	if name == "" {
		name = fmt.Sprintf("%s:%d", server, port)
	}

	proxy := map[string]interface{}{
		"type":   "socks5",
		"name":   name,
		"server": server,
		"port":   port,
		"udp":    true,
	}

	if username != "" {
		proxy["username"] = username
	}
	if password != "" {
		proxy["password"] = password
	}

	return proxy, nil
}

// parseServerPort 解析 server:port 格式，支持 IPv6
func parseServerPort(serverPort string) (string, int) {
	// 处理 IPv6 地址 [ipv6]:port
	if strings.HasPrefix(serverPort, "[") {
		closeBracketIdx := strings.Index(serverPort, "]")
		if closeBracketIdx != -1 {
			server := serverPort[1:closeBracketIdx]
			portPart := serverPort[closeBracketIdx+1:]
			portPart = strings.TrimPrefix(portPart, ":")
			port, _ := strconv.Atoi(portPart)
			return server, port
		}
	}

	// IPv4 或域名
	lastColonIdx := strings.LastIndex(serverPort, ":")
	if lastColonIdx == -1 {
		return serverPort, 0
	}

	server := serverPort[:lastColonIdx]
	port, _ := strconv.Atoi(serverPort[lastColonIdx+1:])
	return server, port
}

// urlDecode 安全的 URL 解码
func urlDecode(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// getStringValue 从 map 中获取字符串值
func getStringValue(m map[string]interface{}, key string, defaultValue string) string {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			return val
		default:
			return fmt.Sprintf("%v", val)
		}
	}
	return defaultValue
}

// getIntValue 从 map 中获取整数值
func getIntValue(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		case string:
			if i, err := strconv.Atoi(val); err == nil {
				return i
			}
		}
	}
	return 0
}

// FetchAndFilterProxiesYAML fetches proxies from external subscription and applies filters
// Returns YAML bytes preserving original field order with 2-space indentation
func FetchAndFilterProxiesYAML(sub *storage.ExternalSubscription, config *storage.ProxyProviderConfig) ([]byte, error) {
	// Fetch subscription content (with caching)
	body, err := fetchSubscriptionContent(sub)
	if err != nil {
		return nil, err
	}

	// Preprocess content (handle base64 encoding and URI list conversion)
	body, err = preprocessSubscriptionContent(body)
	if err != nil {
		return nil, fmt.Errorf("preprocess subscription content: %w", err)
	}

	// Parse YAML content using yaml.Node to preserve order
	var rootNode yaml.Node
	if err := yaml.Unmarshal(body, &rootNode); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Find proxies node
	proxiesNode := findProxiesNode(&rootNode)
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("no proxies found in subscription")
	}

	// Apply filters to proxies node
	filteredProxiesNode := applyFiltersToNode(proxiesNode, config)

	// Apply overrides to proxies node
	if config.Override != "" {
		applyOverridesToNode(filteredProxiesNode, config.Override)
	}

	// Reorder proxy fields (name, type, server, port first)
	reorderProxiesNode(filteredProxiesNode)

	// Build output document
	outputDoc := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{
			{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "proxies"},
					filteredProxiesNode,
				},
			},
		},
	}

	// Encode with 2-space indentation
	// Sanitize explicit string tags before encoding to prevent !!str from appearing in output
	sanitizeExplicitStringTags(outputDoc)

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(outputDoc); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	encoder.Close()

	// Fix emoji escapes and quoted numbers
	result := RemoveUnicodeEscapeQuotes(buf.String())
	return []byte(result), nil
}

// findProxiesNode finds the proxies node in YAML document
func findProxiesNode(root *yaml.Node) *yaml.Node {
	if root == nil {
		return nil
	}

	// Handle document node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return findProxiesNode(root.Content[0])
	}

	// Handle mapping node
	if root.Kind == yaml.MappingNode {
		for i := 0; i < len(root.Content)-1; i += 2 {
			keyNode := root.Content[i]
			valueNode := root.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "proxies" {
				return valueNode
			}
		}
	}

	return nil
}

// fetchSubscriptionNodeNames fetches subscription content and returns all node names
func fetchSubscriptionNodeNames(sub *storage.ExternalSubscription) ([]string, error) {
	// Fetch subscription content (with caching)
	body, err := fetchSubscriptionContent(sub)
	if err != nil {
		return nil, err
	}

	// Preprocess content (handle base64 encoding)
	body, err = preprocessSubscriptionContent(body)
	if err != nil {
		return nil, fmt.Errorf("preprocess subscription content: %w", err)
	}

	// Parse YAML content
	var rootNode yaml.Node
	if err := yaml.Unmarshal(body, &rootNode); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Find proxies node
	proxiesNode := findProxiesNode(&rootNode)
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("no proxies found in subscription")
	}

	// Extract node names
	var nodeNames []string
	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}

		// Find "name" field
		for i := 0; i < len(proxyNode.Content)-1; i += 2 {
			keyNode := proxyNode.Content[i]
			valueNode := proxyNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" && valueNode.Kind == yaml.ScalarNode {
				nodeNames = append(nodeNames, valueNode.Value)
				break
			}
		}
	}

	return nodeNames, nil
}

// NodeInfo 节点信息（名称和服务器地址）
type NodeInfo struct {
	Name   string `json:"name"`
	Server string `json:"server"`
}

// fetchSubscriptionNodes fetches subscription content and returns all nodes with name and server
func fetchSubscriptionNodes(sub *storage.ExternalSubscription) ([]NodeInfo, error) {
	// Fetch subscription content (with caching)
	body, err := fetchSubscriptionContent(sub)
	if err != nil {
		return nil, err
	}

	// Preprocess content (handle base64 encoding)
	body, err = preprocessSubscriptionContent(body)
	if err != nil {
		return nil, fmt.Errorf("preprocess subscription content: %w", err)
	}

	// Parse YAML content
	var rootNode yaml.Node
	if err := yaml.Unmarshal(body, &rootNode); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Find proxies node
	proxiesNode := findProxiesNode(&rootNode)
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("no proxies found in subscription")
	}

	// Extract node info (name and server)
	var nodes []NodeInfo
	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}

		node := NodeInfo{}
		for i := 0; i < len(proxyNode.Content)-1; i += 2 {
			keyNode := proxyNode.Content[i]
			valueNode := proxyNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode && valueNode.Kind == yaml.ScalarNode {
				switch keyNode.Value {
				case "name":
					node.Name = valueNode.Value
				case "server":
					node.Server = valueNode.Value
				}
			}
		}
		if node.Name != "" {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// checkFilterMatches checks if filter/exclude-filter/geo-ip-filter matches any nodes
// Returns the count of matching nodes
func checkFilterMatches(sub *storage.ExternalSubscription, filter, excludeFilter, geoIPFilter string) (int, error) {
	// Fetch nodes
	nodes, err := fetchSubscriptionNodes(sub)
	if err != nil {
		return 0, err
	}

	logger.Info("[checkFilterMatches] 订阅节点信息", "sub_name", sub.Name, "node_count", len(nodes), "filter", filter, "exclude_filter", excludeFilter, "geo_ip_filter", geoIPFilter)

	// Compile regexes
	var filterRegex, excludeRegex *regexp.Regexp

	if filter != "" {
		filterRegex, err = regexp.Compile(filter)
		if err != nil {
			logger.Info("[checkFilterMatches] 无效的过滤正则表达式", "error", err)
			return 0, fmt.Errorf("invalid filter regex: %w", err)
		}
	}

	if excludeFilter != "" {
		excludeRegex, err = regexp.Compile(excludeFilter)
		if err != nil {
			logger.Info("[checkFilterMatches] 无效的排除过滤正则表达式", "error", err)
			return 0, fmt.Errorf("invalid exclude-filter regex: %w", err)
		}
	}

	// Build GeoIP filter country codes map
	geoIPCountryCodes := make(map[string]bool)
	if geoIPFilter != "" {
		for _, code := range strings.Split(geoIPFilter, ",") {
			geoIPCountryCodes[strings.TrimSpace(strings.ToUpper(code))] = true
		}
	}

	// Count matching nodes
	matchCount := 0
	for _, node := range nodes {
		// Apply exclude-filter first (remove matching names)
		if excludeRegex != nil && excludeRegex.MatchString(node.Name) {
			continue
		}

		// Apply filter and GeoIP matching
		if filterRegex != nil {
			if filterRegex.MatchString(node.Name) {
				// Node name matches filter regex, count it
				matchCount++
				continue
			}

			// Node name doesn't match, check GeoIP if available
			if len(geoIPCountryCodes) > 0 && node.Server != "" {
				countryCode := getGeoIPCountryCode(node.Server)
				if countryCode != "" && geoIPCountryCodes[countryCode] {
					// IP location matches, count it
					matchCount++
					continue
				}
			}
			// Neither regex nor GeoIP matched, skip this node
			continue
		}

		// No filter regex, only GeoIP filter
		if len(geoIPCountryCodes) > 0 {
			if node.Server != "" {
				countryCode := getGeoIPCountryCode(node.Server)
				if countryCode != "" && geoIPCountryCodes[countryCode] {
					matchCount++
				}
			}
			continue
		}

		// No filter at all, count all nodes
		matchCount++
	}

	logger.Info("[checkFilterMatches] 匹配结果", "filter", filter, "geo_ip_filter", geoIPFilter, "match_count", matchCount)
	return matchCount, nil
}

// reorderProxiesNode reorders fields in each proxy node using util.ReorderProxyNode
func reorderProxiesNode(proxiesNode *yaml.Node) {
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return
	}

	for i, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind == yaml.MappingNode {
			proxiesNode.Content[i] = util.ReorderProxyNode(proxyNode)
		}
	}
}

// applyFiltersToNode applies filter, exclude-filter, exclude-type and geo-ip-filter to proxies node
func applyFiltersToNode(proxiesNode *yaml.Node, config *storage.ProxyProviderConfig) *yaml.Node {
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return proxiesNode
	}

	result := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: make([]*yaml.Node, 0),
	}

	// Compile regexes
	var filterRegex, excludeRegex *regexp.Regexp
	var err error

	if config.Filter != "" {
		filterRegex, err = regexp.Compile(config.Filter)
		if err != nil {
			logger.Info("[ProxyProviderServe] 无效的过滤正则表达式", "error", err)
			filterRegex = nil
		}
	}

	if config.ExcludeFilter != "" {
		excludeRegex, err = regexp.Compile(config.ExcludeFilter)
		if err != nil {
			logger.Info("[ProxyProviderServe] 无效的排除过滤正则表达式", "error", err)
			excludeRegex = nil
		}
	}

	logger.Info("[applyFiltersToNode] 配置过滤器信息", "config_name", config.Name, "filter", config.Filter, "exclude_filter_len", len(config.ExcludeFilter), "exclude_filter", config.ExcludeFilter, "filter_regex_valid", filterRegex != nil, "exclude_regex_valid", excludeRegex != nil)

	// Build exclude type map
	excludeTypeMap := make(map[string]bool)
	if config.ExcludeType != "" {
		excludeTypes := strings.Split(config.ExcludeType, ",")
		for _, t := range excludeTypes {
			excludeTypeMap[strings.TrimSpace(strings.ToLower(t))] = true
		}
	}

	// Build GeoIP filter country codes map
	geoIPCountryCodes := make(map[string]bool)
	if config.GeoIPFilter != "" {
		for _, code := range strings.Split(config.GeoIPFilter, ",") {
			geoIPCountryCodes[strings.TrimSpace(strings.ToUpper(code))] = true
		}
	}

	// Filter proxies
	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}

		// Extract name, type and server from proxy node
		name := util.GetNodeFieldValue(proxyNode, "name")
		proxyType := util.GetNodeFieldValue(proxyNode, "type")
		server := util.GetNodeFieldValue(proxyNode, "server")

		// Apply exclude-filter first (remove matching names)
		if excludeRegex != nil && excludeRegex.MatchString(name) {
			logger.Info("[applyFiltersToNode] 排除节点(excludeFilter): %s", name)
			continue
		}

		// Apply exclude-type (remove matching protocol types)
		if len(excludeTypeMap) > 0 && excludeTypeMap[strings.ToLower(proxyType)] {
			continue
		}

		// Apply filter and GeoIP matching
		// If filter is set, use it as primary matching method
		// If GeoIP filter is also set, nodes not matching the regex can still be included if IP matches
		if filterRegex != nil {
			if filterRegex.MatchString(name) {
				// Node name matches filter regex, include it
				result.Content = append(result.Content, proxyNode)
				continue
			}

			// Node name doesn't match, check GeoIP if available
			if len(geoIPCountryCodes) > 0 && server != "" {
				countryCode := getGeoIPCountryCode(server)
				if countryCode != "" && geoIPCountryCodes[countryCode] {
					// IP location matches, include the node
					result.Content = append(result.Content, proxyNode)
					continue
				}
			}
			// Neither regex nor GeoIP matched, skip this node
			continue
		}

		// No filter regex, only GeoIP filter
		if len(geoIPCountryCodes) > 0 {
			if server != "" {
				countryCode := getGeoIPCountryCode(server)
				if countryCode != "" && geoIPCountryCodes[countryCode] {
					result.Content = append(result.Content, proxyNode)
				}
			}
			continue
		}

		// No filter at all, include the node
		result.Content = append(result.Content, proxyNode)
	}

	return result
}

// applyOverridesToNode applies override configuration to proxies node
func applyOverridesToNode(proxiesNode *yaml.Node, overrideJSON string) {
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode || overrideJSON == "" {
		return
	}

	var overrides map[string]any
	if err := json.Unmarshal([]byte(overrideJSON), &overrides); err != nil {
		logger.Info("[ProxyProviderServe] 无效的覆盖JSON配置", "error", err)
		return
	}

	// Apply overrides to each proxy
	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}

		for key, value := range overrides {
			util.SetNodeField(proxyNode, key, value)
		}
	}
}

// createEmptyCacheEntry 创建空缓存条目
func createEmptyCacheEntry(sub *storage.ExternalSubscription, config *storage.ProxyProviderConfig) *CacheEntry {
	return &CacheEntry{
		ConfigID:  config.ID,
		YAMLData:  []byte("proxies: []\n"),
		Nodes:     []any{},
		NodeNames: []string{},
		Prefix:    sub.Name,
		FetchedAt: time.Now(),
		Interval:  config.Interval,
		NodeCount: 0,
	}
}

// RefreshProxyProviderCache 刷新代理集合缓存
func RefreshProxyProviderCache(sub *storage.ExternalSubscription, config *storage.ProxyProviderConfig) (*CacheEntry, error) {
	// 拉取并过滤节点
	yamlBytes, err := FetchAndFilterProxiesYAML(sub, config)
	if err != nil {
		return nil, fmt.Errorf("fetch and filter proxies: %w", err)
	}

	// 检查返回内容是否为空
	if len(yamlBytes) == 0 {
		logger.Info("[RefreshProxyProviderCache] 配置返回空内容", "config_id", config.ID)
		entry := createEmptyCacheEntry(sub, config)
		cache := GetProxyProviderCache()
		cache.Set(config.ID, entry)
		return entry, nil
	}

	// 解析 YAML 获取节点列表
	var result map[string]any
	if err := yaml.Unmarshal(yamlBytes, &result); err != nil {
		// YAML 解析失败，记录日志并返回空缓存（而不是报错）
		contentPreview := string(yamlBytes)
		if len(contentPreview) > 200 {
			contentPreview = contentPreview[:200] + "..."
		}
		logger.Info("[RefreshProxyProviderCache] YAML解析失败", "config_id", config.ID, "error", err, "content_preview", contentPreview)
		entry := createEmptyCacheEntry(sub, config)
		cache := GetProxyProviderCache()
		cache.Set(config.ID, entry)
		return entry, nil
	}

	proxiesRaw, ok := result["proxies"].([]any)
	if !ok {
		proxiesRaw = []any{}
	}

	// 提取节点名称（使用订阅名称作为前缀标识）
	prefix := sub.Name
	nodeNames := make([]string, 0, len(proxiesRaw))
	for _, p := range proxiesRaw {
		if m, ok := p.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				nodeNames = append(nodeNames, name)
			}
		}
	}

	// 创建缓存条目
	entry := &CacheEntry{
		ConfigID:  config.ID,
		YAMLData:  yamlBytes,
		Nodes:     proxiesRaw,
		NodeNames: nodeNames,
		Prefix:    prefix,
		FetchedAt: time.Now(),
		Interval:  config.Interval,
		NodeCount: len(proxiesRaw),
	}

	// 存入缓存
	cache := GetProxyProviderCache()
	cache.Set(config.ID, entry)

	logger.Info("[RefreshProxyProviderCache] 刷新缓存成功", "id", config.ID, "node_count", len(proxiesRaw))
	return entry, nil
}
