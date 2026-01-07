package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"miaomiaowu/internal/proxygroups"
	"miaomiaowu/internal/storage"
)

// ProxyGroupsHandler 提供代理组配置的读取接口
// 从内存 Store 中获取当前配置快照
type proxyGroupsHandler struct {
	store *proxygroups.Store
}

// NewProxyGroupsHandler 创建代理组配置处理器
func NewProxyGroupsHandler(store *proxygroups.Store) http.Handler {
	if store == nil {
		panic("proxy groups handler requires store")
	}
	return &proxyGroupsHandler{store: store}
}

func (h *proxyGroupsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "仅支持 GET 请求", http.StatusMethodNotAllowed)
		return
	}

	// 获取配置快照及元数据
	data, sourceURL, syncedAt := h.store.Snapshot()

	// 设置响应头,提供配置来源和同步时间信息
	w.Header().Set("Content-Type", "application/json")
	if sourceURL != "" {
		w.Header().Set("X-Proxy-Groups-Source", sourceURL)
	}
	if !syncedAt.IsZero() {
		w.Header().Set("X-Proxy-Groups-Synced-At", syncedAt.Format(time.RFC3339))
	}

	// 直接返回 JSON 数据(已验证)
	_, _ = w.Write(data)
}

// ProxyGroupsSyncHandler 处理代理组配置的远程同步请求
type proxyGroupsSyncHandler struct {
	repo  *storage.TrafficRepository
	store *proxygroups.Store
}

type proxyGroupsSyncRequest struct {
	SourceURL string `json:"source_url"`
}

type proxyGroupsSyncResponse struct {
	Message   string `json:"message"`
	SourceURL string `json:"source_url,omitempty"`
	Timestamp string `json:"timestamp"`
}

// NewProxyGroupsSyncHandler 创建同步处理器
// 需要 repository 来获取持久化的远程地址配置
func NewProxyGroupsSyncHandler(repo *storage.TrafficRepository, store *proxygroups.Store) http.Handler {
	if repo == nil || store == nil {
		panic("proxy groups sync handler requires repository and store")
	}
	return &proxyGroupsSyncHandler{
		repo:  repo,
		store: store,
	}
}

func (h *proxyGroupsSyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	var payload proxyGroupsSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	// 优先使用请求中的 URL,如果为空则从系统配置中获取
	sourceURL := strings.TrimSpace(payload.SourceURL)
	if sourceURL == "" {
		systemConfig, err := h.repo.GetSystemConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get system config: %w", err))
			return
		}
		sourceURL = strings.TrimSpace(systemConfig.ProxyGroupsSourceURL)
	}

	// 从远程下载并验证配置
	data, resolvedURL, err := proxygroups.FetchConfig(sourceURL)
	if err != nil {
		switch {
		case errors.Is(err, proxygroups.ErrInvalidConfig):
			writeError(w, http.StatusBadRequest, err)
		case errors.Is(err, proxygroups.ErrDownloadFailed):
			writeError(w, http.StatusBadGateway, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	// 更新内存中的配置
	if err := h.store.Update(data, resolvedURL, time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("update store: %w", err))
		return
	}

	// 返回同步成功响应
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proxyGroupsSyncResponse{
		Message:   fmt.Sprintf("代理组配置同步成功 (来源: %s)", resolvedURL),
		SourceURL: resolvedURL,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}
