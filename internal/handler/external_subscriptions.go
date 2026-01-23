package handler

import (
	"encoding/json"
	"errors"
	"miaomiaowu/internal/logger"
	"net/http"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type externalSubscriptionRequest struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	UserAgent   string `json:"user_agent"`
	TrafficMode string `json:"traffic_mode"` // 流量统计方式: "download", "upload", "both"
}

type externalSubscriptionResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	UserAgent   string  `json:"user_agent"`
	NodeCount   int     `json:"node_count"`
	LastSyncAt  *string `json:"last_sync_at"`
	Upload      int64   `json:"upload"`       // 已上传流量（字节）
	Download    int64   `json:"download"`     // 已下载流量（字节）
	Total       int64   `json:"total"`        // 总流量（字节）
	Expire      *string `json:"expire"`       // 过期时间
	TrafficMode string  `json:"traffic_mode"` // 流量统计方式: "download", "upload", "both"
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func NewExternalSubscriptionsHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("external subscriptions handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleListExternalSubscriptions(w, r, repo, username)
		case http.MethodPost:
			handleCreateExternalSubscription(w, r, repo, username)
		case http.MethodPut:
			handleUpdateExternalSubscription(w, r, repo, username)
		case http.MethodDelete:
			handleDeleteExternalSubscription(w, r, repo, username)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	})
}

func handleListExternalSubscriptions(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	subs, err := repo.ListExternalSubscriptions(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := make([]externalSubscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		var lastSyncAt *string
		if sub.LastSyncAt != nil {
			formatted := sub.LastSyncAt.Format(time.RFC3339)
			lastSyncAt = &formatted
		}

		var expire *string
		if sub.Expire != nil {
			formatted := sub.Expire.Format(time.RFC3339)
			expire = &formatted
		}

		resp = append(resp, externalSubscriptionResponse{
			ID:          sub.ID,
			Name:        sub.Name,
			URL:         sub.URL,
			UserAgent:   sub.UserAgent,
			NodeCount:   sub.NodeCount,
			LastSyncAt:  lastSyncAt,
			Upload:      sub.Upload,
			Download:    sub.Download,
			Total:       sub.Total,
			Expire:      expire,
			TrafficMode: sub.TrafficMode,
			CreatedAt:   sub.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   sub.UpdatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleCreateExternalSubscription(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	var payload externalSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription name is required"))
		return
	}

	url := strings.TrimSpace(payload.URL)
	if url == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription url is required"))
		return
	}

	// Fetch subscription to get traffic info
	var trafficUpload, trafficDownload, trafficTotal int64
	var trafficExpire *time.Time

	userAgent := payload.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		logger.Info("[外部订阅] 创建请求失败", "name", name, "error", err)
	} else {
		req.Header.Set("User-Agent", userAgent)
		logger.Info("[外部订阅] 获取流量信息", "name", name, "user_agent", userAgent)
		resp, err := client.Do(req)
		if err != nil {
			logger.Info("[外部订阅] 请求失败", "error", err)
		} else {
			defer resp.Body.Close()
			logger.Info("[外部订阅] 响应状态", "name", name, "status_code", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				// Parse subscription-userinfo header for traffic info
				userInfo := resp.Header.Get("subscription-userinfo")
				logger.Info("[外部订阅] subscription-userinfo头", "name", name, "header", userInfo)
				if userInfo != "" {
					trafficUpload, trafficDownload, trafficTotal, trafficExpire = ParseTrafficInfoHeader(userInfo)
					logger.Info("[外部订阅] 解析流量信息", "upload", trafficUpload, "download", trafficDownload, "total", trafficTotal)
				}
			}
		}
	}

	// 如果使用的不是 clash-meta UA 且没有获取到流量信息，尝试用 clash-meta UA 再次请求
	clashMetaUA := "clash-meta/2.4.0"
	if trafficTotal == 0 && !strings.Contains(strings.ToLower(userAgent), "clash") {
		logger.Info("[外部订阅] 未获取到流量信息，尝试使用 clash-meta UA 重新获取", "name", name)
		retryReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
		if err == nil {
			retryReq.Header.Set("User-Agent", clashMetaUA)
			retryResp, err := client.Do(retryReq)
			if err == nil {
				defer retryResp.Body.Close()
				if retryResp.StatusCode == http.StatusOK {
					userInfo := retryResp.Header.Get("subscription-userinfo")
					logger.Info("[外部订阅] clash-meta UA 获取到 subscription-userinfo", "name", name, "header", userInfo)
					if userInfo != "" {
						trafficUpload, trafficDownload, trafficTotal, trafficExpire = ParseTrafficInfoHeader(userInfo)
						logger.Info("[外部订阅] clash-meta UA 解析流量信息成功", "upload", trafficUpload, "download", trafficDownload, "total", trafficTotal)
					}
				}
			} else {
				logger.Info("[外部订阅] clash-meta UA 请求失败", "error", err)
			}
		}
	}

	now := time.Now()
	sub := storage.ExternalSubscription{
		Username:    username,
		Name:        name,
		URL:         url,
		UserAgent:   payload.UserAgent,   // 会在存储层使用默认值如果为空
		TrafficMode: payload.TrafficMode, // 会在存储层使用默认值如果为空
		NodeCount:   0,
		LastSyncAt:  &now,
		Upload:      trafficUpload,
		Download:    trafficDownload,
		Total:       trafficTotal,
		Expire:      trafficExpire,
	}

	id, err := repo.CreateExternalSubscription(r.Context(), sub)
	if err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionExists) {
			writeError(w, http.StatusConflict, errors.New("subscription with this URL already exists"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	created, err := repo.GetExternalSubscription(r.Context(), id, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var lastSyncAt *string
	if created.LastSyncAt != nil {
		formatted := created.LastSyncAt.Format(time.RFC3339)
		lastSyncAt = &formatted
	}

	var expire *string
	if created.Expire != nil {
		formatted := created.Expire.Format(time.RFC3339)
		expire = &formatted
	}

	resp := externalSubscriptionResponse{
		ID:          created.ID,
		Name:        created.Name,
		URL:         created.URL,
		UserAgent:   created.UserAgent,
		NodeCount:   created.NodeCount,
		LastSyncAt:  lastSyncAt,
		Upload:      created.Upload,
		Download:    created.Download,
		Total:       created.Total,
		Expire:      expire,
		TrafficMode: created.TrafficMode,
		CreatedAt:   created.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   created.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleUpdateExternalSubscription(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid subscription id"))
		return
	}

	var payload externalSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription name is required"))
		return
	}

	url := strings.TrimSpace(payload.URL)
	if url == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription url is required"))
		return
	}

	existing, err := repo.GetExternalSubscription(r.Context(), id, username)
	if err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
			writeError(w, http.StatusNotFound, errors.New("subscription not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 如果没有传 TrafficMode，保留现有的
	trafficMode := payload.TrafficMode
	if trafficMode == "" {
		trafficMode = existing.TrafficMode
	}

	sub := storage.ExternalSubscription{
		ID:          id,
		Username:    username,
		Name:        name,
		URL:         url,
		UserAgent:   payload.UserAgent, // 会在存储层使用默认值如果为空
		TrafficMode: trafficMode,
		NodeCount:   existing.NodeCount,
		LastSyncAt:  existing.LastSyncAt,
		Upload:      existing.Upload,
		Download:    existing.Download,
		Total:       existing.Total,
		Expire:      existing.Expire,
	}

	if err := repo.UpdateExternalSubscription(r.Context(), sub); err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
			writeError(w, http.StatusNotFound, errors.New("subscription not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	updated, err := repo.GetExternalSubscription(r.Context(), id, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var lastSyncAt *string
	if updated.LastSyncAt != nil {
		formatted := updated.LastSyncAt.Format(time.RFC3339)
		lastSyncAt = &formatted
	}

	var expire *string
	if updated.Expire != nil {
		formatted := updated.Expire.Format(time.RFC3339)
		expire = &formatted
	}

	resp := externalSubscriptionResponse{
		ID:          updated.ID,
		Name:        updated.Name,
		URL:         updated.URL,
		UserAgent:   updated.UserAgent,
		NodeCount:   updated.NodeCount,
		LastSyncAt:  lastSyncAt,
		Upload:      updated.Upload,
		Download:    updated.Download,
		Total:       updated.Total,
		Expire:      expire,
		TrafficMode: updated.TrafficMode,
		CreatedAt:   updated.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   updated.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleDeleteExternalSubscription(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid subscription id"))
		return
	}

	if err := repo.DeleteExternalSubscription(r.Context(), id, username); err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
			writeError(w, http.StatusNotFound, errors.New("subscription not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// NewExternalSubscriptionNodesHandler returns a handler that lists node names from an external subscription
func NewExternalSubscriptionNodesHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription id is required"))
			return
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid subscription id"))
			return
		}

		// 获取外部订阅
		sub, err := repo.GetExternalSubscription(r.Context(), id, username)
		if err != nil {
			if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
				writeError(w, http.StatusNotFound, errors.New("subscription not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// 获取节点信息列表（名称和服务器地址）
		nodes, err := fetchSubscriptionNodes(&sub)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// 提取节点名称
		nodeNames := make([]string, len(nodes))
		for i, node := range nodes {
			nodeNames[i] = node.Name
		}

		logger.Info("[外部订阅节点] 返回节点列表", "subscription", sub.Name, "count", len(nodeNames))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node_names": nodeNames,
			"nodes":      nodes,
			"count":      len(nodes),
		})
	})
}

// NewExternalSubscriptionCheckFilterHandler returns a handler that checks if a filter matches any nodes
func NewExternalSubscriptionCheckFilterHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		var req struct {
			SubscriptionID int64  `json:"subscription_id"`
			Filter         string `json:"filter"`
			ExcludeFilter  string `json:"exclude_filter"`
			GeoIPFilter    string `json:"geo_ip_filter"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		// 获取外部订阅
		sub, err := repo.GetExternalSubscription(r.Context(), req.SubscriptionID, username)
		if err != nil {
			if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
				writeError(w, http.StatusNotFound, errors.New("subscription not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// 检查过滤条件是否有匹配的节点
		matchCount, err := checkFilterMatches(&sub, req.Filter, req.ExcludeFilter, req.GeoIPFilter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"has_matches": matchCount > 0,
			"match_count": matchCount,
		})
	})
}
