package handler

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type subscribeFileDownloadHandler struct {
	repo        *storage.TrafficRepository
	subscribeDir string
}

// NewSubscribeFileDownloadHandler returns a handler for downloading subscribe files with token authentication.
func NewSubscribeFileDownloadHandler(repo *storage.TrafficRepository, subscribeDir string) http.Handler {
	if repo == nil {
		panic("subscribe file download handler requires repository")
	}

	return &subscribeFileDownloadHandler{
		repo:        repo,
		subscribeDir: subscribeDir,
	}
}

func (h *subscribeFileDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
		return
	}

	// Extract filename from URL path
	path := strings.TrimPrefix(r.URL.Path, "/subscribes/")
	path = strings.TrimPrefix(path, "/")
	filename := filepath.Base(path)

	if filename == "" || filename == "." || filename == ".." {
		writeError(w, http.StatusBadRequest, errors.New("invalid filename"))
		return
	}

	// Validate file extension
	ext := filepath.Ext(filename)
	if ext != ".yaml" && ext != ".yml" {
		writeError(w, http.StatusBadRequest, errors.New("only YAML files are supported"))
		return
	}

	// Authenticate using token from query parameter
	queryToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if queryToken == "" {
		writeError(w, http.StatusUnauthorized, errors.New("token is required"))
		return
	}

	username, err := h.repo.ValidateUserToken(r.Context(), queryToken)
	if err != nil {
		if errors.Is(err, storage.ErrTokenNotFound) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid token"))
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	// Set username in context for potential future use
	ctx := auth.ContextWithUsername(r.Context(), username)
	r = r.WithContext(ctx)

	// Read the file
	cleanedFilename := filepath.Clean(filename)
	if strings.HasPrefix(cleanedFilename, "..") {
		writeError(w, http.StatusBadRequest, errors.New("invalid filename"))
		return
	}

	filePath := filepath.Join(h.subscribeDir, cleanedFilename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, errors.New("file not found"))
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
