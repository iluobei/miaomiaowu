package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	client     *http.Client
}

type proxyGroupsSyncRequest struct {
	SourceURL string `json:"source_url"`
}

type proxyGroupsSyncResponse struct {
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// NewProxyGroupsSyncHandler creates a new handler for syncing proxy groups config
func NewProxyGroupsSyncHandler(configPath string) http.Handler {
	if configPath == "" {
		panic("proxy groups sync handler requires config path")
	}
	return &proxyGroupsSyncHandler{
		configPath: configPath,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
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

	// Determine source URL: request payload > environment variable
	sourceURL := payload.SourceURL
	if sourceURL == "" {
		sourceURL = os.Getenv("PROXY_GROUPS_SOURCE_URL")
	}
	if sourceURL == "" {
		// Default GitHub URL
		sourceURL = "https://raw.githubusercontent.com/你的用户名/你的仓库/main/configs/proxy-groups.json"
	}

	// Download config from remote
	resp, err := h.client.Get(sourceURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("failed to download config: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Errorf("remote returned status %d", resp.StatusCode))
		return
	}

	// Read response body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("failed to read response: %w", err))
		return
	}

	// Validate JSON structure
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON config: %w", err))
		return
	}

	// Ensure target directory exists
	dir := filepath.Dir(h.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to create config dir: %w", err))
		return
	}

	// Write to temporary file first (atomic write)
	tmpPath := h.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to write temp file: %w", err))
		return
	}

	// Rename to final destination
	if err := os.Rename(tmpPath, h.configPath); err != nil {
		// Cleanup temp file on error
		_ = os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to replace config file: %w", err))
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proxyGroupsSyncResponse{
		Message:   "代理组配置同步成功",
		Timestamp: time.Now().Format(time.RFC3339),
	})
}
