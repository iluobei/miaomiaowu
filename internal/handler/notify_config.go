package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/storage"
)

type notifyConfigResponse struct {
	NotifyEnabled          bool   `json:"notify_enabled"`
	TelegramBotToken       string `json:"telegram_bot_token"`
	TelegramChatID         string `json:"telegram_chat_id"`
	NotifySubscribeFetch   bool   `json:"notify_subscribe_fetch"`
	NotifyLogin            bool   `json:"notify_login"`
	NotifyIPBan            bool   `json:"notify_ip_ban"`
	NotifySilentMode       bool   `json:"notify_silent_mode"`
	NotifyDailyTraffic     bool   `json:"notify_daily_traffic"`
	NotifyExpiry           bool   `json:"notify_expiry"`
	NotifyNodeProbeOffline bool   `json:"notify_node_probe_offline"`
	NotifyNodeProbeOnline  bool   `json:"notify_node_probe_online"`
	NotifyDailyTrafficTime string `json:"notify_daily_traffic_time"`
}

type notifyConfigRequest struct {
	NotifyEnabled          bool   `json:"notify_enabled"`
	TelegramBotToken       string `json:"telegram_bot_token"`
	TelegramChatID         string `json:"telegram_chat_id"`
	NotifySubscribeFetch   bool   `json:"notify_subscribe_fetch"`
	NotifyLogin            bool   `json:"notify_login"`
	NotifyIPBan            bool   `json:"notify_ip_ban"`
	NotifySilentMode       bool   `json:"notify_silent_mode"`
	NotifyDailyTraffic     bool   `json:"notify_daily_traffic"`
	NotifyExpiry           bool   `json:"notify_expiry"`
	NotifyNodeProbeOffline bool   `json:"notify_node_probe_offline"`
	NotifyNodeProbeOnline  bool   `json:"notify_node_probe_online"`
	NotifyDailyTrafficTime string `json:"notify_daily_traffic_time"`
}

// NewNotifyConfigHandler creates a handler for notification configuration.
func NewNotifyConfigHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		// Check for /test suffix
		if strings.HasSuffix(r.URL.Path, "/test") && r.Method == http.MethodPost {
			handleNotifyTest(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleGetNotifyConfig(w, r, repo)
		case http.MethodPut:
			handleUpdateNotifyConfig(w, r, repo)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	})
}

func handleGetNotifyConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository) {
	sysCfg, err := repo.GetSystemConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Mask bot token for security (show only last 4 chars)
	maskedToken := sysCfg.TelegramBotToken
	if len(maskedToken) > 4 {
		maskedToken = strings.Repeat("*", len(maskedToken)-4) + maskedToken[len(maskedToken)-4:]
	}

	resp := notifyConfigResponse{
		NotifyEnabled:          sysCfg.NotifyEnabled,
		TelegramBotToken:       maskedToken,
		TelegramChatID:         sysCfg.TelegramChatID,
		NotifySubscribeFetch:   sysCfg.NotifySubscribeFetch,
		NotifyLogin:            sysCfg.NotifyLogin,
		NotifyIPBan:            sysCfg.NotifyIPBan,
		NotifySilentMode:       sysCfg.NotifySilentMode,
		NotifyDailyTraffic:     sysCfg.NotifyDailyTraffic,
		NotifyExpiry:           sysCfg.NotifyExpiry,
		NotifyNodeProbeOffline: sysCfg.NotifyNodeProbeOffline,
		NotifyNodeProbeOnline:  sysCfg.NotifyNodeProbeOnline,
		NotifyDailyTrafficTime: sysCfg.NotifyDailyTrafficTime,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleUpdateNotifyConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository) {
	var req notifyConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	sysCfg, err := repo.GetSystemConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// If token is masked (all stars + 4 chars), keep the existing token
	if req.TelegramBotToken != "" && !strings.Contains(req.TelegramBotToken, "*") {
		sysCfg.TelegramBotToken = req.TelegramBotToken
	}

	sysCfg.NotifyEnabled = req.NotifyEnabled
	sysCfg.TelegramChatID = req.TelegramChatID
	sysCfg.NotifySubscribeFetch = req.NotifySubscribeFetch
	sysCfg.NotifyLogin = req.NotifyLogin
	sysCfg.NotifyIPBan = req.NotifyIPBan
	sysCfg.NotifySilentMode = req.NotifySilentMode
	sysCfg.NotifyDailyTraffic = req.NotifyDailyTraffic
	sysCfg.NotifyExpiry = req.NotifyExpiry
	sysCfg.NotifyNodeProbeOffline = req.NotifyNodeProbeOffline
	sysCfg.NotifyNodeProbeOnline = req.NotifyNodeProbeOnline
	if req.NotifyDailyTrafficTime != "" {
		sysCfg.NotifyDailyTrafficTime = req.NotifyDailyTrafficTime
	}

	if err := repo.UpdateSystemConfig(r.Context(), sysCfg); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Hot-reload the global notifier
	if n := GetNotifier(); n != nil {
		n.UpdateConfig(notify.Config{
			Enabled:                sysCfg.NotifyEnabled,
			BotToken:               sysCfg.TelegramBotToken,
			ChatID:                 sysCfg.TelegramChatID,
			NotifySubscribeFetch:   sysCfg.NotifySubscribeFetch,
			NotifyLogin:            sysCfg.NotifyLogin,
			NotifyIPBan:            sysCfg.NotifyIPBan,
			NotifySilentMode:       sysCfg.NotifySilentMode,
			NotifyDailyTraffic:     sysCfg.NotifyDailyTraffic,
			NotifyExpiry:           sysCfg.NotifyExpiry,
			NotifyNodeProbeOffline: sysCfg.NotifyNodeProbeOffline,
			NotifyNodeProbeOnline:  sysCfg.NotifyNodeProbeOnline,
			DailyTrafficTime:       sysCfg.NotifyDailyTrafficTime,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	n := GetNotifier()
	if n == nil {
		writeError(w, http.StatusInternalServerError, errors.New("notifier not initialized"))
		return
	}

	if err := n.SendTest(r.Context()); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
