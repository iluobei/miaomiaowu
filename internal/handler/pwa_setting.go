package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"miaomiaowu/internal/storage"
)

// PWA(安装到桌面 + service worker 缓存)开关,全站生效,默认开。
//
//	GET /api/admin/system-settings/pwa      → {"enabled":true|false}
//	PUT /api/admin/system-settings/pwa      {"enabled":true|false}
//
// 前端不另外请求这个值:它随首屏 HTML 的 <meta name="mmw-pwa"> 下发(internal/web/pwa.go),
// 登录页在注册 SW 之前就知道该不该注册。

const settingPWAEnabled = "pwa_enabled"

// LoadPWAEnabled 读开关。没设过 = 开;只有明确存了 "false" 才算关。
func LoadPWAEnabled(ctx context.Context, repo *storage.TrafficRepository) bool {
	if repo == nil {
		return true
	}
	v, _ := repo.GetSystemSetting(ctx, settingPWAEnabled)
	return v != "false"
}

type PWASettingHandler struct {
	repo      *storage.TrafficRepository
	onChanged func(bool)
}

// NewPWASettingHandler onChanged 在保存成功后调用,用来同步首屏注入与 /sw.js(web.SetPWAEnabled)。
func NewPWASettingHandler(repo *storage.TrafficRepository, onChanged func(bool)) *PWASettingHandler {
	return &PWASettingHandler{repo: repo, onChanged: onChanged}
}

func (h *PWASettingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		respondJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": LoadPWAEnabled(r.Context(), h.repo)})
	case http.MethodPut:
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
			respondJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "enabled 必须是 true 或 false"})
			return
		}
		value := "false"
		if *req.Enabled {
			value = "true"
		}
		if err := h.repo.SetSystemSetting(r.Context(), settingPWAEnabled, value); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
			return
		}
		if h.onChanged != nil {
			h.onChanged(*req.Enabled)
		}
		respondJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": *req.Enabled})
	default:
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "method not allowed"})
	}
}
