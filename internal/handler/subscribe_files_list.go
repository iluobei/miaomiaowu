package handler

import (
	"net/http"

	"miaomiaowu/internal/storage"
)

type subscribeFilesListHandler struct {
	repo *storage.TrafficRepository
}

// NewSubscribeFilesListHandler returns a handler for listing subscribe files (for all authenticated users).
func NewSubscribeFilesListHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("subscribe files list handler requires repository")
	}

	return &subscribeFilesListHandler{
		repo: repo,
	}
}

func (h *subscribeFilesListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	files, err := h.repo.ListSubscribeFiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Convert to DTO format
	result := make([]subscribeFileDTO, 0, len(files))
	for _, file := range files {
		result = append(result, subscribeFileDTO{
			ID:          file.ID,
			Name:        file.Name,
			Description: file.Description,
			Type:        file.Type,
			Filename:    file.Filename,
			CreatedAt:   file.CreatedAt,
			UpdatedAt:   file.UpdatedAt,
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"files": result,
	})
}
