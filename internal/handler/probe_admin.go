package handler

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/storage"
)

type probeConfigHandler struct {
	repo *storage.TrafficRepository
}

type probeServerPayload struct {
	ID                  int64   `json:"id"`
	ServerID            string  `json:"server_id"`
	Name                string  `json:"name"`
	TrafficMethod       string  `json:"traffic_method"`
	MonthlyTrafficGB    float64 `json:"monthly_traffic_gb"`
	MonthlyTrafficBytes int64   `json:"monthly_traffic_bytes"`
	Position            int     `json:"position"`
}

type probeConfigPayload struct {
	ProbeType string               `json:"probe_type"`
	Address   string               `json:"address"`
	Servers   []probeServerPayload `json:"servers"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}

type probeConfigUpdateRequest struct {
	ProbeType string `json:"probe_type"`
	Address   string `json:"address"`
	Servers   []struct {
		ServerID         string  `json:"server_id"`
		Name             string  `json:"name"`
		TrafficMethod    string  `json:"traffic_method"`
		MonthlyTrafficGB float64 `json:"monthly_traffic_gb"`
	} `json:"servers"`
}

func NewProbeConfigHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("probe config handler requires repository")
	}

	return &probeConfigHandler{repo: repo}
}

func (h *probeConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPut:
		h.handleUpdate(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (h *probeConfigHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.repo.GetProbeConfig(r.Context())
	if err != nil {
		if errors.Is(err, storage.ErrProbeConfigNotFound) {
			// Return empty config instead of 404 when not configured yet
			respondJSON(w, http.StatusOK, map[string]any{
				"config": probeConfigPayload{
					ProbeType: "nezha",
					Address:   "",
					Servers:   []probeServerPayload{},
				},
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"config": convertProbeConfigResponse(cfg),
	})
}

func (h *probeConfigHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var payload probeConfigUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeBadRequest(w, "请求数据格式错误")
		return
	}

	type sanitizedServer struct {
		ServerID            string
		Name                string
		TrafficMethod       string
		MonthlyTrafficBytes int64
	}

	probeType := strings.ToLower(strings.TrimSpace(payload.ProbeType))
	if _, ok := getAllowedProbeTypes()[probeType]; !ok {
		writeBadRequest(w, "不支持的探针类型")
		return
	}

	address := strings.TrimSpace(payload.Address)
	if address == "" {
		writeBadRequest(w, "探针地址不能为空")
		return
	}

	if len(payload.Servers) == 0 {
		writeBadRequest(w, "请至少配置一个服务器")
		return
	}

	allowedMethods := getAllowedTrafficMethods()
	sanitized := make([]sanitizedServer, 0, len(payload.Servers))
	for idx, srv := range payload.Servers {
		serverID := strings.TrimSpace(srv.ServerID)
		if serverID == "" {
			writeBadRequest(w, formatServerError(idx, "服务器 ID 不能为空"))
			return
		}

		name := strings.TrimSpace(srv.Name)
		if name == "" {
			writeBadRequest(w, formatServerError(idx, "服务器名称不能为空"))
			return
		}

		method := strings.ToLower(strings.TrimSpace(srv.TrafficMethod))
		if _, ok := allowedMethods[method]; !ok {
			writeBadRequest(w, formatServerError(idx, "不支持的流量计算方式"))
			return
		}

		if srv.MonthlyTrafficGB < 0 {
			writeBadRequest(w, formatServerError(idx, "月流量不能为负数"))
			return
		}

		monthlyBytes := int64(math.Round(srv.MonthlyTrafficGB * bytesPerGigabyte))
		if monthlyBytes < 0 {
			monthlyBytes = 0
		}

		sanitized = append(sanitized, sanitizedServer{
			ServerID:            serverID,
			Name:                name,
			TrafficMethod:       method,
			MonthlyTrafficBytes: monthlyBytes,
		})
	}

	servers := make([]storage.ProbeServer, 0, len(sanitized))
	for _, srv := range sanitized {
		servers = append(servers, storage.ProbeServer{
			ServerID:            srv.ServerID,
			Name:                srv.Name,
			TrafficMethod:       srv.TrafficMethod,
			MonthlyTrafficBytes: srv.MonthlyTrafficBytes,
		})
	}

	updated, err := h.repo.UpsertProbeConfig(r.Context(), storage.ProbeConfig{
		ProbeType: probeType,
		Address:   address,
		Servers:   servers,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"config": convertProbeConfigResponse(updated),
	})
}

func convertProbeConfigResponse(cfg storage.ProbeConfig) probeConfigPayload {
	servers := make([]probeServerPayload, 0, len(cfg.Servers))
	for _, srv := range cfg.Servers {
		gb := float64(srv.MonthlyTrafficBytes) / bytesPerGigabyte
		gb = math.Round(gb*100) / 100
		servers = append(servers, probeServerPayload{
			ID:                  srv.ID,
			ServerID:            srv.ServerID,
			Name:                srv.Name,
			TrafficMethod:       srv.TrafficMethod,
			MonthlyTrafficGB:    gb,
			MonthlyTrafficBytes: srv.MonthlyTrafficBytes,
			Position:            srv.Position,
		})
	}

	return probeConfigPayload{
		ProbeType: cfg.ProbeType,
		Address:   cfg.Address,
		Servers:   servers,
		CreatedAt: cfg.CreatedAt,
		UpdatedAt: cfg.UpdatedAt,
	}
}

func formatServerError(idx int, message string) string {
	return message + " (行" + strconv.Itoa(idx+1) + ")"
}

func getAllowedProbeTypes() map[string]struct{} {
	return map[string]struct{}{
		storage.ProbeTypeNezha:   {},
		storage.ProbeTypeNezhaV0: {},
		storage.ProbeTypeDstatus: {},
		storage.ProbeTypeKomari:  {},
	}
}

func getAllowedTrafficMethods() map[string]struct{} {
	return map[string]struct{}{
		storage.TrafficMethodUp:   {},
		storage.TrafficMethodDown: {},
		storage.TrafficMethodBoth: {},
	}
}
