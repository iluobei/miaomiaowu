package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type proxyProviderConfigRequest struct {
	ExternalSubscriptionID int64  `json:"external_subscription_id"`
	Name                   string `json:"name"`
	Type                   string `json:"type"`
	Interval               int    `json:"interval"`
	Proxy                  string `json:"proxy"`
	SizeLimit              int    `json:"size_limit"`
	Header                 string `json:"header"` // JSON string

	HealthCheckEnabled       bool   `json:"health_check_enabled"`
	HealthCheckURL           string `json:"health_check_url"`
	HealthCheckInterval      int    `json:"health_check_interval"`
	HealthCheckTimeout       int    `json:"health_check_timeout"`
	HealthCheckLazy          bool   `json:"health_check_lazy"`
	HealthCheckExpectedStatus int   `json:"health_check_expected_status"`

	Filter        string `json:"filter"`
	ExcludeFilter string `json:"exclude_filter"`
	ExcludeType   string `json:"exclude_type"`
	Override      string `json:"override"` // JSON string

	ProcessMode string `json:"process_mode"` // 'client' or 'mmw'
}

type proxyProviderConfigResponse struct {
	ID                       int64  `json:"id"`
	ExternalSubscriptionID   int64  `json:"external_subscription_id"`
	Name                     string `json:"name"`
	Type                     string `json:"type"`
	Interval                 int    `json:"interval"`
	Proxy                    string `json:"proxy"`
	SizeLimit                int    `json:"size_limit"`
	Header                   string `json:"header"`
	HealthCheckEnabled       bool   `json:"health_check_enabled"`
	HealthCheckURL           string `json:"health_check_url"`
	HealthCheckInterval      int    `json:"health_check_interval"`
	HealthCheckTimeout       int    `json:"health_check_timeout"`
	HealthCheckLazy          bool   `json:"health_check_lazy"`
	HealthCheckExpectedStatus int   `json:"health_check_expected_status"`
	Filter                   string `json:"filter"`
	ExcludeFilter            string `json:"exclude_filter"`
	ExcludeType              string `json:"exclude_type"`
	Override                 string `json:"override"`
	ProcessMode              string `json:"process_mode"`
	CreatedAt                string `json:"created_at"`
	UpdatedAt                string `json:"updated_at"`
}

func NewProxyProviderConfigsHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("proxy provider configs handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleListProxyProviderConfigs(w, r, repo, username)
		case http.MethodPost:
			handleCreateProxyProviderConfig(w, r, repo, username)
		case http.MethodPut:
			handleUpdateProxyProviderConfig(w, r, repo, username)
		case http.MethodDelete:
			handleDeleteProxyProviderConfig(w, r, repo, username)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	})
}

func handleListProxyProviderConfigs(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	// Check if filtering by external_subscription_id
	externalSubIDStr := r.URL.Query().Get("external_subscription_id")

	var configs []storage.ProxyProviderConfig
	var err error

	if externalSubIDStr != "" {
		externalSubID, parseErr := strconv.ParseInt(externalSubIDStr, 10, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid external_subscription_id"))
			return
		}
		configs, err = repo.ListProxyProviderConfigsBySubscription(r.Context(), externalSubID)
	} else {
		configs, err = repo.ListProxyProviderConfigs(r.Context(), username)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := make([]proxyProviderConfigResponse, 0, len(configs))
	for _, config := range configs {
		resp = append(resp, toProxyProviderConfigResponse(config))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleCreateProxyProviderConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	var payload proxyProviderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("proxy provider name is required"))
		return
	}

	if payload.ExternalSubscriptionID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("external_subscription_id is required"))
		return
	}

	// Verify that external subscription exists and belongs to user
	sub, err := repo.GetExternalSubscription(r.Context(), payload.ExternalSubscriptionID, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if sub.ID == 0 {
		writeError(w, http.StatusNotFound, errors.New("external subscription not found"))
		return
	}

	// Set defaults
	configType := payload.Type
	if configType == "" {
		configType = "http"
	}
	interval := payload.Interval
	if interval <= 0 {
		interval = 3600
	}
	proxy := payload.Proxy
	if proxy == "" {
		proxy = "DIRECT"
	}
	healthCheckURL := payload.HealthCheckURL
	if healthCheckURL == "" {
		healthCheckURL = "https://www.gstatic.com/generate_204"
	}
	healthCheckInterval := payload.HealthCheckInterval
	if healthCheckInterval <= 0 {
		healthCheckInterval = 300
	}
	healthCheckTimeout := payload.HealthCheckTimeout
	if healthCheckTimeout <= 0 {
		healthCheckTimeout = 5000
	}
	healthCheckExpectedStatus := payload.HealthCheckExpectedStatus
	if healthCheckExpectedStatus <= 0 {
		healthCheckExpectedStatus = 204
	}
	processMode := payload.ProcessMode
	if processMode == "" {
		processMode = "client"
	}

	config := &storage.ProxyProviderConfig{
		Username:                 username,
		ExternalSubscriptionID:   payload.ExternalSubscriptionID,
		Name:                     name,
		Type:                     configType,
		Interval:                 interval,
		Proxy:                    proxy,
		SizeLimit:                payload.SizeLimit,
		Header:                   payload.Header,
		HealthCheckEnabled:       payload.HealthCheckEnabled,
		HealthCheckURL:           healthCheckURL,
		HealthCheckInterval:      healthCheckInterval,
		HealthCheckTimeout:       healthCheckTimeout,
		HealthCheckLazy:          payload.HealthCheckLazy,
		HealthCheckExpectedStatus: healthCheckExpectedStatus,
		Filter:                   payload.Filter,
		ExcludeFilter:            payload.ExcludeFilter,
		ExcludeType:              payload.ExcludeType,
		Override:                 payload.Override,
		ProcessMode:              processMode,
	}

	id, err := repo.CreateProxyProviderConfig(r.Context(), config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	config.ID = id
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toProxyProviderConfigResponse(*config))
}

func handleUpdateProxyProviderConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	var payload proxyProviderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("proxy provider name is required"))
		return
	}

	// Verify that config exists and belongs to user
	existing, err := repo.GetProxyProviderConfig(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if existing == nil || existing.Username != username {
		writeError(w, http.StatusNotFound, errors.New("proxy provider config not found"))
		return
	}

	// Set defaults
	configType := payload.Type
	if configType == "" {
		configType = "http"
	}
	interval := payload.Interval
	if interval <= 0 {
		interval = 3600
	}
	proxy := payload.Proxy
	if proxy == "" {
		proxy = "DIRECT"
	}
	healthCheckURL := payload.HealthCheckURL
	if healthCheckURL == "" {
		healthCheckURL = "https://www.gstatic.com/generate_204"
	}
	healthCheckInterval := payload.HealthCheckInterval
	if healthCheckInterval <= 0 {
		healthCheckInterval = 300
	}
	healthCheckTimeout := payload.HealthCheckTimeout
	if healthCheckTimeout <= 0 {
		healthCheckTimeout = 5000
	}
	healthCheckExpectedStatus := payload.HealthCheckExpectedStatus
	if healthCheckExpectedStatus <= 0 {
		healthCheckExpectedStatus = 204
	}
	processMode := payload.ProcessMode
	if processMode == "" {
		processMode = "client"
	}

	config := &storage.ProxyProviderConfig{
		ID:                        id,
		Username:                  username,
		ExternalSubscriptionID:    existing.ExternalSubscriptionID,
		Name:                      name,
		Type:                      configType,
		Interval:                  interval,
		Proxy:                     proxy,
		SizeLimit:                 payload.SizeLimit,
		Header:                    payload.Header,
		HealthCheckEnabled:        payload.HealthCheckEnabled,
		HealthCheckURL:            healthCheckURL,
		HealthCheckInterval:       healthCheckInterval,
		HealthCheckTimeout:        healthCheckTimeout,
		HealthCheckLazy:           payload.HealthCheckLazy,
		HealthCheckExpectedStatus: healthCheckExpectedStatus,
		Filter:                    payload.Filter,
		ExcludeFilter:             payload.ExcludeFilter,
		ExcludeType:               payload.ExcludeType,
		Override:                  payload.Override,
		ProcessMode:               processMode,
	}

	if err := repo.UpdateProxyProviderConfig(r.Context(), config); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	config.CreatedAt = existing.CreatedAt
	config.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toProxyProviderConfigResponse(*config))
}

func handleDeleteProxyProviderConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	if err := repo.DeleteProxyProviderConfig(r.Context(), id, username); err != nil {
		if err.Error() == "proxy provider config not found or not owned by user" {
			writeError(w, http.StatusNotFound, errors.New("proxy provider config not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toProxyProviderConfigResponse(config storage.ProxyProviderConfig) proxyProviderConfigResponse {
	return proxyProviderConfigResponse{
		ID:                        config.ID,
		ExternalSubscriptionID:    config.ExternalSubscriptionID,
		Name:                      config.Name,
		Type:                      config.Type,
		Interval:                  config.Interval,
		Proxy:                     config.Proxy,
		SizeLimit:                 config.SizeLimit,
		Header:                    config.Header,
		HealthCheckEnabled:        config.HealthCheckEnabled,
		HealthCheckURL:            config.HealthCheckURL,
		HealthCheckInterval:       config.HealthCheckInterval,
		HealthCheckTimeout:        config.HealthCheckTimeout,
		HealthCheckLazy:           config.HealthCheckLazy,
		HealthCheckExpectedStatus: config.HealthCheckExpectedStatus,
		Filter:                    config.Filter,
		ExcludeFilter:             config.ExcludeFilter,
		ExcludeType:               config.ExcludeType,
		Override:                  config.Override,
		ProcessMode:               config.ProcessMode,
		CreatedAt:                 config.CreatedAt.Format(time.RFC3339),
		UpdatedAt:                 config.UpdatedAt.Format(time.RFC3339),
	}
}
