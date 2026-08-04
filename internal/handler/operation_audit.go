package handler

import (
	"context"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func OperationAuditMiddleware(next http.Handler, repo *storage.TrafficRepository, tokens *auth.TokenStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutating := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete
		if !mutating || !strings.HasPrefix(r.URL.Path, "/api/admin/") {
			next.ServeHTTP(w, r)
			return
		}
		recorder := &auditResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		actor, _ := tokens.Lookup(strings.TrimSpace(r.Header.Get(auth.AuthHeader)))
		_ = repo.InsertOperationLog(context.Background(), storage.OperationLog{Actor: actor, Method: r.Method, Path: r.URL.Path, Status: recorder.status, IP: GetClientIP(r)})
	})
}
