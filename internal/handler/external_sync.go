package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"miaomiaowu/internal/logger"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/util"

	"gopkg.in/yaml.v3"
)

// syncExternalSubscriptionsManual is for manual sync triggered by user - syncs ALL external subscriptions regardless of ForceSyncExternal setting
func syncExternalSubscriptionsManual(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string) error {
	if repo == nil || username == "" {
		return fmt.Errorf("invalid parameters")
	}

	logger.Info("[外部订阅同步-手动] 开始手动同步外部订阅", "user", username)

	// Get user settings to check match rule (but ignore ForceSyncExternal for manual sync)
	userSettings, err := repo.GetUserSettings(ctx, username)
	if err != nil {
		logger.Info("[外部订阅同步-手动] 获取用户设置失败，使用默认设置", "error", err)
		userSettings.MatchRule = "node_name"
		userSettings.SyncScope = "saved_only"
		userSettings.KeepNodeName = true
	}

	matchRuleDesc := map[string]string{
		"node_name":        "节点名称",
		"server_port":      "服务器:端口",
		"type_server_port": "类型:服务器:端口",
	}
	syncScopeDesc := map[string]string{
		"saved_only": "仅同步已保存节点",
		"all":        "同步所有节点",
	}

	logger.Info("[外部订阅同步-手动] 同步配置",
		"match_rule", userSettings.MatchRule,
		"match_rule_desc", matchRuleDesc[userSettings.MatchRule],
		"sync_scope", userSettings.SyncScope,
		"sync_scope_desc", syncScopeDesc[userSettings.SyncScope],
		"keep_node_name", userSettings.KeepNodeName)

	// Get user's external subscriptions
	externalSubs, err := repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		logger.Info("[外部订阅同步-手动] 获取外部订阅列表失败", "error", err)
		return fmt.Errorf("list external subscriptions: %w", err)
	}

	if len(externalSubs) == 0 {
		logger.Info("[外部订阅同步-手动] 没有配置外部订阅，跳过同步", "user", username)
		return nil
	}

	logger.Info("[外部订阅同步-手动] 外部订阅数量", "user", username, "count", len(externalSubs))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track total nodes synced
	totalNodesSynced := 0

	for i, sub := range externalSubs {
		logger.Info("[外部订阅同步-手动] 开始同步订阅", "index", i+1, "total", len(externalSubs), "name", sub.Name)
		nodeCount, updatedSub, err := syncSingleExternalSubscription(ctx, client, repo, subscribeDir, username, sub, userSettings)
		if err != nil {
			logger.Info("[外部订阅同步-手动] 同步订阅失败", "index", i+1, "total", len(externalSubs), "name", sub.Name, "error", err)
			continue
		}

		totalNodesSynced += nodeCount

		// Update last sync time and node count
		now := time.Now()
		updatedSub.LastSyncAt = &now
		updatedSub.NodeCount = nodeCount
		if err := repo.UpdateExternalSubscription(ctx, updatedSub); err != nil {
			logger.Info("[外部订阅同步-手动] 更新订阅同步时间失败", "name", sub.Name, "error", err)
		}
		logger.Info("[外部订阅同步-手动] 订阅同步完成", "index", i+1, "total", len(externalSubs), "name", sub.Name, "node_count", nodeCount)
	}

	logger.Info("[外部订阅同步-手动] 同步完成", "user", username, "subscription_count", len(externalSubs), "total_nodes", totalNodesSynced)

	return nil
}

// syncExternalSubscriptions fetches nodes from all external subscriptions and updates the node table
func syncExternalSubscriptions(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string) error {
	if repo == nil || username == "" {
		return fmt.Errorf("invalid parameters")
	}

	logger.Info("[外部订阅同步-自动] 用户 开始自动同步外部订阅", "user", username)

	// Get user settings to check match rule and ForceSyncExternal
	userSettings, err := repo.GetUserSettings(ctx, username)
	if err != nil {
		logger.Info("[外部订阅同步-自动] 获取用户设置失败，使用默认设置", "error", err)
		userSettings.MatchRule = "node_name"
		userSettings.SyncScope = "saved_only"
		userSettings.KeepNodeName = true
		userSettings.ForceSyncExternal = false
	}

	matchRuleDesc := map[string]string{
		"node_name":        "节点名称",
		"server_port":      "服务器:端口",
		"type_server_port": "类型:服务器:端口",
	}
	syncScopeDesc := map[string]string{
		"saved_only": "仅同步已保存节点",
		"all":        "同步所有节点",
	}

	logger.Info("[外部订阅同步-自动] 同步配置",
		"match_rule", userSettings.MatchRule,
		"match_rule_desc", matchRuleDesc[userSettings.MatchRule],
		"sync_scope", userSettings.SyncScope,
		"sync_scope_desc", syncScopeDesc[userSettings.SyncScope],
		"keep_node_name", userSettings.KeepNodeName)

	// Get user's external subscriptions
	externalSubs, err := repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		logger.Info("[外部订阅同步-自动] 获取外部订阅列表失败", "error", err)
		return fmt.Errorf("list external subscriptions: %w", err)
	}

	if len(externalSubs) == 0 {
		logger.Info("[外部订阅同步-自动] 用户 没有配置外部订阅，跳过同步", "user", username)
		return nil
	}

	// If ForceSyncExternal is enabled, only sync subscriptions used in config files
	var subsToSync []storage.ExternalSubscription
	if userSettings.ForceSyncExternal {
		logger.Info("[外部订阅同步-自动] 强制同步已开启，正在筛选配置文件中使用的订阅...")
		usedURLs, err := getUsedExternalSubscriptionURLs(ctx, repo, subscribeDir, username)
		if err != nil {
			logger.Info("[外部订阅同步-自动] 获取配置文件中使用的订阅URL失败，将同步所有订阅", "error", err)
			subsToSync = externalSubs
		} else {
			// Filter subscriptions to only those used in config files
			for _, sub := range externalSubs {
				if _, used := usedURLs[sub.URL]; used {
					subsToSync = append(subsToSync, sub)
					logger.Info("[外部订阅同步-自动] 订阅 在配置文件中被使用，将进行同步", "name", sub.Name)
				} else {
					logger.Info("[外部订阅同步-自动] 订阅 未在配置文件中使用，跳过同步", "name", sub.Name)
				}
			}
			logger.Info("[外部订阅同步-自动] 筛选完成", "sync_count", len(subsToSync), "total_count", len(externalSubs))
		}
	} else {
		subsToSync = externalSubs
	}

	if len(subsToSync) == 0 {
		logger.Info("[外部订阅同步-自动] 用户 没有需要同步的订阅", "user", username)
		return nil
	}

	logger.Info("[外部订阅同步-自动] 用户共有外部订阅需要同步", "user", username, "count", len(subsToSync))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track total nodes synced
	totalNodesSynced := 0

	for i, sub := range subsToSync {
		logger.Info("[外部订阅同步-自动] 开始同步订阅", "index", i+1, "total", len(subsToSync), "name", sub.Name)
		nodeCount, updatedSub, err := syncSingleExternalSubscription(ctx, client, repo, subscribeDir, username, sub, userSettings)
		if err != nil {
			logger.Info("[外部订阅同步-自动] 同步订阅失败", "index", i+1, "total", len(subsToSync), "name", sub.Name, "error", err)
			continue
		}

		totalNodesSynced += nodeCount

		// Update last sync time and node count
		// Use updatedSub which contains traffic info from parseAndUpdateTrafficInfo
		now := time.Now()
		updatedSub.LastSyncAt = &now
		updatedSub.NodeCount = nodeCount
		if err := repo.UpdateExternalSubscription(ctx, updatedSub); err != nil {
			logger.Info("[外部订阅同步-自动] 更新订阅 的同步时间失败", "name", sub.Name, "error", err)
		}
		logger.Info("[外部订阅同步-自动] 订阅同步完成", "index", i+1, "total", len(subsToSync), "name", sub.Name, "node_count", nodeCount)
	}

	logger.Info("[外部订阅同步-自动] 用户同步完成", "user", username, "subscription_count", len(subsToSync), "total_nodes", totalNodesSynced)

	return nil
}

// getUsedExternalSubscriptionURLs extracts all external subscription URLs used in user's subscribe files
func getUsedExternalSubscriptionURLs(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string) (map[string]bool, error) {
	usedURLs := make(map[string]bool)

	if subscribeDir == "" {
		return usedURLs, fmt.Errorf("subscribe directory not configured")
	}

	// Get all subscribe files for the user
	allFiles, err := repo.ListSubscribeFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscribe files: %w", err)
	}

	// Read each YAML file from the subscribe directory
	for _, file := range allFiles {
		// Read the YAML file from disk
		filePath := fmt.Sprintf("%s/%s", subscribeDir, file.Filename)
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Info("[External Sync] Failed to read file", "value", filePath, "error", err)
			continue
		}

		// Parse YAML content
		var yamlContent map[string]any
		if err := yaml.Unmarshal(content, &yamlContent); err != nil {
			logger.Info("[External Sync] Failed to parse YAML for file", "name", file.Name, "error", err)
			continue
		}

		// Extract proxy-providers URLs
		if proxyProviders, ok := yamlContent["proxy-providers"].(map[string]any); ok {
			for _, provider := range proxyProviders {
				if providerMap, ok := provider.(map[string]any); ok {
					if url, ok := providerMap["url"].(string); ok && url != "" {
						usedURLs[url] = true
						logger.Info("[External Sync] Found used subscription URL in file", "name", file.Name, "param", url)
					}
				}
			}
		}
	}

	return usedURLs, nil
}

// syncSingleExternalSubscription fetches and syncs nodes from a single external subscription
// Returns: node count, updated subscription info, error
func syncSingleExternalSubscription(ctx context.Context, client *http.Client, repo *storage.TrafficRepository, subscribeDir, username string, sub storage.ExternalSubscription, settings storage.UserSettings) (int, storage.ExternalSubscription, error) {
	matchRule := settings.MatchRule
	syncScope := settings.SyncScope
	keepNodeName := settings.KeepNodeName

	logger.Info("[外部订阅同步] 开始获取订阅内容", "name", sub.Name, "url", sub.URL)

	// Fetch subscription content
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.URL, nil)
	if err != nil {
		logger.Info("[外部订阅同步] 创建HTTP请求失败", "error", err)
		return 0, sub, fmt.Errorf("create request: %w", err)
	}

	// 使用订阅保存的 User-Agent，如果为空则使用默认值
	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	req.Header.Set("User-Agent", userAgent)
	logger.Info("[外部订阅同步] 使用 User-Agent", "user_agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		logger.Info("[外部订阅同步] 请求订阅URL失败", "error", err)
		return 0, sub, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	logger.Info("[外部订阅同步] HTTP响应状态码", "status_code", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		logger.Info("[外部订阅同步] 订阅返回非200状态码", "status_code", resp.StatusCode)
		return 0, sub, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse subscription-userinfo header if sync_traffic is enabled
	if settings.SyncTraffic {
		userInfo := resp.Header.Get("subscription-userinfo")
		if userInfo != "" {
			logger.Info("[外部订阅同步] 发现流量信息头，开始解析...")
			parseAndUpdateTrafficInfo(ctx, repo, &sub, userInfo)
		} else if !strings.Contains(strings.ToLower(userAgent), "clash") {
			// 如果使用的不是 clash UA 且没有获取到流量信息，尝试用 clash-meta UA 再请求一次
			logger.Info("[外部订阅同步] 未获取到流量信息，尝试使用 clash-meta UA 获取", "name", sub.Name)
			clashMetaUA := "clash-meta/2.4.0"
			trafficReq, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.URL, nil)
			if err == nil {
				trafficReq.Header.Set("User-Agent", clashMetaUA)
				trafficResp, err := client.Do(trafficReq)
				if err == nil {
					defer trafficResp.Body.Close()
					if trafficResp.StatusCode == http.StatusOK {
						trafficUserInfo := trafficResp.Header.Get("subscription-userinfo")
						if trafficUserInfo != "" {
							logger.Info("[外部订阅同步] clash-meta UA 获取流量信息成功", "name", sub.Name)
							parseAndUpdateTrafficInfo(ctx, repo, &sub, trafficUserInfo)
						}
					}
				} else {
					logger.Info("[外部订阅同步] clash-meta UA 请求失败", "error", err)
				}
			}
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Info("[外部订阅同步] 读取响应内容失败", "error", err)
		return 0, sub, fmt.Errorf("read response body: %w", err)
	}

	logger.Info("[外部订阅同步] 成功获取订阅内容", "size", len(body))

	var proxies []any

	// 首先尝试解析为 YAML (Clash 格式)
	var yamlContent map[string]any
	if err := yaml.Unmarshal(body, &yamlContent); err == nil {
		// YAML 解析成功，提取 proxies
		if p, ok := yamlContent["proxies"].([]any); ok && len(p) > 0 {
			proxies = p
			logger.Info("[外部订阅同步] 解析为 Clash YAML 格式", "name", sub.Name, "count", len(proxies))
		}
	}

	// 如果 YAML 解析失败或没有 proxies，尝试 v2ray 格式 (base64 编码的 URI 列表)
	if len(proxies) == 0 {
		logger.Info("[外部订阅同步] 尝试解析为 v2ray 格式", "name", sub.Name)
		v2rayProxies, err := ParseV2raySubscription(string(body))
		if err == nil && len(v2rayProxies) > 0 {
			// 将 map[string]any 转换为 []any
			for _, p := range v2rayProxies {
				proxies = append(proxies, p)
			}
			logger.Info("[外部订阅同步] 解析为 v2ray 格式成功", "name", sub.Name, "count", len(proxies))
		}
	}

	if len(proxies) == 0 {
		logger.Info("[外部订阅同步] 订阅中未找到节点(proxies)数据")
		return 0, sub, fmt.Errorf("no proxies found in subscription")
	}

	logger.Info("[外部订阅同步] 解析到节点", "name", sub.Name, "count", len(proxies))

	// Apply node name filter if configured
	nodeNameFilter := strings.TrimSpace(settings.NodeNameFilter)
	if nodeNameFilter != "" {
		filterRegex, err := regexp.Compile(nodeNameFilter)
		if err != nil {
			logger.Info("[外部订阅同步] 节点名称过滤正则表达式无效，跳过过滤", "pattern", nodeNameFilter, "error", err)
		} else {
			filteredProxies := make([]any, 0, len(proxies))
			filteredCount := 0
			for _, proxy := range proxies {
				if proxyMap, ok := proxy.(map[string]any); ok {
					if proxyName, ok := proxyMap["name"].(string); ok {
						if filterRegex.MatchString(proxyName) {
							filteredCount++
							logger.Info("[外部订阅同步] 过滤节点", "name", proxyName, "pattern", nodeNameFilter)
							continue
						}
					}
				}
				filteredProxies = append(filteredProxies, proxy)
			}
			if filteredCount > 0 {
				logger.Info("[外部订阅同步] 节点过滤完成", "filtered_count", filteredCount, "remaining_count", len(filteredProxies))
			}
			proxies = filteredProxies
		}
	}

	// Convert to storage.Node format
	nodesToUpdate := make([]storage.Node, 0, len(proxies))

	for _, proxy := range proxies {
		proxyMap, ok := proxy.(map[string]any)
		if !ok {
			continue
		}

		proxyName, ok := proxyMap["name"].(string)
		if !ok || proxyName == "" {
			continue
		}

		// Marshal proxy to JSON for storage
		clashConfigBytes, err := json.Marshal(proxyMap)
		if err != nil {
			continue
		}

		// Use clash config as parsed config as well
		parsedConfigBytes := clashConfigBytes

		// Determine protocol type
		protocol := "unknown"
		if proxyType, ok := proxyMap["type"].(string); ok {
			protocol = proxyType
		}

		node := storage.Node{
			Username:     username,
			RawURL:       sub.URL, // Save external subscription URL for tracking
			NodeName:     proxyName,
			Protocol:     protocol,
			ParsedConfig: string(parsedConfigBytes),
			ClashConfig:  string(clashConfigBytes),
			Enabled:      true,
			Tag:          sub.Name, // Use external subscription name as tag
		}

		nodesToUpdate = append(nodesToUpdate, node)
	}

	if len(nodesToUpdate) == 0 {
		logger.Info("[外部订阅同步] 没有有效的节点可以同步")
		return 0, sub, fmt.Errorf("no valid nodes to sync")
	}

	logger.Info("[外部订阅同步] 准备同步节点", "count", len(nodesToUpdate))

	// Get existing nodes once
	existingNodes, err := repo.ListNodes(ctx, username)
	if err != nil {
		logger.Info("[外部订阅同步] 获取已保存节点列表失败", "error", err)
		return 0, sub, fmt.Errorf("list existing nodes: %w", err)
	}

	logger.Info("[外部订阅同步] 数据库中已有节点", "count", len(existingNodes))

	// Sync nodes to database (replace nodes based on match rule)
	syncedCount := 0
	updatedCount := 0
	createdCount := 0
	skippedCount := 0

	for _, node := range nodesToUpdate {
		var existingNode *storage.Node

		// Parse new node's clash config for matching
		var newNodeClashConfig map[string]any
		if err := json.Unmarshal([]byte(node.ClashConfig), &newNodeClashConfig); err != nil {
			continue
		}

		newServer, _ := newNodeClashConfig["server"].(string)
		newPort := newNodeClashConfig["port"]
		newType, _ := newNodeClashConfig["type"].(string)

		// Match based on rule
		switch matchRule {
		case "type_server_port":
			// Match by type:server:port
			matchKey := fmt.Sprintf("%s:%s:%v", newType, newServer, newPort)
			if newServer != "" && newPort != nil && newType != "" {
				for i := range existingNodes {
					var existingClashConfig map[string]any
					if err := json.Unmarshal([]byte(existingNodes[i].ClashConfig), &existingClashConfig); err == nil {
						existingServer, _ := existingClashConfig["server"].(string)
						existingPort := existingClashConfig["port"]
						existingType, _ := existingClashConfig["type"].(string)

						// Compare type:server:port
						if existingType == newType && existingServer == newServer && fmt.Sprintf("%v", existingPort) == fmt.Sprintf("%v", newPort) {
							existingNode = &existingNodes[i]
							logger.Info("[外部订阅同步] 节点 按 type:server:port 匹配成功 -> 已有节点", "node_name", node.NodeName, "param", matchKey, "node_name", existingNode.NodeName)
							break
						}
					}
				}
				if existingNode == nil {
					logger.Info("[外部订阅同步] 节点 按 type:server:port 未找到匹配", "node_name", node.NodeName, "param", matchKey)
				}
			}
		case "server_port":
			// Match by server:port
			matchKey := fmt.Sprintf("%s:%v", newServer, newPort)
			if newServer != "" && newPort != nil {
				for i := range existingNodes {
					var existingClashConfig map[string]any
					if err := json.Unmarshal([]byte(existingNodes[i].ClashConfig), &existingClashConfig); err == nil {
						existingServer, _ := existingClashConfig["server"].(string)
						existingPort := existingClashConfig["port"]

						// Compare server:port
						if existingServer == newServer && fmt.Sprintf("%v", existingPort) == fmt.Sprintf("%v", newPort) {
							existingNode = &existingNodes[i]
							logger.Info("[外部订阅同步] 节点 按 server:port 匹配成功 -> 已有节点", "node_name", node.NodeName, "param", matchKey, "node_name", existingNode.NodeName)
							break
						}
					}
				}
				if existingNode == nil {
					logger.Info("[外部订阅同步] 节点 按 server:port 未找到匹配", "node_name", node.NodeName, "param", matchKey)
				}
			}
		default:
			// Default: match by node name
			for i := range existingNodes {
				if existingNodes[i].NodeName == node.NodeName {
					existingNode = &existingNodes[i]
					logger.Info("[外部订阅同步] 节点 按名称匹配成功", "node_name", node.NodeName)
					break
				}
			}
			if existingNode == nil {
				logger.Info("[外部订阅同步] 节点 按名称未找到匹配", "node_name", node.NodeName)
			}
		}

		if existingNode != nil {
			// Update existing node
			oldNodeName := existingNode.NodeName

			// Update node fields from external subscription
			existingNode.RawURL = node.RawURL
			existingNode.Protocol = node.Protocol
			existingNode.ParsedConfig = node.ParsedConfig
			existingNode.ClashConfig = node.ClashConfig
			existingNode.Enabled = node.Enabled
			existingNode.Tag = node.Tag

			// Handle node name based on keepNodeName setting
			if !keepNodeName {
				existingNode.NodeName = node.NodeName // Update to new name from external subscription
				if oldNodeName != node.NodeName {
					logger.Info("[外部订阅同步] 更新节点名称 ->", "value", oldNodeName, "node_name", node.NodeName)
				}
			} else {
				logger.Info("[外部订阅同步] 保留原节点名称 (外部订阅名称)", "value", oldNodeName, "node_name", node.NodeName)
				// 更新 ClashConfig 和 ParsedConfig 中的 name 字段为保留的节点名称
				var clashConfig map[string]any
				if err := json.Unmarshal([]byte(existingNode.ClashConfig), &clashConfig); err == nil {
					clashConfig["name"] = oldNodeName
					if updatedClash, err := json.Marshal(clashConfig); err == nil {
						existingNode.ClashConfig = string(updatedClash)
					}
				}
				var parsedConfig map[string]any
				if err := json.Unmarshal([]byte(existingNode.ParsedConfig), &parsedConfig); err == nil {
					parsedConfig["name"] = oldNodeName
					if updatedParsed, err := json.Marshal(parsedConfig); err == nil {
						existingNode.ParsedConfig = string(updatedParsed)
					}
				}
			}

			_, err := repo.UpdateNode(ctx, *existingNode)
			if err != nil {
				logger.Info("[外部订阅同步] 更新节点 失败", "node_name", existingNode.NodeName, "error", err)
				continue
			}

			logger.Info("[外部订阅同步] 成功更新节点 (ID)", "node_name", existingNode.NodeName, "id", existingNode.ID)

			// Sync to YAML files (handle name change if needed)
			if subscribeDir != "" {
				if err := syncNodeToYAMLFiles(subscribeDir, oldNodeName, existingNode.NodeName, existingNode.ClashConfig); err != nil {
					logger.Info("[外部订阅同步] 同步节点 到YAML文件失败", "node_name", existingNode.NodeName, "error", err)
				}
			}

			syncedCount++
			updatedCount++
		} else {
			// New node not found in existing nodes
			// Check sync scope: only create new nodes if syncScope is "all"
			if syncScope == "all" {
				_, err := repo.CreateNode(ctx, node)
				if err != nil {
					logger.Info("[外部订阅同步] 创建新节点 失败", "node_name", node.NodeName, "error", err)
					continue
				}
				logger.Info("[外部订阅同步] 成功创建新节点", "node_name", node.NodeName)
				syncedCount++
				createdCount++
			} else {
				logger.Info("[外部订阅同步] 跳过新节点 (同步范围: 仅已保存节点)", "node_name", node.NodeName)
				skippedCount++
			}
		}
	}

	logger.Info("[外部订阅同步] 订阅同步完成", "name", sub.Name, "synced_count", syncedCount, "total_count", len(nodesToUpdate), "updated", updatedCount, "created", createdCount, "skipped", skippedCount)

	// 同步代理集合节点到 YAML（仅处理 mmw 模式）
	if err := syncProxyProviderNodesToYAML(ctx, repo, subscribeDir, username, sub); err != nil {
		logger.Info("[外部订阅同步] 同步代理集合节点到YAML失败", "error", err)
		// 不影响主流程，仅记录日志
	}

	return syncedCount, sub, nil
}

// ParseTrafficInfoHeader parses subscription-userinfo header and returns traffic info
// Format: upload=0; download=685404160; total=1073741824; expire=1705276800
// This function only parses the header, does not update database
func ParseTrafficInfoHeader(userInfo string) (upload, download, total int64, expire *time.Time) {
	parts := strings.Split(userInfo, ";")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "upload":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				upload = v
			}
		case "download":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				download = v
			}
		case "total":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				total = v
			}
		case "expire":
			// 修复remnawave永不过期expire值为0导致过期时间错误设置为1970-01-01
			if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
				expireTime := time.Unix(v, 0)
				expire = &expireTime
			}
		}
	}

	return
}

// parseAndUpdateTrafficInfo parses subscription-userinfo header and updates traffic info
// Format: upload=0; download=685404160; total=1073741824; expire=1705276800
func parseAndUpdateTrafficInfo(ctx context.Context, repo *storage.TrafficRepository, sub *storage.ExternalSubscription, userInfo string) {
	logger.Info("[External Sync] Parsing traffic info for subscription ()", "name", sub.Name, "url", sub.URL)
	logger.Info("[External Sync] Raw subscription-userinfo", "value", userInfo)

	// Parse subscription-userinfo
	// Example: upload=0; download=685404160; total=1073741824; expire=1705276800
	parts := strings.Split(userInfo, ";")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "upload":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				sub.Upload = v
				logger.Info("[外部订阅同步] 解析上传流量", "bytes", v, "mb", float64(v)/(1024*1024))
			} else if f, err := strconv.ParseFloat(value, 64); err == nil {
				// 支持带小数点的值，取整
				sub.Upload = int64(f)
				logger.Info("[外部订阅同步] 解析上传流量(浮点)", "bytes", sub.Upload, "mb", f/(1024*1024))
			} else {
				logger.Info("[外部订阅同步] 解析上传流量失败", "value", value, "error", err)
			}
		case "download":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				sub.Download = v
				logger.Info("[外部订阅同步] 解析下载流量", "bytes", v, "mb", float64(v)/(1024*1024))
			} else if f, err := strconv.ParseFloat(value, 64); err == nil {
				// 支持带小数点的值，取整
				sub.Download = int64(f)
				logger.Info("[外部订阅同步] 解析下载流量(浮点)", "bytes", sub.Download, "mb", f/(1024*1024))
			} else {
				logger.Info("[外部订阅同步] 解析下载流量失败", "value", value, "error", err)
			}
		case "total":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				sub.Total = v
				logger.Info("[外部订阅同步] 解析总流量", "bytes", v, "gb", float64(v)/(1024*1024*1024))
			} else if f, err := strconv.ParseFloat(value, 64); err == nil {
				// 支持带小数点的值，取整
				sub.Total = int64(f)
				logger.Info("[外部订阅同步] 解析总流量(浮点)", "bytes", sub.Total, "gb", f/(1024*1024*1024))
			} else {
				logger.Info("[外部订阅同步] 解析总流量失败", "value", value, "error", err)
			}
		case "expire":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
				expireTime := time.Unix(v, 0)
				sub.Expire = &expireTime
				logger.Info("[外部订阅同步] 解析过期时间", "expire", expireTime.Format("2006-01-02 15:04:05"))
			} else if f, err := strconv.ParseFloat(value, 64); err == nil && int64(f) > 0 {
				expireTime := time.Unix(int64(f), 0)
				sub.Expire = &expireTime
				logger.Info("[外部订阅同步] 解析过期时间(浮点)", "expire", expireTime.Format("2006-01-02 15:04:05"))
			}
		}
	}

	// Update subscription in database
	if err := repo.UpdateExternalSubscription(ctx, *sub); err != nil {
		logger.Info("[外部订阅同步] 更新订阅流量信息失败", "name", sub.Name, "error", err)
	} else {
		logger.Info("[外部订阅同步] 更新订阅流量信息成功", "name", sub.Name)
		logger.Info("[外部订阅同步] 上传流量", "bytes", sub.Upload, "mb", float64(sub.Upload)/(1024*1024))
		logger.Info("[外部订阅同步] 下载流量", "bytes", sub.Download, "mb", float64(sub.Download)/(1024*1024))
		logger.Info("[外部订阅同步] 总流量", "bytes", sub.Total, "gb", float64(sub.Total)/(1024*1024*1024))
		logger.Info("[外部订阅同步] 已用流量", "bytes", sub.Upload+sub.Download, "gb", float64(sub.Upload+sub.Download)/(1024*1024*1024))
		if sub.Expire != nil {
			logger.Info("[外部订阅同步] 过期时间", "expire", sub.Expire.Format("2006-01-02 15:04:05"))
		}
	}
}

// SyncExternalSubscriptionsHandler is an HTTP handler for manually triggering external subscription sync
type SyncExternalSubscriptionsHandler struct {
	repo         *storage.TrafficRepository
	subscribeDir string
}

// NewSyncExternalSubscriptionsHandler creates a new handler for manual sync
func NewSyncExternalSubscriptionsHandler(repo *storage.TrafficRepository, subscribeDir string) http.Handler {
	return &SyncExternalSubscriptionsHandler{
		repo:         repo,
		subscribeDir: subscribeDir,
	}
}

// SyncSingleExternalSubscriptionHandler is an HTTP handler for syncing a single external subscription
type SyncSingleExternalSubscriptionHandler struct {
	repo         *storage.TrafficRepository
	subscribeDir string
}

// NewSyncSingleExternalSubscriptionHandler creates a new handler for single subscription sync
func NewSyncSingleExternalSubscriptionHandler(repo *storage.TrafficRepository, subscribeDir string) http.Handler {
	return &SyncSingleExternalSubscriptionHandler{
		repo:         repo,
		subscribeDir: subscribeDir,
	}
}

func (h *SyncSingleExternalSubscriptionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get username from context (set by auth middleware)
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get subscription ID from query parameter
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "缺少订阅ID参数",
		})
		return
	}

	subID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "无效的订阅ID",
		})
		return
	}

	logger.Info("[Sync API] Single subscription sync triggered by user, subscription ID", "user", username, "param", subID)

	// Get user settings
	userSettings, err := h.repo.GetUserSettings(r.Context(), username)
	if err != nil {
		logger.Info("[Sync API] 获取用户设置失败，使用默认设置", "error", err)
		userSettings.MatchRule = "node_name"
		userSettings.SyncScope = "saved_only"
		userSettings.KeepNodeName = true
	}

	// Get the specific subscription
	externalSubs, err := h.repo.ListExternalSubscriptions(r.Context(), username)
	if err != nil {
		logger.Info("[Sync API] Failed to list external subscriptions", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "获取订阅列表失败",
		})
		return
	}

	// Find the subscription by ID
	var targetSub *storage.ExternalSubscription
	for i := range externalSubs {
		if externalSubs[i].ID == subID {
			targetSub = &externalSubs[i]
			break
		}
	}

	if targetSub == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "未找到指定订阅",
		})
		return
	}

	logger.Info("[Sync API] 开始同步单个订阅 (ID)", "name", targetSub.Name, "id", targetSub.ID)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	nodeCount, updatedSub, err := syncSingleExternalSubscription(r.Context(), client, h.repo, h.subscribeDir, username, *targetSub, userSettings)
	if err != nil {
		logger.Info("[Sync API] Failed to sync subscription", "name", targetSub.Name, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("同步失败: %v", err),
		})
		return
	}

	// Update last sync time and node count
	now := time.Now()
	updatedSub.LastSyncAt = &now
	updatedSub.NodeCount = nodeCount
	if err := h.repo.UpdateExternalSubscription(r.Context(), updatedSub); err != nil {
		logger.Info("[Sync API] 更新订阅 的同步时间失败", "name", targetSub.Name, "error", err)
	}

	logger.Info("[Sync API] Successfully synced subscription , synced nodes", "name", targetSub.Name, "param", nodeCount)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"message":    fmt.Sprintf("订阅 %s 同步成功", targetSub.Name),
		"node_count": nodeCount,
	})
}

func (h *SyncExternalSubscriptionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get username from context (set by auth middleware)
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Info("[Sync API] Manual sync triggered by user", "user", username)

	// Use manual sync function which ignores ForceSyncExternal setting
	if err := syncExternalSubscriptionsManual(r.Context(), h.repo, h.subscribeDir, username); err != nil {
		logger.Info("[Sync API] Failed to sync external subscriptions for user", "user", username, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("同步失败: %v", err),
		})
		return
	}

	logger.Info("[Sync API] Successfully synced external subscriptions for user", "user", username)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "外部订阅同步成功",
	})
}

// syncProxyProviderNodesToYAML 将代理集合的节点直接同步到订阅 YAML 文件
// 仅处理 process_mode='mmw' 的代理集合配置
// 这样用户获取订阅时不需要再请求妙妙屋接口，节点直接在 proxies 中
func syncProxyProviderNodesToYAML(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string, sub storage.ExternalSubscription) error {
	if repo == nil || subscribeDir == "" {
		return nil
	}

	// 获取此外部订阅对应的代理集合配置
	configs, err := repo.ListProxyProviderConfigsBySubscription(ctx, sub.ID)
	if err != nil {
		return fmt.Errorf("list proxy provider configs: %w", err)
	}

	// 筛选 process_mode='mmw' 的配置
	var mmwConfigs []storage.ProxyProviderConfig
	for _, cfg := range configs {
		if cfg.ProcessMode == "mmw" {
			mmwConfigs = append(mmwConfigs, cfg)
		}
	}

	if len(mmwConfigs) == 0 {
		logger.Info("[代理集合同步] 外部订阅 没有妙妙屋处理模式的代理集合配置", "name", sub.Name)
		return nil
	}

	logger.Info("[代理集合同步] 外部订阅有妙妙屋处理模式的代理集合配置", "name", sub.Name, "count", len(mmwConfigs))

	// 获取所有订阅文件
	files, err := repo.ListSubscribeFiles(ctx)
	if err != nil {
		return fmt.Errorf("list subscribe files: %w", err)
	}

	// 处理每个代理集合配置
	cache := GetProxyProviderCache()
	for _, config := range mmwConfigs {
		logger.Info("[代理集合同步] 处理代理集合", "name", config.Name)

		var proxiesRaw []any

		// 优先使用缓存
		if entry, ok := cache.Get(config.ID); ok && !cache.IsExpired(entry) {
			logger.Info("[代理集合同步] 使用缓存 ID=, 节点数", "id", config.ID, "node_count", entry.NodeCount)
			proxiesRaw = entry.Nodes
		} else {
			// 缓存未命中或过期，刷新缓存
			entry, err := RefreshProxyProviderCache(&sub, &config)
			if err != nil {
				logger.Info("[代理集合同步] 获取代理集合 的节点失败", "name", config.Name, "error", err)
				continue
			}
			proxiesRaw = entry.Nodes
		}

		if len(proxiesRaw) == 0 {
			logger.Info("[代理集合同步] 代理集合 没有节点", "name", config.Name)
			continue
		}

		logger.Info("[代理集合同步] 代理集合获取到节点", "name", config.Name, "count", len(proxiesRaw))

		// 为节点添加前缀（只使用名称前缀，即第一个 - 之前的部分）
		namePrefix := config.Name
		if idx := strings.Index(config.Name, "-"); idx > 0 {
			namePrefix = config.Name[:idx]
		}
		prefix := fmt.Sprintf("〖%s〗", namePrefix)

		// 复制节点数据并添加前缀（避免污染缓存中的原始数据）
		proxiesCopy := make([]any, len(proxiesRaw))
		nodeNames := make([]string, 0, len(proxiesRaw))
		for i, proxy := range proxiesRaw {
			if proxyMap, ok := proxy.(map[string]any); ok {
				nodeCopy := copyMapForSync(proxyMap)
				if originalName, ok := nodeCopy["name"].(string); ok {
					newName := prefix + originalName
					nodeCopy["name"] = newName
					nodeNames = append(nodeNames, newName)
				}
				proxiesCopy[i] = nodeCopy
			}
		}
		proxiesRaw = proxiesCopy

		// 更新每个订阅文件
		for _, file := range files {
			if err := updateYAMLFileWithProxyProviderNodes(subscribeDir, file.Filename, config.Name, prefix, proxiesRaw, nodeNames); err != nil {
				logger.Info("[代理集合同步] 更新文件 失败", "filename", file.Filename, "error", err)
				continue
			}
		}
	}

	return nil
}

// updateYAMLFileWithProxyProviderNodes 更新单个 YAML 文件，将代理集合节点添加到 proxies 和 proxy-groups
// 使用 yaml.Node 保持字段顺序，使用 RemoveUnicodeEscapeQuotes 处理 emoji 编码
func updateYAMLFileWithProxyProviderNodes(subscribeDir, filename, providerName, prefix string, proxies []any, nodeNames []string) error {
	filePath := fmt.Sprintf("%s/%s", subscribeDir, filename)

	// 读取 YAML 文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// 使用 yaml.Node 解析以保持字段顺序
	var rootNode yaml.Node
	if err := yaml.Unmarshal(content, &rootNode); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}

	// 获取文档节点
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return nil
	}
	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return nil
	}

	modified := false

	// 查找 proxy-groups 节点
	var proxyGroupsNode *yaml.Node
	var proxiesNode *yaml.Node
	var proxyProvidersNode *yaml.Node
	var proxyProvidersKeyIndex int = -1

	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode {
			switch keyNode.Value {
			case "proxy-groups":
				proxyGroupsNode = valueNode
			case "proxies":
				proxiesNode = valueNode
			case "proxy-providers":
				proxyProvidersNode = valueNode
				proxyProvidersKeyIndex = i
			}
		}
	}

	if proxyGroupsNode == nil || proxyGroupsNode.Kind != yaml.SequenceNode {
		return nil // 没有 proxy-groups，跳过
	}

	// 遍历 proxy-groups，检查是否使用了此代理集合
	// 记录是否需要创建新代理组
	needCreateNewGroup := false

	for _, groupNode := range proxyGroupsNode.Content {
		if groupNode.Kind != yaml.MappingNode {
			continue
		}

		// 查找 use 和 proxies 字段
		var useNode *yaml.Node
		var useKeyIndex int = -1
		var groupProxiesNode *yaml.Node
		var groupName string

		for i := 0; i < len(groupNode.Content)-1; i += 2 {
			keyNode := groupNode.Content[i]
			valueNode := groupNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode {
				switch keyNode.Value {
				case "use":
					useNode = valueNode
					useKeyIndex = i
				case "proxies":
					groupProxiesNode = valueNode
				case "name":
					if valueNode.Kind == yaml.ScalarNode {
						groupName = valueNode.Value
					}
				}
			}
		}

		if useNode == nil || useNode.Kind != yaml.SequenceNode {
			continue
		}

		// 检查是否包含此代理集合
		foundProvider := false
		newUseContent := make([]*yaml.Node, 0)
		for _, useItem := range useNode.Content {
			if useItem.Kind == yaml.ScalarNode && useItem.Value == providerName {
				foundProvider = true
			} else {
				newUseContent = append(newUseContent, useItem)
			}
		}

		if !foundProvider {
			continue
		}

		modified = true
		needCreateNewGroup = true
		logger.Info("[代理集合同步] 在文件 的代理组 中找到代理集合 的引用", "value", filename, "param", groupName, "arg", providerName)

		// 确保 proxies 节点存在
		if groupProxiesNode == nil {
			groupProxiesNode = &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0)}
			groupNode.Content = append(groupNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "proxies"},
				groupProxiesNode,
			)
		}

		// 移除此代理集合的旧节点（以 prefix 开头的）和旧的代理组名称
		newProxiesContent := make([]*yaml.Node, 0)
		for _, p := range groupProxiesNode.Content {
			if p.Kind == yaml.ScalarNode {
				// 移除以 prefix 开头的节点名称
				if strings.HasPrefix(p.Value, prefix) {
					continue
				}
				// 移除同名的旧代理组（如果存在）
				if p.Value == providerName {
					continue
				}
			}
			newProxiesContent = append(newProxiesContent, p)
		}

		// 只添加新代理组名称到原代理组（而不是所有节点名称）
		newProxiesContent = append(newProxiesContent, &yaml.Node{Kind: yaml.ScalarNode, Value: providerName})
		groupProxiesNode.Content = newProxiesContent

		// 更新 use 字段（移除此代理集合）
		if len(newUseContent) == 0 && useKeyIndex >= 0 {
			// 删除 use 字段
			groupNode.Content = append(groupNode.Content[:useKeyIndex], groupNode.Content[useKeyIndex+2:]...)
		} else {
			useNode.Content = newUseContent
		}

		logger.Info("[代理集合同步] 代理组 更新完成: 添加了代理组 的引用", "value", groupName, "param", providerName)
	}

	// 妙妙屋模式：检查是否存在与代理集合同名的 proxy-group
	// 如果存在，直接更新它（不需要 use 字段）
	if !needCreateNewGroup {
		for _, groupNode := range proxyGroupsNode.Content {
			if groupNode.Kind != yaml.MappingNode {
				continue
			}
			name := util.GetNodeFieldValue(groupNode, "name")
			if name == providerName {
				// 找到同名的 proxy-group，这是妙妙屋模式
				needCreateNewGroup = true
				modified = true
				logger.Info("[代理集合同步] 妙妙屋模式：找到同名代理组", "name", providerName)
				break
			}
		}
	}

	// 用于存储当前代理组中的旧节点名称（处理节点减少时删除顶层 proxies 中的旧配置）
	var oldNodeNamesInGroup map[string]bool

	// 创建或更新以代理集合名称命名的新代理组
	if needCreateNewGroup {
		// 检查是否已存在同名代理组
		existingGroupNode := (*yaml.Node)(nil)
		for _, groupNode := range proxyGroupsNode.Content {
			if groupNode.Kind == yaml.MappingNode {
				name := util.GetNodeFieldValue(groupNode, "name")
				if name == providerName {
					existingGroupNode = groupNode
					break
				}
			}
		}

		if existingGroupNode != nil {
			// 更新已存在的代理组的 proxies
			var existingProxiesNode *yaml.Node
			for i := 0; i < len(existingGroupNode.Content)-1; i += 2 {
				keyNode := existingGroupNode.Content[i]
				valueNode := existingGroupNode.Content[i+1]
				if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "proxies" {
					existingProxiesNode = valueNode
					break
				}
			}

			// 先收集旧节点名称，用于后续删除顶层 proxies 中的旧配置
			oldNodeNamesInGroup = make(map[string]bool)
			if existingProxiesNode != nil && existingProxiesNode.Kind == yaml.SequenceNode {
				for _, p := range existingProxiesNode.Content {
					if p.Kind == yaml.ScalarNode && strings.HasPrefix(p.Value, prefix) {
						oldNodeNamesInGroup[p.Value] = true
					}
				}
			}
			oldCount := len(oldNodeNamesInGroup)

			if existingProxiesNode == nil {
				existingProxiesNode = &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0)}
				existingGroupNode.Content = append(existingGroupNode.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "proxies"},
					existingProxiesNode,
				)
			}

			// 移除旧节点（以 prefix 开头的），添加新节点
			newContent := make([]*yaml.Node, 0)
			for _, p := range existingProxiesNode.Content {
				if p.Kind == yaml.ScalarNode && strings.HasPrefix(p.Value, prefix) {
					continue
				}
				newContent = append(newContent, p)
			}
			for _, nodeName := range nodeNames {
				newContent = append(newContent, &yaml.Node{Kind: yaml.ScalarNode, Value: nodeName})
			}
			existingProxiesNode.Content = newContent
			logger.Info("[代理集合同步] 更新已存在的代理组", "name", providerName, "old_count", oldCount, "new_count", len(nodeNames))
		} else {
			// 创建新代理组（类型为 url-test）
			newGroupNode := &yaml.Node{Kind: yaml.MappingNode}
			newGroupNode.Content = append(newGroupNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "name"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: providerName},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "type"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "url-test"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "url"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "http://www.gstatic.com/generate_204"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "interval"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "300"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "tolerance"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "50"},
			)

			// 添加 proxies 字段，包含所有节点名称
			newGroupProxies := &yaml.Node{Kind: yaml.SequenceNode}
			for _, nodeName := range nodeNames {
				newGroupProxies.Content = append(newGroupProxies.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: nodeName})
			}
			newGroupNode.Content = append(newGroupNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "proxies"},
				newGroupProxies,
			)

			// 添加新代理组到 proxy-groups
			proxyGroupsNode.Content = append(proxyGroupsNode.Content, newGroupNode)
			logger.Info("[代理集合同步] 创建新代理组", "name", providerName, "node_count", len(nodeNames))
		}
	}

	if !modified {
		return nil // 没有修改，不需要保存
	}

	// 确保 proxies 节点存在
	if proxiesNode == nil {
		proxiesNode = &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0)}
		// 在文档开头添加 proxies
		docContent.Content = append([]*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "proxies"},
			proxiesNode,
		}, docContent.Content...)
	}

	// 构建新节点名称集合，用于精确匹配删除
	newNodeNameSet := make(map[string]bool)
	for _, name := range nodeNames {
		newNodeNameSet[name] = true
	}

	// 移除属于当前代理集合的旧节点配置
	// 使用代理组中收集的旧节点名称列表
	// 同时也删除新节点名称，以便后面重新添加最新配置
	newProxiesContent := make([]*yaml.Node, 0)
	for _, p := range proxiesNode.Content {
		if p.Kind == yaml.MappingNode {
			name := util.GetNodeFieldValue(p, "name")
			// 如果节点名称在旧节点列表中，则删除（处理节点减少的情况）
			if oldNodeNamesInGroup != nil && oldNodeNamesInGroup[name] {
				continue
			}
			// 如果节点名称在新节点列表中，也删除（后面会重新添加最新配置）
			if newNodeNameSet[name] {
				continue
			}
		}
		newProxiesContent = append(newProxiesContent, p)
	}

	// 添加新节点（使用 util.ReorderProxyFieldsToNode 保持字段顺序）
	for _, proxy := range proxies {
		if proxyMap, ok := proxy.(map[string]any); ok {
			proxyNode := util.ReorderProxyFieldsToNode(proxyMap)
			newProxiesContent = append(newProxiesContent, proxyNode)
		}
	}
	proxiesNode.Content = newProxiesContent

	// 清理 proxy-providers（如果不再被使用）
	if proxyProvidersNode != nil && proxyProvidersNode.Kind == yaml.MappingNode && proxyProvidersKeyIndex >= 0 {
		// 查找并删除对应的 provider
		newProvidersContent := make([]*yaml.Node, 0)
		for i := 0; i < len(proxyProvidersNode.Content)-1; i += 2 {
			keyNode := proxyProvidersNode.Content[i]
			valueNode := proxyProvidersNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == providerName {
				continue // 跳过此 provider
			}
			newProvidersContent = append(newProvidersContent, keyNode, valueNode)
		}

		if len(newProvidersContent) == 0 {
			// 删除整个 proxy-providers
			docContent.Content = append(docContent.Content[:proxyProvidersKeyIndex], docContent.Content[proxyProvidersKeyIndex+2:]...)
		} else {
			proxyProvidersNode.Content = newProvidersContent
		}
	}

	// 编码 YAML，使用 2 空格缩进
	// Sanitize explicit string tags before encoding to prevent !!str from appearing in output
	sanitizeExplicitStringTags(&rootNode)

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&rootNode); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	encoder.Close()

	// 处理 unicode 转义和数字引号
	result := RemoveUnicodeEscapeQuotes(buf.String())

	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	logger.Info("[代理集合同步] 文件 更新完成", "value", filename)
	return nil
}

// copyMapForSync 深拷贝 map（用于代理节点同步，避免污染缓存）
func copyMapForSync(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		switch vv := v.(type) {
		case map[string]any:
			result[k] = copyMapForSync(vv)
		case []any:
			copied := make([]any, len(vv))
			for i, item := range vv {
				if itemMap, ok := item.(map[string]any); ok {
					copied[i] = copyMapForSync(itemMap)
				} else {
					copied[i] = item
				}
			}
			result[k] = copied
		default:
			result[k] = v
		}
	}
	return result
}
