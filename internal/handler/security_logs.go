package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

// SecurityLogHandler 提供安全日志（探测/封禁事件流 + 当前封禁列表）查询与封禁管理，admin 专用。
//
// 路由（在 main.go 注册，全部 RequireAdmin 包裹）：
//
//	GET    /api/admin/security/events?kind=&ip=&limit=&offset=  事件流（后端分页）
//	GET    /api/admin/security/bans                             当前生效封禁列表
//	POST   /api/admin/security/bans   {ip, permanent}           手动封禁 / 提升为永久
//	DELETE /api/admin/security/bans/{ip}                        解封
type SecurityLogHandler struct {
	repo *storage.TrafficRepository
}

func NewSecurityLogHandler(repo *storage.TrafficRepository) *SecurityLogHandler {
	return &SecurityLogHandler{repo: repo}
}

func (h *SecurityLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/security/")

	switch {
	case path == "events" && r.Method == http.MethodGet:
		h.handleEvents(w, r)
	case path == "bans" && r.Method == http.MethodGet:
		h.handleListBans(w, r)
	case path == "bans" && r.Method == http.MethodPost:
		h.handleCreateBan(w, r)
	case strings.HasPrefix(path, "bans/") && r.Method == http.MethodDelete:
		h.handleUnban(w, r, strings.TrimPrefix(path, "bans/"))
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (h *SecurityLogHandler) handleEvents(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	limit := atoiDefault(r.URL.Query().Get("limit"), 200)
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)

	events, err := h.repo.ListSecurityEvents(r.Context(), kind, ip, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if events == nil {
		events = []storage.SecurityEvent{}
	}
	respondJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *SecurityLogHandler) handleListBans(w http.ResponseWriter, r *http.Request) {
	bans, err := h.repo.ListActiveIPBans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if bans == nil {
		bans = []storage.IPBan{}
	}
	respondJSON(w, http.StatusOK, map[string]any{"bans": bans})
}

func (h *SecurityLogHandler) handleCreateBan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP        string `json:"ip"`
		Permanent bool   `json:"permanent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeBadRequest(w, "invalid body")
		return
	}
	ip := strings.TrimSpace(body.IP)
	if net.ParseIP(ip) == nil {
		writeBadRequest(w, "invalid ip address")
		return
	}
	p := GetBruteForceProtector()
	if p == nil {
		writeError(w, http.StatusServiceUnavailable, nil)
		return
	}
	p.BanIP(ip, body.Permanent, auth.UsernameFromContext(r.Context()))
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SecurityLogHandler) handleUnban(w http.ResponseWriter, r *http.Request, ip string) {
	ip = strings.TrimSpace(ip)
	if net.ParseIP(ip) == nil {
		writeBadRequest(w, "invalid ip address")
		return
	}
	p := GetBruteForceProtector()
	if p == nil {
		writeError(w, http.StatusServiceUnavailable, nil)
		return
	}
	p.UnbanIP(ip, auth.UsernameFromContext(r.Context()))
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return def
}
