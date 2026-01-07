package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	proxygroups "miaomiaowu/proxy_groups"
)

// ProxyGroupsHandler handles GET requests for proxy groups configuration
type proxyGroupsHandler struct {
	configPath string
}

// NewProxyGroupsHandler creates a new handler for retrieving proxy groups config
func NewProxyGroupsHandler(configPath string) http.Handler {
	if configPath == "" {
		panic("proxy groups handler requires config path")
	}
	return &proxyGroupsHandler{configPath: configPath}
}

func (h *proxyGroupsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "仅支持 GET 请求", http.StatusMethodNotAllowed)
		return
	}

	var data any
	if err := proxygroups.Load(h.configPath, &data); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// ProxyGroupsSyncHandler handles POST requests to sync proxy groups config from remote
type proxyGroupsSyncHandler struct {
	configPath string
}

type proxyGroupsSyncRequest struct {
	SourceURL string `json:"source_url"`
}

type proxyGroupsSyncResponse struct {
	Message   string `json:"message"`
	SourceURL string `json:"source_url,omitempty"`
	Timestamp string `json:"timestamp"`
}

// NewProxyGroupsSyncHandler creates a new handler for syncing proxy groups config
func NewProxyGroupsSyncHandler(configPath string) http.Handler {
	if configPath == "" {
		panic("proxy groups sync handler requires config path")
	}
	return &proxyGroupsSyncHandler{
		configPath: configPath,
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

	// Resolve the actual source URL (request > env > default)
	sourceURL := payload.SourceURL
	resolvedURL := proxygroups.ResolveSourceURL(sourceURL)

	// Sync configuration from the resolved URL
	if err := proxygroups.SyncFromSource(h.configPath, sourceURL); err != nil {
		// Return appropriate HTTP status based on error type
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

	// Return success response with source URL
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proxyGroupsSyncResponse{
		Message:   fmt.Sprintf("代理组配置同步成功 (来源: %s)", resolvedURL),
		SourceURL: resolvedURL,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}
