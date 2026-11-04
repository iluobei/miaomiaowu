package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"miaomiaowu/internal/auth"
)

func TestExternalSubscriptionNamePrefixAPI(t *testing.T) {
	repo := relayTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateUser(ctx, "bob", "", "", "x", "user", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := NewExternalSubscriptionsHandler(repo)
	// 127.0.0.1 会被 SSRF 客户端拒绝，创建接口仍会落库，测试不依赖外网。
	const subURL = "http://127.0.0.1:1/sub"

	createBody := `{"name":"机场A","url":"` + subURL + `","name_prefix":"〖机场A〗","auto_update":true,"update_interval_minutes":30}`
	rec := serveExternalSub(t, h, http.MethodPost, "/api/user/external-subscriptions", createBody, "bob")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d body %s", rec.Code, rec.Body.String())
	}
	created := decodeExternalSub(t, rec)
	if created.NamePrefix != "〖机场A〗" {
		t.Fatalf("created prefix = %q", created.NamePrefix)
	}
	stored, err := repo.GetExternalSubscription(ctx, created.ID, "bob")
	if err != nil {
		t.Fatalf("get created: %v", err)
	}
	stored.AppliedNamePrefix = "【aa】"
	if err := repo.UpdateExternalSubscription(ctx, stored); err != nil {
		t.Fatalf("set applied prefix: %v", err)
	}
	if !created.AutoUpdate || created.UpdateIntervalMinutes != 30 {
		t.Fatalf("auto update not stored: %+v", created)
	}
	updateURL := "/api/user/external-subscriptions?id=" + strconv.FormatInt(created.ID, 10)

	// 流量统计切换这类请求不带 name_prefix，不能把前缀清掉。
	updateBody := `{"name":"机场A","url":"` + subURL + `","traffic_mode":"download"}`
	rec = serveExternalSub(t, h, http.MethodPut, updateURL, updateBody, "bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("update status %d body %s", rec.Code, rec.Body.String())
	}
	updated := decodeExternalSub(t, rec)
	if updated.NamePrefix != "〖机场A〗" {
		t.Fatalf("omitted prefix was wiped: %q", updated.NamePrefix)
	}
	reloaded, err := repo.GetExternalSubscription(ctx, created.ID, "bob")
	if err != nil {
		t.Fatalf("reload after omit: %v", err)
	}
	if reloaded.AppliedNamePrefix != "【aa】" {
		t.Fatalf("editing the subscription wiped the applied prefix: %q", reloaded.AppliedNamePrefix)
	}
	if updated.TrafficMode != "download" {
		t.Fatalf("traffic mode = %q", updated.TrafficMode)
	}

	clearBody := `{"name":"机场A","url":"` + subURL + `","name_prefix":""}`
	rec = serveExternalSub(t, h, http.MethodPut, updateURL, clearBody, "bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("clear status %d body %s", rec.Code, rec.Body.String())
	}
	cleared := decodeExternalSub(t, rec)
	if cleared.NamePrefix != "" {
		t.Fatalf("cleared prefix = %q", cleared.NamePrefix)
	}

	spaceBody := `{"name":"机场A","url":"` + subURL + `","name_prefix":"[机场A] "}`
	rec = serveExternalSub(t, h, http.MethodPut, updateURL, spaceBody, "bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("space prefix status %d body %s", rec.Code, rec.Body.String())
	}
	spaced := decodeExternalSub(t, rec)
	if spaced.NamePrefix != "[机场A] " {
		t.Fatalf("trailing space prefix = %q", spaced.NamePrefix)
	}
	reloaded, err = repo.GetExternalSubscription(ctx, created.ID, "bob")
	if err != nil {
		t.Fatalf("reload after prefix edit: %v", err)
	}
	if reloaded.NamePrefix != "[机场A] " || reloaded.AppliedNamePrefix != "【aa】" {
		t.Fatalf("prefix edit = name %q applied %q", reloaded.NamePrefix, reloaded.AppliedNamePrefix)
	}

	rec = serveExternalSub(t, h, http.MethodGet, "/api/user/external-subscriptions", "", "bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d body %s", rec.Code, rec.Body.String())
	}
	var listed []externalSubscriptionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 || listed[0].NamePrefix != "[机场A] " {
		t.Fatalf("list = %+v", listed)
	}
}

func serveExternalSub(t *testing.T, h http.Handler, method, target, body, username string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req = req.WithContext(auth.ContextWithUsername(context.Background(), username))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeExternalSub(t *testing.T, rec *httptest.ResponseRecorder) externalSubscriptionResponse {
	t.Helper()
	var resp externalSubscriptionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body %s", err, rec.Body.String())
	}
	return resp
}
