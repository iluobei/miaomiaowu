package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type session struct {
	username string
	expiry   time.Time
}

type contextKey string

const (
	userContextKey contextKey = "miaomiaowu/auth/username"
)

const AuthHeader = "MM-Authorization"

type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]session
	ttl    time.Duration
}

func NewTokenStore(ttl time.Duration) *TokenStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &TokenStore{
		tokens: make(map[string]session),
		ttl:    ttl,
	}
}

func (s *TokenStore) Issue(username string) (string, time.Time, error) {
	return s.IssueWithTTL(username, s.ttl)
}

// IssueWithTTL creates a new token for the specified username with a custom TTL.
func (s *TokenStore) IssueWithTTL(username string, ttl time.Duration) (string, time.Time, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", time.Time{}, errors.New("username is required")
	}

	if ttl <= 0 {
		ttl = s.ttl
	}

	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}

	expiry := time.Now().Add(ttl)

	s.mu.Lock()
	s.tokens[token] = session{username: username, expiry: expiry}
	s.mu.Unlock()

	return token, expiry, nil
}

func (s *TokenStore) Validate(token string) bool {
	_, ok := s.Lookup(token)
	return ok
}

func (s *TokenStore) Revoke(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

func (s *TokenStore) RevokeAll() {
	s.mu.Lock()
	s.tokens = make(map[string]session)
	s.mu.Unlock()
}

// LoadSession adds a session to the in-memory store. Used to restore sessions from database on startup.
func (s *TokenStore) LoadSession(token, username string, expiry time.Time) {
	token = strings.TrimSpace(token)
	username = strings.TrimSpace(username)
	if token == "" || username == "" {
		return
	}

	// Skip expired sessions
	if time.Now().After(expiry) {
		return
	}

	s.mu.Lock()
	s.tokens[token] = session{username: username, expiry: expiry}
	s.mu.Unlock()
}

// UpdateUsername rewrites in-memory sessions from oldUsername to newUsername.
func (s *TokenStore) UpdateUsername(oldUsername, newUsername string) {
	oldUsername = strings.TrimSpace(oldUsername)
	newUsername = strings.TrimSpace(newUsername)
	if oldUsername == "" || newUsername == "" || oldUsername == newUsername {
		return
	}

	s.mu.Lock()
	for token, sess := range s.tokens {
		if sess.username == oldUsername {
			s.tokens[token] = session{username: newUsername, expiry: sess.expiry}
		}
	}
	s.mu.Unlock()
}

// Lookup returns the username associated with the provided token if the session is valid.
func (s *TokenStore) Lookup(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.tokens[token]
	if !ok {
		return "", false
	}

	if time.Now().After(session.expiry) {
		delete(s.tokens, token)
		return "", false
	}

	return session.username, true
}

func ContextWithUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, userContextKey, username)
}

// UsernameFromContext retrieves the authenticated username from the request context.
func UsernameFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	username, _ := ctx.Value(userContextKey).(string)
	return username
}

// UsernameOrDefault returns the username if present, otherwise returns the provided fallback value.
func UsernameOrDefault(ctx context.Context, fallback string) string {
	if name := UsernameFromContext(ctx); name != "" {
		return name
	}
	return fallback
}

func randomToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func RequireToken(store *TokenStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.Header.Get(AuthHeader))
		if username, ok := store.Lookup(token); ok {
			ctx := ContextWithUsername(r.Context(), username)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		WriteUnauthorizedResponse(w)
	})
}

// UserRepository provides user information for authorization checks.
type UserRepository interface {
	GetUser(ctx context.Context, username string) (User, error)
}

// User represents basic user information needed for authorization.
type User struct {
	Username string
	Role     string
	IsActive bool
}

// RequireAdmin ensures the authenticated user has admin role.
func RequireAdmin(store *TokenStore, repo UserRepository, next http.Handler) http.Handler {
	return RequireToken(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := UsernameFromContext(r.Context())
		if username == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
			return
		}

		user, err := repo.GetUser(r.Context(), username)
		if err != nil || user.Role != "admin" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
			return
		}

		next.ServeHTTP(w, r)
	}))
}

func WriteUnauthorizedResponse(w http.ResponseWriter) {
	if w == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code": http.StatusUnauthorized,
		"msg":  "无效凭据",
	})
}
