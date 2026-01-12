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
	logger.Info("[代理集合缓存] 更新缓存", "id", configID, "node_count", entry.NodeCount)
}

// UpdateInterval 更新缓存条目的interval字段
// 当配置更新时，需要同步更新缓存中的interval，以确保过期判断使用最新值
func (c *ProxyProviderCache) UpdateInterval(configID int64, interval int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[configID]; ok {
		entry.Interval = interval
	}
}

// Delete 删除缓存条目
func (c *ProxyProviderCache) Delete(configID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, configID)
	logger.Info("[代理集合缓存] 删除缓存", "id", configID)
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
		logger.Info("[代理集合缓存] 启动时获取用户列表失败", "error", err)
		return
	}

	totalConfigs := 0
	successCount := 0

	for _, user := range users {
		// 获取用户的代理集合配置
		configs, err := repo.ListProxyProviderConfigs(ctx, user.Username)
		if err != nil {
			logger.Info("[代理集合缓存] 获取用户的代理集合配置失败", "username", user.Username, "error", err)
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
				logger.Info("[代理集合缓存] 获取代理集合的外部订阅失败", "config_name", config.Name, "error", err)
				continue
			}

			// 刷新缓存
			_, err = RefreshProxyProviderCache(&sub, &config)
			if err != nil {
				logger.Info("[代理集合缓存] 刷新代理集合缓存失败", "config_name", config.Name, "error", err)
				continue
			}

			successCount++
		}
	}

	logger.Info("[代理集合缓存] 启动初始化完成", "total_configs", totalConfigs, "success_count", successCount)
}

// 定时同步器相关常量
const (
	// 扫描周期：每15秒检查一次是否有代理集合需要刷新
	proxyProviderScanInterval = 15 * time.Second
	// 配置重载周期：每5分钟重新加载一次所有MMW模式的代理集合配置
	// 注意：配置重载会执行全表扫描，频率过高会影响前台API性能（SQLite单连接限制）
	proxyProviderReloadInterval = 5 * time.Minute
	// 重试延迟基础值：首次失败后等待30秒重试
	proxyProviderRetryBase = 30 * time.Second
	// 重试延迟最大值：最多等待10分钟后重试
	proxyProviderRetryMax = 10 * time.Minute
	// 并发刷新worker数量：最多同时刷新4个代理集合
	proxyProviderWorkerLimit = 4
	// 默认刷新间隔：如果配置未指定interval或无效，默认1小时
	defaultProxyInterval = 3600
	// 节点日志预览大小：日志中最多显示前5个节点名称
	nodeLogPreviewSize = 5
	// 刷新操作超时时间：每次刷新操作最多1分钟
	refreshOperationTimeout = time.Minute
	// 配置加载超时时间：重新加载配置最多30秒
	configLoadTimeout = 30 * time.Second
)

// proxyProviderSyncState 记录单个代理集合的同步状态
type proxyProviderSyncState struct {
	// blockUntil 阻塞直到此时间，用于实现指数退避重试
	blockUntil time.Time
	// retryDelay 当前重试延迟，失败后会翻倍
	retryDelay time.Duration
}

// scheduledProxyConfig 待刷新的代理集合配置及原因
type scheduledProxyConfig struct {
	// cfg 代理集合配置
	cfg storage.ProxyProviderConfig
	// reason 需要刷新的原因（用于日志）
	reason string
}

// dbSubscriptionCacheEntry 订阅元数据缓存条目（用于同一扫描周期内共享订阅数据库记录）
// 与 proxy_provider_serve.go 中的 subscriptionCacheEntry 不同，这里缓存的是数据库记录，而非订阅内容
type dbSubscriptionCacheEntry struct {
	sub       storage.ExternalSubscription
	fetchedAt time.Time
}

// proxyProviderCacheSyncer 代理集合缓存同步器
// 负责定时检查所有MMW模式的代理集合，根据interval自动刷新过期的缓存
type proxyProviderCacheSyncer struct {
	repo    *storage.TrafficRepository // 数据库仓库
	cache   *ProxyProviderCache         // 缓存管理器
	mu      sync.Mutex                  // 保护以下字段的互斥锁
	configs map[int64]storage.ProxyProviderConfig // 当前所有MMW配置 (key: config ID)
	state   map[int64]*proxyProviderSyncState     // 每个配置的同步状态 (key: config ID)
	running map[int64]struct{}                    // 正在刷新的配置ID集合
	workers chan struct{}                         // worker限流通道
	wg      sync.WaitGroup                        // 等待所有worker完成

	// 订阅元数据缓存：在同一个扫描周期内共享订阅数据库记录，避免重复查询数据库
	subCacheMu sync.RWMutex                            // 订阅缓存锁
	subCache   map[int64]*dbSubscriptionCacheEntry     // 订阅缓存 (key: external_subscription_id)
}

// StartProxyProviderCacheSync 启动代理集合缓存定时同步
// 该函数会阻塞，直到context被取消
func StartProxyProviderCacheSync(ctx context.Context, repo *storage.TrafficRepository) {
	if repo == nil {
		return
	}

	syncer := newProxyProviderCacheSyncer(repo)
	syncer.run(ctx)
}

// newProxyProviderCacheSyncer 创建新的同步器实例
func newProxyProviderCacheSyncer(repo *storage.TrafficRepository) *proxyProviderCacheSyncer {
	return &proxyProviderCacheSyncer{
		repo:     repo,
		cache:    GetProxyProviderCache(),
		configs:  make(map[int64]storage.ProxyProviderConfig),
		state:    make(map[int64]*proxyProviderSyncState),
		running:  make(map[int64]struct{}),
		workers:  make(chan struct{}, proxyProviderWorkerLimit),
		subCache: make(map[int64]*dbSubscriptionCacheEntry),
	}
}

// run 运行同步器主循环
func (s *proxyProviderCacheSyncer) run(ctx context.Context) {
	logger.Info("[代理集合定时同步] 调度器启动",
		"scan_interval", proxyProviderScanInterval.String(),
		"reload_interval", proxyProviderReloadInterval.String(),
		"max_workers", proxyProviderWorkerLimit)
	defer logger.Info("[代理集合定时同步] 调度器已退出")

	// 启动时立即加载配置
	s.reloadConfigs(ctx)

	// 创建定时器
	scanTicker := time.NewTicker(proxyProviderScanInterval)
	reloadTicker := time.NewTicker(proxyProviderReloadInterval)
	defer scanTicker.Stop()
	defer reloadTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("[代理集合定时同步] 收到停止信号，等待所有刷新任务完成")
			s.wg.Wait()
			return
		case <-reloadTicker.C:
			// 定期重新加载配置，以获取最新的代理集合列表和interval设置
			s.reloadConfigs(ctx)
		case <-scanTicker.C:
			// 定期扫描，检查是否有需要刷新的代理集合
			s.runSyncCycle(ctx)
		}
	}
}

// runSyncCycle 执行一次同步周期
// 检查所有配置，对需要刷新的代理集合启动worker
func (s *proxyProviderCacheSyncer) runSyncCycle(ctx context.Context) {
	// 清空上一个周期的订阅缓存
	s.clearSubscriptionCache()

	jobs := s.collectDueConfigs()
	if len(jobs) == 0 {
		return
	}

	// 启动所有任务
	for _, job := range jobs {
		s.launchWorker(ctx, job)
	}

	// 等待本周期所有任务完成后，记录缓存统计
	// 注意：不需要等待完成，下个周期开始时会清空缓存
}

// collectDueConfigs 收集所有需要刷新的代理集合配置
func (s *proxyProviderCacheSyncer) collectDueConfigs() []scheduledProxyConfig {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	var jobs []scheduledProxyConfig
	for id, cfg := range s.configs {
		// 如果该配置已经在刷新中，跳过
		if _, busy := s.running[id]; busy {
			continue
		}

		// 获取或创建状态
		state := s.ensureStateLocked(id)

		// 如果还在重试阻塞期，跳过
		if !state.blockUntil.IsZero() && now.Before(state.blockUntil) {
			continue
		}

		// 检查是否需要刷新
		if job, ok := s.shouldRefreshLocked(cfg, now); ok {
			s.running[id] = struct{}{}
			jobs = append(jobs, job)
		}
	}

	if len(jobs) > 0 {
		logger.Info("[代理集合定时同步] 本次扫描发现需要刷新的配置", "count", len(jobs))
	}

	return jobs
}

// shouldRefreshLocked 判断是否需要刷新（必须在锁保护下调用）
func (s *proxyProviderCacheSyncer) shouldRefreshLocked(cfg storage.ProxyProviderConfig, now time.Time) (scheduledProxyConfig, bool) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultProxyInterval
	}

	// 检查缓存是否存在
	entry, ok := s.cache.Get(cfg.ID)
	if !ok {
		return scheduledProxyConfig{
			cfg:    cfg,
			reason: "缓存不存在，立即同步",
		}, true
	}

	// 计算缓存过期时间
	expireAt := entry.FetchedAt.Add(time.Duration(interval) * time.Second)
	if now.Before(expireAt) {
		// 未过期，不需要刷新
		return scheduledProxyConfig{}, false
	}

	// 已过期，需要刷新
	return scheduledProxyConfig{
		cfg:    cfg,
		reason: "缓存已过期，按interval刷新",
	}, true
}

// launchWorker 启动一个worker来刷新代理集合
func (s *proxyProviderCacheSyncer) launchWorker(ctx context.Context, job scheduledProxyConfig) {
	// 尝试获取worker槽位，如果上下文已取消则直接返回
	select {
	case <-ctx.Done():
		s.markTaskFinished(job.cfg.ID)
		return
	case s.workers <- struct{}{}:
		// 获取到槽位，继续执行
	}

	s.wg.Add(1)
	go func() {
		defer func() {
			<-s.workers              // 释放worker槽位
			s.wg.Done()              // 通知WaitGroup
			s.markTaskFinished(job.cfg.ID) // 标记任务完成
		}()
		s.refreshSingle(ctx, job)
	}()
}

// refreshSingle 刷新单个代理集合
func (s *proxyProviderCacheSyncer) refreshSingle(ctx context.Context, job scheduledProxyConfig) {
	cfg := job.cfg

	logger.Info("[代理集合定时同步] 开始刷新节点",
		"config_id", cfg.ID,
		"name", cfg.Name,
		"username", cfg.Username,
		"reason", job.reason,
		"interval_seconds", cfg.Interval)

	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		logger.Info("[代理集合定时同步] 上下文已结束，跳过刷新", "config_id", cfg.ID)
		return
	}

	// 创建带超时的上下文
	runCtx, cancel := context.WithTimeout(ctx, refreshOperationTimeout)
	defer cancel()

	// 先从缓存获取订阅信息（同一个扫描周期内共享）
	sub, fromCache := s.getOrFetchSubscription(runCtx, cfg.ExternalSubscriptionID, cfg.Username)
	if sub == nil {
		logger.Warn("[代理集合定时同步] 获取外部订阅失败",
			"config_id", cfg.ID,
			"subscription_id", cfg.ExternalSubscriptionID)
		s.recordFailure(cfg.ID)
		return
	}

	if fromCache {
		logger.Info("[代理集合定时同步] 使用缓存的订阅数据",
			"config_id", cfg.ID,
			"subscription_id", cfg.ExternalSubscriptionID)
	}

	// 刷新缓存
	entry, err := RefreshProxyProviderCache(sub, &cfg)
	if err != nil {
		logger.Warn("[代理集合定时同步] 刷新缓存失败",
			"config_id", cfg.ID,
			"name", cfg.Name,
			"error", err)
		s.recordFailure(cfg.ID)
		return
	}

	// 刷新成功，记录日志
	nodePreview := makeNodePreview(entry.NodeNames, nodeLogPreviewSize)
	logger.Info("[代理集合定时同步] 刷新成功",
		"config_id", cfg.ID,
		"name", cfg.Name,
		"node_count", entry.NodeCount,
		"node_preview", nodePreview,
		"from_subscription_cache", fromCache)
	s.recordSuccess(cfg.ID)
}

// recordFailure 记录刷新失败，更新重试延迟（指数退避）
func (s *proxyProviderCacheSyncer) recordFailure(configID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensureStateLocked(configID)

	// 计算新的重试延迟（指数退避）
	delay := state.retryDelay
	if delay == 0 {
		delay = proxyProviderRetryBase
	} else {
		delay *= 2
		if delay > proxyProviderRetryMax {
			delay = proxyProviderRetryMax
		}
	}

	state.retryDelay = delay
	state.blockUntil = time.Now().Add(delay)

	logger.Info("[代理集合定时同步] 同步失败，已安排重试",
		"config_id", configID,
		"retry_after", delay.String())
}

// recordSuccess 记录刷新成功，重置重试延迟
func (s *proxyProviderCacheSyncer) recordSuccess(configID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ensureStateLocked(configID)
	state.retryDelay = 0
	state.blockUntil = time.Time{}
}

// markTaskFinished 标记任务完成，从running集合中移除
func (s *proxyProviderCacheSyncer) markTaskFinished(configID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, configID)
}

// reloadConfigs 重新加载所有MMW模式的代理集合配置
func (s *proxyProviderCacheSyncer) reloadConfigs(ctx context.Context) {
	loadCtx, cancel := context.WithTimeout(ctx, configLoadTimeout)
	defer cancel()

	configs, err := s.repo.ListMMWProxyProviderConfigs(loadCtx)
	if err != nil {
		logger.Warn("[代理集合定时同步] 重新加载代理集合配置失败", "error", err)
		return
	}

	logger.Info("[代理集合定时同步] 已加载代理集合配置", "count", len(configs))

	// 构建新的配置映射，并同步更新缓存中的interval
	nextConfigs := make(map[int64]storage.ProxyProviderConfig, len(configs))
	for _, cfg := range configs {
		nextConfigs[cfg.ID] = cfg
		// 同步更新缓存中的interval
		s.cache.UpdateInterval(cfg.ID, cfg.Interval)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新配置
	s.configs = nextConfigs

	// 清理已删除配置的状态和缓存
	for id := range s.state {
		if _, ok := nextConfigs[id]; !ok {
			delete(s.state, id)
			s.cache.Delete(id) // 清理缓存
		}
	}

	// 清理已删除配置的running标记
	for id := range s.running {
		if _, ok := nextConfigs[id]; !ok {
			delete(s.running, id)
		}
	}
}

// ensureStateLocked 获取或创建状态（必须在锁保护下调用）
func (s *proxyProviderCacheSyncer) ensureStateLocked(id int64) *proxyProviderSyncState {
	if st, ok := s.state[id]; ok {
		return st
	}
	st := &proxyProviderSyncState{}
	s.state[id] = st
	return st
}

// getOrFetchSubscription 获取或拉取订阅信息（带缓存）
// 返回订阅信息和是否来自缓存的标志
func (s *proxyProviderCacheSyncer) getOrFetchSubscription(ctx context.Context, subscriptionID int64, username string) (*storage.ExternalSubscription, bool) {
	// 先尝试从缓存读取
	s.subCacheMu.RLock()
	if entry, ok := s.subCache[subscriptionID]; ok {
		s.subCacheMu.RUnlock()
		return &entry.sub, true
	}
	s.subCacheMu.RUnlock()

	// 缓存未命中，从数据库获取
	sub, err := s.repo.GetExternalSubscription(ctx, subscriptionID, username)
	if err != nil {
		return nil, false
	}
	if sub.ID == 0 {
		return nil, false
	}

	// 存入缓存
	s.subCacheMu.Lock()
	s.subCache[subscriptionID] = &dbSubscriptionCacheEntry{
		sub:       sub,
		fetchedAt: time.Now(),
	}
	s.subCacheMu.Unlock()

	return &sub, false
}

// clearSubscriptionCache 清空订阅缓存（每个扫描周期开始时调用）
func (s *proxyProviderCacheSyncer) clearSubscriptionCache() {
	s.subCacheMu.Lock()
	defer s.subCacheMu.Unlock()

	// 记录缓存统计
	if len(s.subCache) > 0 {
		logger.Info("[代理集合定时同步] 清空订阅缓存", "cached_subscriptions", len(s.subCache))
		s.subCache = make(map[int64]*dbSubscriptionCacheEntry)
	}
}

// makeNodePreview 生成节点名称预览（用于日志）
func makeNodePreview(nodes []string, limit int) []string {
	if len(nodes) <= limit {
		return append([]string{}, nodes...)
	}
	preview := append([]string{}, nodes[:limit]...)
	return append(preview, "...")
}
