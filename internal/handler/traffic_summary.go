package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"miaomiaowu/internal/logger"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

const bytesPerGigabyte = 1073741824.0

type TrafficSummaryHandler struct {
	client *http.Client
	repo   *storage.TrafficRepository
}

type trafficSummaryResponse struct {
	Metrics trafficSummaryMetrics `json:"metrics"`
	History []trafficDailyUsage   `json:"history"`
}

type trafficSummaryMetrics struct {
	TotalLimitGB     float64 `json:"total_limit_gb"`
	TotalUsedGB      float64 `json:"total_used_gb"`
	TotalRemainingGB float64 `json:"total_remaining_gb"`
	UsagePercentage  float64 `json:"usage_percentage"`
}

type trafficDailyUsage struct {
	Date   string  `json:"date"`
	UsedGB float64 `json:"used_gb"`
}

type batchTrafficResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    map[string]struct {
		Monthly struct {
			Limit     json.Number `json:"limit"`
			Remaining json.Number `json:"remaining"`
			Used      json.Number `json:"used"`
		} `json:"monthly"`
	} `json:"data"`
}

func NewTrafficSummaryHandler(repo *storage.TrafficRepository) *TrafficSummaryHandler {
	if repo == nil {
		panic("traffic summary handler requires repository")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	return newTrafficSummaryHandler(client, repo)
}

func newTrafficSummaryHandler(client *http.Client, repo *storage.TrafficRepository) *TrafficSummaryHandler {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	return &TrafficSummaryHandler{client: client, repo: repo}
}

func (h *TrafficSummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
		return
	}

	ctx := r.Context()
	username := auth.UsernameFromContext(ctx)

	var totalLimit, totalRemaining, totalUsed int64
	var probeErr error

	totalLimit, totalRemaining, totalUsed, probeErr = h.fetchTotals(ctx, username, nil)
	if probeErr != nil {
		// Log the error but continue to try external subscription traffic
		if errors.Is(probeErr, storage.ErrProbeConfigNotFound) {
			logger.Info("[Traffic] Probe not configured, will use external subscription traffic only")
		} else {
			logger.Info("[流量] 获取探针流量失败", "error", probeErr)
		}
		// Reset values in case of error
		totalLimit, totalRemaining, totalUsed = 0, 0, 0
	}

	// Add external subscription traffic if sync_traffic is enabled
	if username != "" {
		externalLimit, externalUsed := h.fetchExternalSubscriptionTraffic(ctx, username)
		totalLimit += externalLimit
		totalUsed += externalUsed
		// Recalculate remaining
		totalRemaining = totalLimit - totalUsed
	}

	// If no traffic data from either source, return appropriate response
	if totalLimit == 0 && totalUsed == 0 && probeErr != nil && !errors.Is(probeErr, storage.ErrProbeConfigNotFound) {
		// Only return error if probe failed (not just not configured) and no external traffic
		writeError(w, http.StatusBadGateway, probeErr)
		return
	}

	if err := h.recordSnapshot(ctx, totalLimit, totalUsed, totalRemaining); err != nil {
		logger.Info("[流量] 记录快照失败", "error", err)
	}

	history, err := h.loadHistory(ctx, 30)
	if err != nil {
		logger.Info("[流量] 加载历史记录失败", "error", err)
	}

	metrics := trafficSummaryMetrics{
		TotalLimitGB:     roundUpTwoDecimals(bytesToGigabytes(totalLimit)),
		TotalUsedGB:      roundUpTwoDecimals(bytesToGigabytes(totalUsed)),
		TotalRemainingGB: roundUpTwoDecimals(bytesToGigabytes(totalRemaining)),
		UsagePercentage:  roundUpTwoDecimals(usagePercentage(totalUsed, totalLimit)),
	}

	response := trafficSummaryResponse{
		Metrics: metrics,
		History: history,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// RecordDailyUsage fetches the latest traffic summary and persists the snapshot.
func (h *TrafficSummaryHandler) RecordDailyUsage(ctx context.Context) error {
	var totalLimit, totalRemaining, totalUsed int64
	var probeErr error

	totalLimit, totalRemaining, totalUsed, probeErr = h.fetchTotals(ctx, "", nil)
	if probeErr != nil {
		if errors.Is(probeErr, storage.ErrProbeConfigNotFound) {
			logger.Info("[流量记录] 探针未配置，仅使用外部订阅流量")
		} else {
			logger.Warn("[流量记录] 获取探针流量失败", "error", probeErr)
		}
		totalLimit, totalRemaining, totalUsed = 0, 0, 0
	} else {
		// Log fetched probe data
		limitGB := roundUpTwoDecimals(bytesToGigabytes(totalLimit))
		usedGB := roundUpTwoDecimals(bytesToGigabytes(totalUsed))
		remainingGB := roundUpTwoDecimals(bytesToGigabytes(totalRemaining))
		usagePercent := roundUpTwoDecimals(usagePercentage(totalUsed, totalLimit))

		logger.Info("[流量记录] 从探针获取流量",
			"limit_gb", limitGB,
			"used_gb", usedGB,
			"remaining_gb", remainingGB,
			"usage_percent", usagePercent)
	}

	// Sync and add external subscription traffic
	externalLimit, externalUsed := h.syncAndFetchExternalSubscriptionTraffic(ctx)
	if externalLimit > 0 || externalUsed > 0 {
		totalLimit += externalLimit
		totalUsed += externalUsed
		totalRemaining = totalLimit - totalUsed
		if totalRemaining < 0 {
			totalRemaining = 0
		}

		logger.Info("[流量记录] 添加外部订阅流量",
			"limit_gb", bytesToGigabytes(externalLimit),
			"used_gb", bytesToGigabytes(externalUsed))
	}

	// If no traffic data from either source, return error only if probe failed (not just not configured)
	if totalLimit == 0 && totalUsed == 0 && probeErr != nil && !errors.Is(probeErr, storage.ErrProbeConfigNotFound) {
		return probeErr
	}

	// Log total traffic
	limitGB := roundUpTwoDecimals(bytesToGigabytes(totalLimit))
	usedGB := roundUpTwoDecimals(bytesToGigabytes(totalUsed))
	remainingGB := roundUpTwoDecimals(bytesToGigabytes(totalRemaining))
	usagePercent := roundUpTwoDecimals(usagePercentage(totalUsed, totalLimit))

	logger.Info("[流量记录] 总计流量",
		"limit_gb", limitGB,
		"used_gb", usedGB,
		"remaining_gb", remainingGB,
		"usage_percent", usagePercent)

	if err := h.recordSnapshot(ctx, totalLimit, totalUsed, totalRemaining); err != nil {
		logger.Error("[流量记录] 保存快照到数据库失败", "error", err)
		return err
	}

	logger.Info("[流量记录] 快照已成功保存到数据库")
	return nil
}

// syncAndFetchExternalSubscriptionTraffic syncs traffic info from external subscriptions when sync_traffic is enabled (system-level setting)
// Returns totalLimit and totalUsed from non-expired subscriptions
func (h *TrafficSummaryHandler) syncAndFetchExternalSubscriptionTraffic(ctx context.Context) (int64, int64) {
	if h.repo == nil {
		return 0, 0
	}

	// Check if sync_traffic is enabled (system-level setting)
	enabled, err := h.repo.IsSyncTrafficEnabled(ctx)
	if err != nil {
		logger.Warn("[流量记录] 检查sync_traffic设置失败", "error", err)
		return 0, 0
	}

	if !enabled {
		logger.Info("[流量记录] sync_traffic未启用，跳过外部订阅同步")
		return 0, 0
	}

	// Get all external subscriptions from all users
	subs, err := h.repo.ListAllExternalSubscriptions(ctx)
	if err != nil {
		logger.Warn("[流量记录] 获取外部订阅失败", "error", err)
		return 0, 0
	}

	if len(subs) == 0 {
		logger.Info("[Traffic Record] No external subscriptions found")
		return 0, 0
	}

	logger.Info("[流量记录] 同步外部订阅", "count", len(subs))

	var totalLimit, totalUsed int64
	now := time.Now()

	for _, sub := range subs {
		// Fetch and update traffic info from subscription URL
		updatedSub, err := h.fetchExternalSubscriptionTrafficInfo(ctx, sub)
		if err != nil {
			logger.Info("[流量记录] 获取订阅流量失败", "name", sub.Name, "error", err)
			// Use existing data if fetch fails
			updatedSub = sub
		} else {
			// Update subscription in database
			if updateErr := h.repo.UpdateExternalSubscription(ctx, updatedSub); updateErr != nil {
				logger.Info("[流量记录] 更新订阅失败", "name", sub.Name, "error", updateErr)
			}
		}

		// Skip expired subscriptions
		if updatedSub.Expire != nil && updatedSub.Expire.Before(now) {
			logger.Info("[流量记录] 跳过已过期订阅", "name", updatedSub.Name, "expired_at", updatedSub.Expire.Format("2006-01-02 15:04:05"))
			continue
		}

		// Add traffic from this subscription
		totalLimit += updatedSub.Total
		totalUsed += updatedSub.Upload + updatedSub.Download

		if updatedSub.Expire == nil {
			logger.Info("[流量记录] 添加长期订阅流量",
				"name", updatedSub.Name,
				"limit_gb", bytesToGigabytes(updatedSub.Total),
				"used_gb", bytesToGigabytes(updatedSub.Upload+updatedSub.Download))
		} else {
			logger.Info("[流量记录] 添加订阅流量",
				"name", updatedSub.Name,
				"limit_gb", bytesToGigabytes(updatedSub.Total),
				"used_gb", bytesToGigabytes(updatedSub.Upload+updatedSub.Download),
				"expires", updatedSub.Expire.Format("2006-01-02 15:04:05"))
		}
	}

	logger.Info("[流量记录] 外部订阅流量总计",
		"limit_gb", bytesToGigabytes(totalLimit),
		"used_gb", bytesToGigabytes(totalUsed))

	return totalLimit, totalUsed
}

// fetchExternalSubscriptionTrafficInfo fetches traffic info from external subscription URL
func (h *TrafficSummaryHandler) fetchExternalSubscriptionTrafficInfo(ctx context.Context, sub storage.ExternalSubscription) (storage.ExternalSubscription, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.URL, nil)
	if err != nil {
		return sub, fmt.Errorf("create request: %w", err)
	}

	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return sub, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return sub, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse subscription-userinfo header
	userInfo := resp.Header.Get("subscription-userinfo")
	if userInfo == "" {
		return sub, nil // No traffic info available
	}

	// Parse traffic info
	upload, download, total, expire := ParseTrafficInfoHeader(userInfo)

	sub.Upload = upload
	sub.Download = download
	sub.Total = total
	sub.Expire = expire

	logger.Info("[流量记录] 解析流量信息",
		"name", sub.Name,
		"upload_mb", float64(upload)/(1024*1024),
		"download_mb", float64(download)/(1024*1024),
		"total_gb", float64(total)/(1024*1024*1024))

	return sub, nil
}

func (h *TrafficSummaryHandler) fetchTotals(ctx context.Context, username string, allowedProbeServers map[string]struct{}) (int64, int64, int64, error) {
	if h.repo == nil {
		return 0, 0, 0, errors.New("traffic repository not configured")
	}

	// Determine which probe servers to include
	var probeFilter map[string]struct{}

	// If allowedProbeServers is explicitly provided, use it as the filter
	if allowedProbeServers != nil {
		probeFilter = make(map[string]struct{}, len(allowedProbeServers))
		for name := range allowedProbeServers {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				probeFilter[trimmed] = struct{}{}
			}
		}

		// If filter is provided but empty after trimming, return zero traffic
		if len(probeFilter) == 0 {
			logger.Info("[Traffic Fetch] Probe filter provided but no valid servers referenced, returning zero traffic")
			return 0, 0, 0, nil
		}
	} else if username != "" {
		// No explicit filter provided, check if probe binding is enabled for this user
		userSettings, err := h.repo.GetUserSettings(ctx, username)
		if err == nil && userSettings.EnableProbeBinding {
			// Get all nodes for this user
			nodes, err := h.repo.ListNodes(ctx, username)
			if err == nil {
				// Collect unique probe server names that are bound to nodes
				boundProbeServers := make(map[string]struct{})
				for _, node := range nodes {
					name := strings.TrimSpace(node.ProbeServer)
					if name != "" {
						boundProbeServers[name] = struct{}{}
					}
				}

				if len(boundProbeServers) > 0 {
					probeFilter = boundProbeServers
				} else {
					logger.Info("[Traffic Fetch] Probe binding enabled but no nodes have bound servers, returning zero traffic")
					return 0, 0, 0, nil
				}
			}
		}
	}

	cfg, err := h.repo.GetProbeConfig(ctx)
	if err != nil {
		return 0, 0, 0, err
	}

	if len(cfg.Servers) == 0 {
		return 0, 0, 0, errors.New("no probe servers configured")
	}

	// Apply probe filter if one was determined
	if probeFilter != nil {
		filteredServers := make([]storage.ProbeServer, 0, len(cfg.Servers))
		for _, srv := range cfg.Servers {
			name := strings.TrimSpace(srv.Name)
			if name == "" {
				continue
			}
			if _, ok := probeFilter[name]; ok {
				filteredServers = append(filteredServers, srv)
			}
		}

		if len(filteredServers) == 0 {
			logger.Info("[Traffic Fetch] Probe filter applied but no matching servers found, returning zero traffic")
			return 0, 0, 0, nil
		}

		cfg.Servers = filteredServers
		logger.Info("[流量获取] 根据绑定过滤探针服务器", "count", len(cfg.Servers))
	}

	serverIDs := make([]string, 0, len(cfg.Servers))
	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}
		serverIDs = append(serverIDs, id)
	}

	if len(serverIDs) == 0 {
		return 0, 0, 0, errors.New("no server ids configured")
	}

	logger.Info("[流量获取] 探针信息",
		"type", cfg.ProbeType,
		"address", cfg.Address,
		"server_count", len(cfg.Servers),
		"server_ids", serverIDs)

	switch cfg.ProbeType {
	case storage.ProbeTypeNezha:
		return h.fetchNezhaTotals(ctx, cfg)
	case storage.ProbeTypeNezhaV0:
		return h.fetchNezhaV0Totals(ctx, cfg)
	case storage.ProbeTypeDstatus:
		return h.fetchBatchSummary(ctx, cfg.Address, serverIDs)
	case storage.ProbeTypeKomari:
		return h.fetchKomariTotals(ctx, cfg)
	default:
		return 0, 0, 0, fmt.Errorf("unsupported probe type: %s", cfg.ProbeType)
	}
}

func (h *TrafficSummaryHandler) fetchNezhaTotals(ctx context.Context, cfg storage.ProbeConfig) (int64, int64, int64, error) {
	baseAddress := strings.TrimSpace(cfg.Address)
	if baseAddress == "" {
		return 0, 0, 0, errors.New("invalid probe address")
	}

	base, err := url.Parse(baseAddress)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
	}

	switch strings.ToLower(base.Scheme) {
	case "", "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	case "ws", "wss":
		// keep as is
	default:
		base.Scheme = "wss"
	}

	endpoint := &url.URL{Path: "/api/v1/ws/server"}
	target := base.ResolveReference(endpoint)

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.DefaultDialer.DialContext(dialCtx, target.String(), nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return 0, 0, 0, fmt.Errorf("connect probe websocket: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, 0, 0, fmt.Errorf("set websocket deadline: %w", err)
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read probe websocket: %w", err)
	}
	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		return 0, 0, 0, errors.New("empty probe websocket payload")
	}

	type nezhaServer struct {
		ID    json.Number `json:"id"`
		State struct {
			NetInTransfer  json.Number `json:"net_in_transfer"`
			NetOutTransfer json.Number `json:"net_out_transfer"`
		} `json:"state"`
	}

	type nezhaSnapshot struct {
		Servers []nezhaServer `json:"servers"`
	}

	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.UseNumber()

	var snapshot nezhaSnapshot

	if message[0] == '[' {
		var frames []nezhaSnapshot
		if err := decoder.Decode(&frames); err != nil {
			return 0, 0, 0, fmt.Errorf("parse probe websocket payload: %w", err)
		}
		if len(frames) == 0 {
			return 0, 0, 0, errors.New("probe websocket payload missing frames")
		}
		snapshot = frames[len(frames)-1]
	} else {
		if err := decoder.Decode(&snapshot); err != nil {
			return 0, 0, 0, fmt.Errorf("parse probe websocket payload: %w", err)
		}
	}

	observed := make(map[string]struct {
		NetIn  int64
		NetOut int64
	})
	for _, entry := range snapshot.Servers {
		var id string
		if v, err := entry.ID.Int64(); err == nil {
			id = strconv.FormatInt(v, 10)
		} else {
			raw := strings.TrimSpace(entry.ID.String())
			if raw != "" {
				if strings.ContainsAny(raw, ".eE") {
					if f, err := entry.ID.Float64(); err == nil {
						id = strconv.FormatInt(int64(math.Round(f)), 10)
					} else {
						id = raw
					}
				} else {
					id = raw
				}
			} else if f, err := entry.ID.Float64(); err == nil {
				id = strconv.FormatInt(int64(math.Round(f)), 10)
			}
		}

		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		netIn := jsonNumberToInt64(entry.State.NetInTransfer)
		netOut := jsonNumberToInt64(entry.State.NetOutTransfer)
		observed[id] = struct {
			NetIn  int64
			NetOut int64
		}{NetIn: netIn, NetOut: netOut}
	}

	var totalLimit int64
	var totalUsed int64

	logger.Info("[Nezha] 处理服务器流量", "count", len(cfg.Servers))

	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}

		totalLimit += srv.MonthlyTrafficBytes

		wsEntry, ok := observed[id]
		if !ok {
			logger.Info("[Nezha] 服务器未在探针数据中找到", "server_id", id)
			continue
		}

		var used int64
		switch strings.ToLower(strings.TrimSpace(srv.TrafficMethod)) {
		case storage.TrafficMethodUp:
			used = wsEntry.NetOut
		case storage.TrafficMethodDown:
			used = wsEntry.NetIn
		default:
			used = wsEntry.NetIn + wsEntry.NetOut
		}

		if used < 0 {
			used = 0
		}
		if srv.MonthlyTrafficBytes > 0 && used > srv.MonthlyTrafficBytes {
			used = srv.MonthlyTrafficBytes
		}

		logger.Info("[Nezha] 服务器流量",
			"server_id", id,
			"net_in_gb", bytesToGigabytes(wsEntry.NetIn),
			"net_out_gb", bytesToGigabytes(wsEntry.NetOut),
			"method", srv.TrafficMethod,
			"used_gb", bytesToGigabytes(used),
			"limit_gb", bytesToGigabytes(srv.MonthlyTrafficBytes))

		totalUsed += used
	}

	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	logger.Info("[Nezha] 总计流量",
		"limit_gb", bytesToGigabytes(totalLimit),
		"used_gb", bytesToGigabytes(totalUsed),
		"remaining_gb", bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func (h *TrafficSummaryHandler) fetchNezhaV0Totals(ctx context.Context, cfg storage.ProbeConfig) (int64, int64, int64, error) {
	baseAddress := strings.TrimSpace(cfg.Address)
	if baseAddress == "" {
		return 0, 0, 0, errors.New("invalid probe address")
	}

	base, err := url.Parse(baseAddress)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/server"}
	target := base.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, 0, 0, err
	}

	type nezhaV0Server struct {
		ID     json.Number `json:"id"`
		Status struct {
			NetInTransfer  json.Number `json:"NetInTransfer"`
			NetOutTransfer json.Number `json:"NetOutTransfer"`
		} `json:"status"`
	}

	type nezhaV0Response struct {
		Result []nezhaV0Server `json:"result"`
	}

	observed := make(map[string]struct {
		NetIn  int64
		NetOut int64
	})

	httpSuccess := false
	resp, httpErr := h.client.Do(req)
	if httpErr == nil {
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			decoder := json.NewDecoder(resp.Body)
			decoder.UseNumber()

			var payload nezhaV0Response
			if err := decoder.Decode(&payload); err == nil && len(payload.Result) > 0 {
				httpSuccess = true
				for _, entry := range payload.Result {
					var id string
					if v, err := entry.ID.Int64(); err == nil {
						id = strconv.FormatInt(v, 10)
					} else {
						raw := strings.TrimSpace(entry.ID.String())
						if raw != "" {
							if strings.ContainsAny(raw, ".eE") {
								if f, err := entry.ID.Float64(); err == nil {
									id = strconv.FormatInt(int64(math.Round(f)), 10)
								} else {
									id = raw
								}
							} else {
								id = raw
							}
						} else if f, err := entry.ID.Float64(); err == nil {
							id = strconv.FormatInt(int64(math.Round(f)), 10)
						}
					}

					id = strings.TrimSpace(id)
					if id == "" {
						continue
					}

					netIn := jsonNumberToInt64(entry.Status.NetInTransfer)
					netOut := jsonNumberToInt64(entry.Status.NetOutTransfer)
					observed[id] = struct {
						NetIn  int64
						NetOut int64
					}{NetIn: netIn, NetOut: netOut}
				}
			}
		}
	}

	// 如果 HTTP 接口失败或没有数据，尝试使用 WebSocket
	if !httpSuccess {
		wsObserved, wsErr := h.fetchNezhaV0TotalsViaWebSocket(ctx, base)
		if wsErr != nil {
			// WebSocket 也失败了，返回综合错误信息
			if httpErr != nil {
				return 0, 0, 0, fmt.Errorf("HTTP 接口失败: %w; WebSocket 接口也失败: %v", httpErr, wsErr)
			}
			return 0, 0, 0, fmt.Errorf("HTTP 接口未获取到数据; WebSocket 接口也失败: %v", wsErr)
		}
		observed = wsObserved
		logger.Info("[Nezha V0] Using WebSocket data as HTTP API failed or returned no data")
	}

	var totalLimit int64
	var totalUsed int64

	logger.Info("[Nezha V0] 处理服务器流量", "count", len(cfg.Servers))

	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}

		totalLimit += srv.MonthlyTrafficBytes

		entry, ok := observed[id]
		if !ok {
			logger.Info("[Nezha V0] 服务器未在探针数据中找到", "server_id", id)
			continue
		}

		var used int64
		switch strings.ToLower(strings.TrimSpace(srv.TrafficMethod)) {
		case storage.TrafficMethodUp:
			used = entry.NetOut
		case storage.TrafficMethodDown:
			used = entry.NetIn
		default:
			used = entry.NetIn + entry.NetOut
		}

		if used < 0 {
			used = 0
		}
		if srv.MonthlyTrafficBytes > 0 && used > srv.MonthlyTrafficBytes {
			used = srv.MonthlyTrafficBytes
		}

		logger.Info("[Nezha V0] 服务器流量",
			"server_id", id,
			"net_in_gb", bytesToGigabytes(entry.NetIn),
			"net_out_gb", bytesToGigabytes(entry.NetOut),
			"method", srv.TrafficMethod,
			"used_gb", bytesToGigabytes(used),
			"limit_gb", bytesToGigabytes(srv.MonthlyTrafficBytes))

		totalUsed += used
	}

	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	logger.Info("[Nezha V0] 总计流量",
		"limit_gb", bytesToGigabytes(totalLimit),
		"used_gb", bytesToGigabytes(totalUsed),
		"remaining_gb", bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func (h *TrafficSummaryHandler) fetchBatchSummary(ctx context.Context, address string, serverIDs []string) (int64, int64, int64, error) {
	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
	}

	return h.fetchBatchTraffic(ctx, base, serverIDs)
}

func (h *TrafficSummaryHandler) fetchKomariTotals(ctx context.Context, cfg storage.ProbeConfig) (int64, int64, int64, error) {
	baseAddress := strings.TrimSpace(cfg.Address)
	if baseAddress == "" {
		return 0, 0, 0, errors.New("invalid probe address")
	}

	base, err := url.Parse(baseAddress)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/rpc2"}
	target := base.ResolveReference(endpoint)

	// Prepare JSON-RPC request
	rpcRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "common:getNodesLatestStatus",
		"id":      3,
	}

	requestBody, err := json.Marshal(rpcRequest)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshal komari request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(requestBody))
	if err != nil {
		return 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("komari request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, fmt.Errorf("komari request failed with status %s", resp.Status)
	}

	type komariResponse struct {
		Result map[string]struct {
			NetTotalUp   json.Number `json:"net_total_up"`
			NetTotalDown json.Number `json:"net_total_down"`
		} `json:"result"`
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	var payload komariResponse
	if err := decoder.Decode(&payload); err != nil {
		return 0, 0, 0, fmt.Errorf("parse komari response: %w", err)
	}

	observed := make(map[string]struct {
		Up   int64
		Down int64
	})
	for id, info := range payload.Result {
		cleanID := strings.TrimSpace(id)
		if cleanID == "" {
			continue
		}

		up := jsonNumberToInt64(info.NetTotalUp)
		if up < 0 {
			up = 0
		}
		down := jsonNumberToInt64(info.NetTotalDown)
		if down < 0 {
			down = 0
		}

		observed[cleanID] = struct {
			Up   int64
			Down int64
		}{Up: up, Down: down}
	}

	var totalLimit int64
	var totalUsed int64

	logger.Info("[Komari] 处理服务器流量", "count", len(cfg.Servers))

	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}

		totalLimit += srv.MonthlyTrafficBytes

		usage, ok := observed[id]
		if !ok {
			logger.Info("[Komari] 服务器未在探针数据中找到", "server_id", id)
			continue
		}

		var used int64
		switch strings.ToLower(strings.TrimSpace(srv.TrafficMethod)) {
		case storage.TrafficMethodUp:
			used = usage.Up
		case storage.TrafficMethodDown:
			used = usage.Down
		default:
			used = usage.Up + usage.Down
		}

		if used < 0 {
			used = 0
		}
		if srv.MonthlyTrafficBytes > 0 && used > srv.MonthlyTrafficBytes {
			used = srv.MonthlyTrafficBytes
		}

		logger.Info("[Komari] 服务器流量",
			"server_id", id,
			"up_gb", bytesToGigabytes(usage.Up),
			"down_gb", bytesToGigabytes(usage.Down),
			"method", srv.TrafficMethod,
			"used_gb", bytesToGigabytes(used),
			"limit_gb", bytesToGigabytes(srv.MonthlyTrafficBytes))

		totalUsed += used
	}

	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	logger.Info("[Komari] 总计流量",
		"limit_gb", bytesToGigabytes(totalLimit),
		"used_gb", bytesToGigabytes(totalUsed),
		"remaining_gb", bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func (h *TrafficSummaryHandler) fetchBatchTraffic(ctx context.Context, base *url.URL, serverIDs []string) (int64, int64, int64, error) {
	payload, err := json.Marshal(map[string][]string{"serverIds": serverIDs})
	if err != nil {
		return 0, 0, 0, err
	}

	endpoint := &url.URL{Path: "/stats/batch-traffic"}
	target := base.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		return 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "miaomiaowu/0.1")

	resp, err := h.client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, errors.New("batch traffic request failed with status " + resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	var payloadResp batchTrafficResponse
	if err := decoder.Decode(&payloadResp); err != nil {
		return 0, 0, 0, err
	}

	if !payloadResp.Success {
		if payloadResp.Message != "" {
			return 0, 0, 0, errors.New(payloadResp.Message)
		}
		return 0, 0, 0, errors.New("batch traffic request unsuccessful")
	}

	var totalLimit int64
	var totalRemaining int64
	var totalUsed int64

	logger.Info("[Dstatus] 处理服务器流量", "count", len(payloadResp.Data))

	for serverID, entry := range payloadResp.Data {
		limit := jsonNumberToInt64(entry.Monthly.Limit)
		used := jsonNumberToInt64(entry.Monthly.Used)
		remaining := jsonNumberToInt64(entry.Monthly.Remaining)

		logger.Info("[Dstatus] 服务器流量",
			"server_id", serverID,
			"limit_gb", bytesToGigabytes(limit),
			"used_gb", bytesToGigabytes(used),
			"remaining_gb", bytesToGigabytes(remaining))

		totalLimit += limit
		totalRemaining += remaining
		totalUsed += used
	}

	logger.Info("[Dstatus] 总计流量",
		"limit_gb", bytesToGigabytes(totalLimit),
		"used_gb", bytesToGigabytes(totalUsed),
		"remaining_gb", bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func jsonNumberToInt64(n json.Number) int64 {
	if n == "" {
		return 0
	}
	if v, err := n.Int64(); err == nil {
		return v
	}
	if f, err := n.Float64(); err == nil {
		if f < 0 {
			return int64(f - 0.5)
		}
		return int64(f + 0.5)
	}
	return 0
}

func roundUpTwoDecimals(value float64) float64 {
	return math.Ceil(value*100) / 100
}

func bytesToGigabytes(total int64) float64 {
	if total <= 0 {
		return 0
	}

	return float64(total) / bytesPerGigabyte
}

func usagePercentage(used, limit int64) float64 {
	if limit <= 0 {
		return 0
	}

	return (float64(used) / float64(limit)) * 100
}

func (h *TrafficSummaryHandler) recordSnapshot(ctx context.Context, totalLimit, totalUsed, totalRemaining int64) error {
	if h.repo == nil {
		return nil
	}

	return h.repo.RecordDaily(ctx, time.Now(), totalLimit, totalUsed, totalRemaining)
}

func (h *TrafficSummaryHandler) loadHistory(ctx context.Context, days int) ([]trafficDailyUsage, error) {
	if h.repo == nil {
		return nil, nil
	}

	records, err := h.repo.ListRecent(ctx, days)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Date.Before(records[j].Date)
	})

	usages := make([]trafficDailyUsage, 0, len(records))
	var prevUsed int64
	var hasPrev bool

	for _, record := range records {
		delta := record.TotalUsed
		if hasPrev {
			delta = record.TotalUsed - prevUsed
			if delta < 0 {
				delta = 0
			}
		}

		prevUsed = record.TotalUsed
		hasPrev = true

		usages = append(usages, trafficDailyUsage{
			Date:   record.Date.Format("2006-01-02"),
			UsedGB: roundUpTwoDecimals(bytesToGigabytes(delta)),
		})
	}

	return usages, nil
}

// fetchExternalSubscriptionTraffic fetches traffic from external subscriptions that are actually used in subscription files
// Returns totalLimit and totalUsed from non-expired subscriptions (or long-term subscriptions without expire date)
func (h *TrafficSummaryHandler) fetchExternalSubscriptionTraffic(ctx context.Context, username string) (int64, int64) {
	// Check if sync_traffic is enabled
	settings, err := h.repo.GetUserSettings(ctx, username)
	if err != nil || !settings.SyncTraffic {
		return 0, 0
	}

	// Get all subscription files for this user
	subscribeFiles, err := h.repo.ListSubscribeFiles(ctx)
	if err != nil {
		logger.Info("[流量] 获取订阅文件列表失败", "error", err)
		return 0, 0
	}

	// Collect all external subscription URLs used across all subscription files
	usedExternalURLs := make(map[string]bool)
	for _, file := range subscribeFiles {
		// Read subscription file content
		filePath := filepath.Join("subscribes", file.Filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Info("[流量] 读取订阅文件失败", "filename", file.Filename, "error", err)
			continue
		}

		// Get external subscription URLs referenced in this file
		fileURLs, err := GetExternalSubscriptionsFromFile(ctx, data, username, h.repo)
		if err != nil {
			logger.Info("[流量] 解析订阅文件失败", "filename", file.Filename, "error", err)
			continue
		}

		// Merge into used URLs
		for url := range fileURLs {
			usedExternalURLs[url] = true
		}
	}

	if len(usedExternalURLs) == 0 {
		logger.Info("[流量] 未找到使用中的外部订阅")
		return 0, 0
	}

	logger.Info("[流量] 找到使用中的外部订阅", "count", len(usedExternalURLs))

	// Get all external subscriptions
	subs, err := h.repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		logger.Info("[流量] 获取外部订阅失败", "error", err)
		return 0, 0
	}

	var totalLimit int64
	var totalUsed int64
	now := time.Now()

	for _, sub := range subs {
		// Skip if this subscription is not used in any subscription file
		if !usedExternalURLs[sub.URL] {
			continue
		}

		// Skip if subscription is expired
		// If Expire is nil, it's a long-term subscription and should not be skipped
		if sub.Expire != nil && sub.Expire.Before(now) {
			logger.Info("[流量] 跳过已过期订阅", "name", sub.Name, "expired_at", sub.Expire.Format("2006-01-02 15:04:05"))
			continue
		}

		// Add traffic from this subscription
		totalLimit += sub.Total
		totalUsed += sub.Upload + sub.Download

		if sub.Expire == nil {
			logger.Info("[流量] 添加长期订阅流量", "name", sub.Name, "limit", sub.Total, "used", sub.Upload+sub.Download)
		} else {
			logger.Info("[流量] 添加订阅流量",
				"name", sub.Name,
				"limit", sub.Total,
				"used", sub.Upload+sub.Download,
				"expires", sub.Expire.Format("2006-01-02 15:04:05"))
		}
	}

	logger.Info("[流量] 外部订阅流量总计", "limit", totalLimit, "used", totalUsed)
	return totalLimit, totalUsed
}

func (h *TrafficSummaryHandler) fetchNezhaV0TotalsViaWebSocket(ctx context.Context, base *url.URL) (map[string]struct {
	NetIn  int64
	NetOut int64
}, error) {
	// 转换 scheme 为 WebSocket
	wsBase := *base // 复制以避免修改原始 URL
	switch strings.ToLower(wsBase.Scheme) {
	case "", "http":
		wsBase.Scheme = "ws"
	case "https":
		wsBase.Scheme = "wss"
	case "ws", "wss":
		// keep as is
	default:
		wsBase.Scheme = "wss"
	}

	endpoint := &url.URL{Path: "/ws"}
	target := wsBase.ResolveReference(endpoint)

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.DefaultDialer.DialContext(dialCtx, target.String(), nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf("无法连接到 WebSocket 接口: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, fmt.Errorf("set websocket deadline: %w", err)
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("未在期望时间内收到服务器数据: %w", err)
	}

	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		return nil, errors.New("empty probe websocket payload")
	}

	type nezhaServer struct {
		ID     json.Number `json:"id"`
		Status struct {
			NetInTransfer  json.Number `json:"NetInTransfer"`
			NetOutTransfer json.Number `json:"NetOutTransfer"`
		} `json:"State"`
	}

	type nezhaSnapshot struct {
		Servers []nezhaServer `json:"servers"`
	}

	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.UseNumber()

	var snapshot nezhaSnapshot

	if message[0] == '[' {
		var frames []nezhaSnapshot
		if err := decoder.Decode(&frames); err != nil {
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
		if len(frames) == 0 {
			return nil, errors.New("探针未返回任何服务器数据")
		}
		snapshot = frames[len(frames)-1]
	} else {
		if err := decoder.Decode(&snapshot); err != nil {
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
	}

	if len(snapshot.Servers) == 0 {
		return nil, errors.New("探针未返回任何服务器数据")
	}

	observed := make(map[string]struct {
		NetIn  int64
		NetOut int64
	})

	for _, entry := range snapshot.Servers {
		var id string
		if v, err := entry.ID.Int64(); err == nil {
			id = strconv.FormatInt(v, 10)
		} else {
			raw := strings.TrimSpace(entry.ID.String())
			if raw != "" {
				if strings.ContainsAny(raw, ".eE") {
					if f, err := entry.ID.Float64(); err == nil {
						id = strconv.FormatInt(int64(math.Round(f)), 10)
					} else {
						id = raw
					}
				} else {
					id = raw
				}
			} else if f, err := entry.ID.Float64(); err == nil {
				id = strconv.FormatInt(int64(math.Round(f)), 10)
			}
		}

		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		netIn := jsonNumberToInt64(entry.Status.NetInTransfer)
		netOut := jsonNumberToInt64(entry.Status.NetOutTransfer)
		observed[id] = struct {
			NetIn  int64
			NetOut int64
		}{NetIn: netIn, NetOut: netOut}
	}

	return observed, nil
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
}
