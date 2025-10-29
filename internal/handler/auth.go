package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar_url"`
	Role      string    `json:"role"`
	IsAdmin   bool      `json:"is_admin"`
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func NewLoginHandler(manager *auth.Manager, tokens *auth.TokenStore, repo *storage.TrafficRepository) http.Handler {
	if manager == nil || tokens == nil {
		panic("login handler requires manager and token store")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload loginRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if strings.TrimSpace(payload.Username) == "" || payload.Password == "" {
			writeError(w, http.StatusBadRequest, errors.New("username and password are required"))
			return
		}

		username := strings.TrimSpace(payload.Username)
		clientIP := getClientIP(r)

		ok, err := manager.Authenticate(r.Context(), username, payload.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if !ok {
			// Log login failure
			now := time.Now().Format("2006-01-02 15:04:05")
			log.Printf("%s - %s - %s - 登录失败", clientIP, now, username)
			writeError(w, http.StatusUnauthorized, errors.New("invalid credentials"))
			return
		}

		user, err := manager.User(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if repo != nil {
			if _, err := repo.GetOrCreateUserToken(r.Context(), username); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}

		// Determine token TTL based on remember_me flag
		var ttl time.Duration
		if payload.RememberMe {
			ttl = 30 * 24 * time.Hour // 1 month
		} else {
			ttl = 24 * time.Hour // 1 day (default)
		}

		token, expiry, err := tokens.IssueWithTTL(username, ttl)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// Persist session to database if repo is available
		if repo != nil {
			if err := repo.CreateSession(r.Context(), token, username, expiry); err != nil {
				log.Printf("Failed to persist session: %v", err)
				// Don't fail the login, just log the error
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(loginResponse{
			Token:     token,
			ExpiresAt: expiry,
			Username:  user.Username,
			Email:     user.Email,
			Nickname:  user.Nickname,
			Avatar:    user.AvatarURL,
			Role:      user.Role,
			IsAdmin:   user.Role == storage.RoleAdmin,
		})
	})
}

func NewCredentialsHandler(manager *auth.Manager, tokens *auth.TokenStore) http.Handler {
	if manager == nil || tokens == nil {
		panic("credentials handler requires manager and token store")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only PUT is supported"))
			return
		}

		var payload credentialsRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		trimmedUsername := strings.TrimSpace(payload.Username)

		if trimmedUsername == "" && payload.Password == "" {
			writeError(w, http.StatusBadRequest, errors.New("username or password must be provided"))
			return
		}

		if err := manager.Update(r.Context(), trimmedUsername, payload.Password); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		tokens.RevokeAll()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
	})
}
