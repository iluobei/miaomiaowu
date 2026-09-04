package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"miaomiaowu/internal/auth"
)

func TestOperationAuditRecordsAdminMutation(t *testing.T) {
	repo := relayTestRepo(t)
	tokens := auth.NewTokenStore(time.Hour)
	token, _, err := tokens.Issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := OperationAuditMiddleware(next, repo, tokens)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/example", nil)
	req.Header.Set(auth.AuthHeader, token)
	req.RemoteAddr = "203.0.113.20:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logs, err := repo.ListOperationLogs(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Actor != "admin" || logs[0].Status != http.StatusNoContent || logs[0].Path != "/api/admin/example" {
		t.Fatalf("unexpected operation logs: %+v", logs)
	}
}
