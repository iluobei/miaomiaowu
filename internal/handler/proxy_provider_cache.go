package handler

import (
	"context"
	"miaomiaowu/internal/logger"
	"sync"
	"time"

	"miaomiaowu/internal/storage"
)

// CacheEntry 代理集合缓存条目
type CacheEntry struct {
	ConfigID   int64            // 配置 ID
	YAMLData   []byte           // 缓存的 YAML 节点数据
	Nodes      []any            // 解析后的节点列表 ([]map[string]any)
	NodeNames  []string         // 节点名称列表（带前缀）
	Prefix     string           // 节点名称前缀
	FetchedAt  time.Time        // 拉取时间
	Interval   int              // 配置的缓存间隔（秒）
	NodeCount  int              // 节点数量
}

// ProxyProviderCache 代理集合内存缓存
type ProxyProviderCache struct {
	mu      sync.RWMutex
	entries map[int64]*CacheEntry // key: config ID
}

// 全局缓存实例
var proxyProviderCache = &ProxyProviderCache{
	entries: make(map[int64]*CacheEntry),
}

// GetProxyProviderCache 获取全局缓存实例
func GetProxyProviderCache() *ProxyProviderCache {
	return proxyProviderCache
}

// Get 获取缓存条目
func (c *ProxyProviderCache) Get(configID int64) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[configID]
	return entry, ok
}

// Set 设置缓存条目
func (c *ProxyProviderCache) Set(configID int64, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[configID] = entry
	logger.Info("[代理集合缓存] 更新缓存 ID=%d, 节点数=%d", configID, entry.NodeCount)
}

// Delete 删除缓存条目
func (c *ProxyProviderCache) Delete(configID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, configID)
	logger.Info("[代理集合缓存] 删除缓存 ID=%d", configID)
}

// IsExpired 检查缓存是否过期
func (c *ProxyProviderCache) IsExpired(entry *CacheEntry) bool {
	if entry == nil {
		return true
	}
	interval := entry.Interval
	if interval <= 0 {
		interval = 3600 // 默认 1 小时
	}
	return time.Since(entry.FetchedAt) > time.Duration(interval)*time.Second
}

// GetCacheStatus 获取缓存状态（用于 API 返回）
func (c *ProxyProviderCache) GetCacheStatus(configID int64) map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[configID]
	if !ok {
		return map[string]any{
			"cached":    false,
			"expired":   true,
			"node_count": 0,
		}
	}

	return map[string]any{
		"cached":     true,
		"expired":    c.IsExpired(entry),
		"node_count": entry.NodeCount,
		"fetched_at": entry.FetchedAt.Format(time.RFC3339),
		"interval":   entry.Interval,
	}
}

// GetAllCacheStatus 获取所有缓存状态
func (c *ProxyProviderCache) GetAllCacheStatus() map[int64]map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[int64]map[string]any)
	for id, entry := range c.entries {
		result[id] = map[string]any{
			"cached":     true,
			"expired":    c.IsExpired(entry),
			"node_count": entry.NodeCount,
			"fetched_at": entry.FetchedAt.Format(time.RFC3339),
			"interval":   entry.Interval,
		}
	}
	return result
}

// Clear 清空所有缓存
func (c *ProxyProviderCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[int64]*CacheEntry)
	logger.Info("[代理集合缓存] 清空所有缓存")
}

// InitProxyProviderCacheOnStartup 服务启动时初始化所有 MMW 模式代理集合的缓存
func InitProxyProviderCacheOnStartup(repo *storage.TrafficRepository) {
	if repo == nil {
		return
	}

	ctx := context.Background()

	// 获取所有用户
	users, err := repo.ListUsers(ctx, 0) // 0 表示不限制数量
	if err != nil {
		logger.Info("[代理集合缓存] 启动时获取用户列表失败: %v", err)
		return
	}

	totalConfigs := 0
	successCount := 0

	for _, user := range users {
		// 获取用户的代理集合配置
		configs, err := repo.ListProxyProviderConfigs(ctx, user.Username)
		if err != nil {
			logger.Info("[代理集合缓存] 获取用户 %s 的代理集合配置失败: %v", user.Username, err)
			continue
		}

		for _, config := range configs {
			if config.ProcessMode != "mmw" {
				continue
			}

			totalConfigs++

			// 获取外部订阅信息
			sub, err := repo.GetExternalSubscription(ctx, config.ExternalSubscriptionID, user.Username)
			if err != nil || sub.ID == 0 {
				logger.Info("[代理集合缓存] 获取代理集合 %s 的外部订阅失败: %v", config.Name, err)
				continue
			}

			// 刷新缓存
			_, err = RefreshProxyProviderCache(&sub, &config)
			if err != nil {
				logger.Info("[代理集合缓存] 刷新代理集合 %s 缓存失败: %v", config.Name, err)
				continue
			}

			successCount++
		}
	}

	logger.Info("[代理集合缓存] 启动初始化完成", "total_configs", totalConfigs, "success_count", successCount)
}
