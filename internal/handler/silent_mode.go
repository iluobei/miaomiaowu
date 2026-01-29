package handler

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
)

var globalSilentModeManager *SilentModeManager

type SilentModeManager struct {
	repo           *storage.TrafficRepository
	tokens         *auth.TokenStore
	lastActiveTime sync.Map   // username -> time.Time
	startTime      time.Time  // 服务启动时间，用于启动后临时恢复
}

func NewSilentModeManager(repo *storage.TrafficRepository, tokens *auth.TokenStore) *SilentModeManager {
	m := &SilentModeManager{
		repo:      repo,
		tokens:    tokens,
		startTime: time.Now(),
	}
	globalSilentModeManager = m
	logger.Info("🔓 [SILENT_MODE] 服务启动，静默模式临时恢复中",
		"start_time", m.startTime.Format("2006-01-02 15:04:05"),
	)
	return m
}

func GetSilentModeManager() *SilentModeManager {
	return globalSilentModeManager
}

func (m *SilentModeManager) RecordSubscriptionAccess(username string) {
	if username == "" {
		return
	}
	m.lastActiveTime.Store(username, time.Now())
	logger.Info("🔓 [SILENT_MODE] 用户获取订阅，恢复访问权限",
		"username", username,
		"time", time.Now().Format("2006-01-02 15:04:05"),
	)
}

func (m *SilentModeManager) isUserActive(username string, timeout int) bool {
	if username == "" {
		return false
	}

	val, ok := m.lastActiveTime.Load(username)
	if !ok {
		return false
	}

	lastActive := val.(time.Time)
	activeUntil := lastActive.Add(time.Duration(timeout) * time.Minute)
	return time.Now().Before(activeUntil)
}

// contoken获取用户名
func (m *SilentModeManager) extractUsername(r *http.Request) string {
	if m.tokens == nil {
		return ""
	}

	if token := strings.TrimSpace(r.Header.Get(auth.AuthHeader)); token != "" {
		if username, ok := m.tokens.Lookup(token); ok {
			return username
		}
	}

	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		if username, ok := m.tokens.Lookup(token); ok {
			return username
		}
	}

	return ""
}

// 触发
func isAllowedPath(path string) bool {
	// 订阅相关接口始终可访问
	allowedPrefixes := []string{
		"/api/clash/subscribe",
		"/api/proxy-provider/",
		"/t/", // 临时订阅
	}

	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	// 短链接处理 (6位字母数字字符，如 /AbC123)
	trimmedPath := strings.Trim(path, "/")
	if len(trimmedPath) == 6 && isAlphanumericPath(trimmedPath) {
		return true
	}

	return false
}

func isAlphanumericPath(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func (m *SilentModeManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg, err := m.repo.GetSystemConfig(context.Background())
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		if !cfg.SilentMode {
			next.ServeHTTP(w, r)
			return
		}

		// 服务启动后的恢复期内，允许所有请求
		recoveryUntil := m.startTime.Add(time.Duration(cfg.SilentModeTimeout) * time.Minute)
		if time.Now().Before(recoveryUntil) {
			next.ServeHTTP(w, r)
			return
		}

		if isAllowedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		username := m.extractUsername(r)

		if username != "" && m.isUserActive(username, cfg.SilentModeTimeout) {
			next.ServeHTTP(w, r)
			return
		}

		logger.Info("🔒 [SILENT_MODE] 请求被拦截",
			"path", r.URL.Path,
			"username", username,
			"client_ip", getClientIP(r),
		)
		w.Header().Set("X-Silent-Mode", "true")
		http.NotFound(w, r)
	})
}
