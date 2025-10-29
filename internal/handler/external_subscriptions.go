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

type externalSubscriptionRequest struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	UserAgent string `json:"user_agent"`
}

type externalSubscriptionResponse struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	URL        string  `json:"url"`
	UserAgent  string  `json:"user_agent"`
	NodeCount  int     `json:"node_count"`
	LastSyncAt *string `json:"last_sync_at"`
	Upload     int64   `json:"upload"`      // 已上传流量（字节）
	Download   int64   `json:"download"`    // 已下载流量（字节）
	Total      int64   `json:"total"`       // 总流量（字节）
	Expire     *string `json:"expire"`      // 过期时间
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

func NewExternalSubscriptionsHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("external subscriptions handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleListExternalSubscriptions(w, r, repo, username)
		case http.MethodPost:
			handleCreateExternalSubscription(w, r, repo, username)
		case http.MethodPut:
			handleUpdateExternalSubscription(w, r, repo, username)
		case http.MethodDelete:
			handleDeleteExternalSubscription(w, r, repo, username)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	})
}

func handleListExternalSubscriptions(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	subs, err := repo.ListExternalSubscriptions(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := make([]externalSubscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		var lastSyncAt *string
		if sub.LastSyncAt != nil {
			formatted := sub.LastSyncAt.Format(time.RFC3339)
			lastSyncAt = &formatted
		}

		var expire *string
		if sub.Expire != nil {
			formatted := sub.Expire.Format(time.RFC3339)
			expire = &formatted
		}

		resp = append(resp, externalSubscriptionResponse{
			ID:         sub.ID,
			Name:       sub.Name,
			URL:        sub.URL,
			UserAgent:  sub.UserAgent,
			NodeCount:  sub.NodeCount,
			LastSyncAt: lastSyncAt,
			Upload:     sub.Upload,
			Download:   sub.Download,
			Total:      sub.Total,
			Expire:     expire,
			CreatedAt:  sub.CreatedAt.Format(time.RFC3339),
			UpdatedAt:  sub.UpdatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleCreateExternalSubscription(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	var payload externalSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription name is required"))
		return
	}

	url := strings.TrimSpace(payload.URL)
	if url == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription url is required"))
		return
	}

	now := time.Now()
	sub := storage.ExternalSubscription{
		Username:   username,
		Name:       name,
		URL:        url,
		UserAgent:  payload.UserAgent, // 会在存储层使用默认值如果为空
		NodeCount:  0,
		LastSyncAt: &now,
	}

	id, err := repo.CreateExternalSubscription(r.Context(), sub)
	if err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionExists) {
			writeError(w, http.StatusConflict, errors.New("subscription with this URL already exists"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	created, err := repo.GetExternalSubscription(r.Context(), id, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var lastSyncAt *string
	if created.LastSyncAt != nil {
		formatted := created.LastSyncAt.Format(time.RFC3339)
		lastSyncAt = &formatted
	}

	var expire *string
	if created.Expire != nil {
		formatted := created.Expire.Format(time.RFC3339)
		expire = &formatted
	}

	resp := externalSubscriptionResponse{
		ID:         created.ID,
		Name:       created.Name,
		URL:        created.URL,
		UserAgent:  created.UserAgent,
		NodeCount:  created.NodeCount,
		LastSyncAt: lastSyncAt,
		Upload:     created.Upload,
		Download:   created.Download,
		Total:      created.Total,
		Expire:     expire,
		CreatedAt:  created.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  created.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleUpdateExternalSubscription(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid subscription id"))
		return
	}

	var payload externalSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription name is required"))
		return
	}

	url := strings.TrimSpace(payload.URL)
	if url == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription url is required"))
		return
	}

	existing, err := repo.GetExternalSubscription(r.Context(), id, username)
	if err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
			writeError(w, http.StatusNotFound, errors.New("subscription not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	sub := storage.ExternalSubscription{
		ID:         id,
		Username:   username,
		Name:       name,
		URL:        url,
		UserAgent:  payload.UserAgent, // 会在存储层使用默认值如果为空
		NodeCount:  existing.NodeCount,
		LastSyncAt: existing.LastSyncAt,
		Upload:     existing.Upload,
		Download:   existing.Download,
		Total:      existing.Total,
		Expire:     existing.Expire,
	}

	if err := repo.UpdateExternalSubscription(r.Context(), sub); err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
			writeError(w, http.StatusNotFound, errors.New("subscription not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	updated, err := repo.GetExternalSubscription(r.Context(), id, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var lastSyncAt *string
	if updated.LastSyncAt != nil {
		formatted := updated.LastSyncAt.Format(time.RFC3339)
		lastSyncAt = &formatted
	}

	var expire *string
	if updated.Expire != nil {
		formatted := updated.Expire.Format(time.RFC3339)
		expire = &formatted
	}

	resp := externalSubscriptionResponse{
		ID:         updated.ID,
		Name:       updated.Name,
		URL:        updated.URL,
		UserAgent:  updated.UserAgent,
		NodeCount:  updated.NodeCount,
		LastSyncAt: lastSyncAt,
		Upload:     updated.Upload,
		Download:   updated.Download,
		Total:      updated.Total,
		Expire:     expire,
		CreatedAt:  updated.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  updated.UpdatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleDeleteExternalSubscription(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("subscription id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid subscription id"))
		return
	}

	if err := repo.DeleteExternalSubscription(r.Context(), id, username); err != nil {
		if errors.Is(err, storage.ErrExternalSubscriptionNotFound) {
			writeError(w, http.StatusNotFound, errors.New("subscription not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
