package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"miaomiaowu/internal/storage"
)

type userSubscriptionsHandler struct {
	repo *storage.TrafficRepository
}

// NewUserSubscriptionsHandler creates a handler for managing user-subscription assignments (admin only).
// Supports:
// - GET /api/admin/users/{username}/subscriptions - get user's subscription IDs
// - PUT /api/admin/users/{username}/subscriptions - update user's subscriptions
func NewUserSubscriptionsHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user subscriptions handler requires repository")
	}

	return &userSubscriptionsHandler{repo: repo}
}

func (h *userSubscriptionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract username from path: /api/admin/users/{username}/subscriptions
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "subscriptions" {
		writeError(w, http.StatusNotFound, errors.New("invalid path"))
		return
	}
	username := parts[0]
	if username == "" {
		writeError(w, http.StatusBadRequest, errors.New("username is required"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, username)
	case http.MethodPut:
		h.handleUpdate(w, r, username)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (h *userSubscriptionsHandler) handleGet(w http.ResponseWriter, r *http.Request, username string) {
	subscriptionIDs, err := h.repo.GetUserSubscriptionIDs(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"subscription_ids": subscriptionIDs,
	})
}

func (h *userSubscriptionsHandler) handleUpdate(w http.ResponseWriter, r *http.Request, username string) {
	var req struct {
		SubscriptionIDs []int64 `json:"subscription_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body")
		return
	}

	// 获取所有有效的订阅ID（使用subscribe_files表）
	allSubscriptions, err := h.repo.ListSubscribeFiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	validIDs := make(map[int64]bool)
	for _, sub := range allSubscriptions {
		validIDs[sub.ID] = true
	}

	// 过滤掉无效的订阅ID（已删除的订阅）
	filteredIDs := make([]int64, 0, len(req.SubscriptionIDs))
	for _, id := range req.SubscriptionIDs {
		if id > 0 && validIDs[id] {
			filteredIDs = append(filteredIDs, id)
		}
	}

	if err := h.repo.SetUserSubscriptions(r.Context(), username, filteredIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "user subscriptions updated successfully",
	})
}
