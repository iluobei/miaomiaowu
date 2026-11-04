package handler

import (
	"encoding/json"
	"errors"
	"net/http"
)

// GET  /api/admin/nodes/hidden            → {"ids": [...]}
// POST /api/admin/nodes/hidden {ids, hidden} → 批量隐藏 / 取消隐藏
// 隐藏只影响节点管理列表的显示,订阅与下发不受影响。
func (h *nodesHandler) handleHiddenNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		ids, err := h.repo.ListHiddenNodeIDs(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"ids": ids})
		return
	}
	var req struct {
		IDs    []int64 `json:"ids"`
		Hidden bool    `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("请求格式错误"))
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("未选择节点"))
		return
	}
	n, err := h.repo.SetNodesHidden(r.Context(), req.IDs, req.Hidden)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"updated": n})
}
