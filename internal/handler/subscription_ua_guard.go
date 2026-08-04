package handler

import (
	"errors"
	"net/http"
	"sync/atomic"
)

var blockUnknownSubscriptionUA atomic.Bool

func SetBlockUnknownSubscriptionUA(enabled bool) {
	blockUnknownSubscriptionUA.Store(enabled)
}

func rejectBlockedSubscriptionUA(w http.ResponseWriter, r *http.Request) bool {
	if !blockUnknownSubscriptionUA.Load() || detectClientTypeFromUA(r.Header.Get("User-Agent")) != "" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	writeError(w, http.StatusForbidden, errors.New("仅允许受支持的代理客户端获取订阅"))
	return true
}
