package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"miaomiaowu/internal/storage"
)

// 外部节点探测的管理 API。

type nodeProbeHandler struct {
	repo  *storage.TrafficRepository
	store *NodeProbeStore
}

func NewNodeProbeHandler(repo *storage.TrafficRepository, store *NodeProbeStore) http.Handler {
	return &nodeProbeHandler{repo: repo, store: store}
}

func (h *nodeProbeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/node-probe")
	path = strings.Trim(path, "/")

	switch {
	case path == "" && r.Method == http.MethodGet:
		h.handleStatus(w, r)
	case path == "settings" && r.Method == http.MethodPut:
		h.handleUpdateSettings(w, r)
	case path == "toggle" && r.Method == http.MethodPost:
		h.handleToggleNode(w, r)
	default:
		writeBadRequest(w, "不支持的路径或方法")
	}
}

// GET /api/admin/node-probe —— 总开关、探测源、各节点最近状态与历史。
func (h *nodeProbeHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	enabled, _ := h.repo.GetSystemSetting(ctx, settingNodeProbeEnabled)
	testerRaw, _ := h.repo.GetSystemSetting(ctx, settingNodeProbeTesterID)
	testerID, _ := strconv.ParseInt(strings.TrimSpace(testerRaw), 10, 64)
	resyncRaw, _ := h.repo.GetSystemSetting(ctx, settingNodeProbeResyncMinutes)
	resyncMinutes, _ := strconv.Atoi(strings.TrimSpace(resyncRaw))
	count, _ := h.repo.CountProbeEnabledNodes(ctx)

	// 内存 ring 的快照。节点多时这份 JSON 会比较大(每节点最多 288 个采样点),
	// 但探测是用户逐个勾选的,通常只有十几个。
	states := h.store.All()

	respondJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"enabled":        enabled == "1",
		"tester_id":      testerID,
		"resync_minutes": resyncMinutes,
		"enabled_count":  count,
		"interval_sec":   int(nodeProbeInterval.Seconds()),
		"states":         states,
	})
}

// PUT /api/admin/node-probe/settings —— 改总开关与探测源。
func (h *nodeProbeHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// 指针:nil = 本次不改这一项,避免只改探测源时把总开关冲掉
		// (服务器设置那边就吃过这个亏:值类型字段没提交即被零值覆盖)。
		Enabled  *bool  `json:"enabled"`
		TesterID *int64 `json:"tester_id"`
		// ResyncMinutes 节点掉线满 N 分钟自动重同步外部订阅;0 = 关闭。
		ResyncMinutes *int `json:"resync_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}
	ctx := r.Context()
	if req.Enabled != nil {
		v := "0"
		if *req.Enabled {
			v = "1"
		}
		if err := h.repo.SetSystemSetting(ctx, settingNodeProbeEnabled, v); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.TesterID != nil {
		if err := h.repo.SetSystemSetting(ctx, settingNodeProbeTesterID,
			strconv.FormatInt(*req.TesterID, 10)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if req.ResyncMinutes != nil {
		v := *req.ResyncMinutes
		if v < 0 {
			writeBadRequest(w, "重同步间隔不能为负")
			return
		}
		// 上限一天:再长就没有「掉线自愈」的意义了,更可能是误填。
		if v > 1440 {
			writeBadRequest(w, "重同步间隔最多 1440 分钟")
			return
		}
		if err := h.repo.SetSystemSetting(ctx, settingNodeProbeResyncMinutes,
			strconv.Itoa(v)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

// POST /api/admin/node-probe/toggle —— 开关单个节点的探测。
func (h *nodeProbeHandler) handleToggleNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID  int64 `json:"node_id"`
		Enabled bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}
	if req.NodeID <= 0 {
		writeBadRequest(w, "node_id 无效")
		return
	}
	if err := h.repo.SetNodeProbeEnabled(r.Context(), req.NodeID, req.Enabled); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// 关掉探测就把历史丢掉,否则面板上还挂着一条永远不再更新的旧曲线。
	if !req.Enabled {
		h.store.Forget(req.NodeID)
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}
