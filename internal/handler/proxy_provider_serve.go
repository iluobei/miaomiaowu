package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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
			log.Printf("[GeoIP] Failed to resolve domain %s: %v", ipOrHost, err)
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
		log.Printf("[GeoIP] Failed to query IP %s: %v", ip, err)
		// 缓存空结果避免重复查询
		geoIPCache.Store(ip, "")
		return ""
	}
	defer resp.Body.Close()

	var result geoIPResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[GeoIP] Failed to decode response for IP %s: %v", ip, err)
		geoIPCache.Store(ip, "")
		return ""
	}

	// 缓存结果
	countryCode := strings.ToUpper(result.CountryCode)
	geoIPCache.Store(ip, countryCode)
	log.Printf("[GeoIP] IP %s -> Country: %s", ip, countryCode)
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
			log.Printf("[ProxyProviderServe] 使用缓存 ID=%d, 节点数=%d", configID, entry.NodeCount)
			w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(entry.YAMLData)
			return
		}

		// 缓存未命中或过期，拉取新数据
		entry, err := RefreshProxyProviderCache(&sub, config)
		if err != nil {
			log.Printf("[ProxyProviderServe] Failed to fetch proxies for config %d: %v", configID, err)
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
			log.Printf("[SubscriptionCache] Hit for URL: %s", sub.URL)
			return entry.content, nil
		}
		// 缓存过期，删除
		subscriptionCache.Delete(cacheKey)
	}

	log.Printf("[SubscriptionCache] Miss for URL: %s, fetching...", sub.URL)

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

// FetchAndFilterProxiesYAML fetches proxies from external subscription and applies filters
// Returns YAML bytes preserving original field order with 2-space indentation
func FetchAndFilterProxiesYAML(sub *storage.ExternalSubscription, config *storage.ProxyProviderConfig) ([]byte, error) {
	// Fetch subscription content (with caching)
	body, err := fetchSubscriptionContent(sub)
	if err != nil {
		return nil, err
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

	log.Printf("[checkFilterMatches] 订阅 %s 共 %d 个节点, filter=%s, excludeFilter=%s, geoIPFilter=%s",
		sub.Name, len(nodes), filter, excludeFilter, geoIPFilter)

	// Compile regexes
	var filterRegex, excludeRegex *regexp.Regexp

	if filter != "" {
		filterRegex, err = regexp.Compile(filter)
		if err != nil {
			log.Printf("[checkFilterMatches] Invalid filter regex: %v", err)
			return 0, fmt.Errorf("invalid filter regex: %w", err)
		}
	}

	if excludeFilter != "" {
		excludeRegex, err = regexp.Compile(excludeFilter)
		if err != nil {
			log.Printf("[checkFilterMatches] Invalid exclude-filter regex: %v", err)
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

	log.Printf("[checkFilterMatches] 匹配结果: filter=%s, geoIPFilter=%s, matchCount=%d", filter, geoIPFilter, matchCount)
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
			log.Printf("[ProxyProviderServe] Invalid filter regex: %v", err)
			filterRegex = nil
		}
	}

	if config.ExcludeFilter != "" {
		excludeRegex, err = regexp.Compile(config.ExcludeFilter)
		if err != nil {
			log.Printf("[ProxyProviderServe] Invalid exclude-filter regex: %v", err)
			excludeRegex = nil
		}
	}

	log.Printf("[applyFiltersToNode] 配置 %s: filter=%q, excludeFilter(len=%d)=%q, filterRegex=%v, excludeRegex=%v",
		config.Name, config.Filter, len(config.ExcludeFilter), config.ExcludeFilter, filterRegex != nil, excludeRegex != nil)

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
			log.Printf("[applyFiltersToNode] 排除节点(excludeFilter): %s", name)
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
		log.Printf("[ProxyProviderServe] Invalid override JSON: %v", err)
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
		log.Printf("[RefreshProxyProviderCache] 配置 %d 返回空内容", config.ID)
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
		log.Printf("[RefreshProxyProviderCache] 配置 %d YAML 解析失败: %v, 原始内容: %s", config.ID, err, contentPreview)
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

	log.Printf("[RefreshProxyProviderCache] 刷新缓存成功 ID=%d, 节点数=%d", config.ID, len(proxiesRaw))
	return entry, nil
}
