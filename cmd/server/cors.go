package main

import (
	"net/http"
	"os"
	"strings"
)

func getAllowedOrigins() []string {
	raw := os.Getenv("ALLOWED_ORIGINS")
	if raw == "" {
		raw = os.Getenv("FRONTEND_ORIGIN")
	}
	return parseAllowedOrigins(raw)
}

func parseAllowedOrigins(raw string) []string {
	if raw == "" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	var origins []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	if len(origins) == 0 {
		return []string{"*"}
	}
	return origins
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	if next == nil {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}
	allowAll := len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowAll || originAllowed(origin, allowedOrigins) {
				setCORSHeaders(w, origin, allowAll)
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func originAllowed(origin string, allowed []string) bool {
	for _, o := range allowed {
		if o == origin {
			return true
		}
	}
	return false
}

func setCORSHeaders(w http.ResponseWriter, origin string, allowAll bool) {
	if allowAll {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		if existing := w.Header().Get("Vary"); existing == "" {
			w.Header().Set("Vary", "Origin")
		} else if !strings.Contains(existing, "Origin") {
			w.Header().Set("Vary", existing+", Origin")
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, MM-Authorization, Content-Type")
	w.Header().Set("Access-Control-Expose-Headers", "X-Silent-Mode")
}
