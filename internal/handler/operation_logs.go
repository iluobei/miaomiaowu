package handler

import (
	"net/http"
	"strconv"

	"miaomiaowu/internal/storage"
)

func NewOperationLogHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		logs, err := repo.ListOperationLogs(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if logs == nil {
			logs = []storage.OperationLog{}
		}
		respondJSON(w, http.StatusOK, map[string]any{"logs": logs})
	})
}
