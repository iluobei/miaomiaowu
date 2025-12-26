package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"miaomiaowu/internal/storage"
)

type setupStatusResponse struct {
	NeedsSetup bool `json:"needs_setup"`
}

type setupRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Nickname  string `json:"nickname"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type setupResponse struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
}

// NewSetupStatusHandler returns a handler that checks if initial setup is needed
func NewSetupStatusHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("setup status handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		users, err := repo.ListUsers(r.Context(), 1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		needsSetup := len(users) == 0

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(setupStatusResponse{NeedsSetup: needsSetup})
	})
}

// NewInitialSetupHandler handles the creation of the first admin user
func NewInitialSetupHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("initial setup handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// Check if setup is still needed
		users, err := repo.ListUsers(r.Context(), 1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if len(users) > 0 {
			writeError(w, http.StatusConflict, errors.New("系统已初始化，无法再次注册"))
			return
		}

		var payload setupRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		password := strings.TrimSpace(payload.Password)
		nickname := strings.TrimSpace(payload.Nickname)
		email := strings.TrimSpace(payload.Email)
		avatarURL := strings.TrimSpace(payload.AvatarURL)

		if username == "" {
			writeError(w, http.StatusBadRequest, errors.New("用户名不能为空"))
			return
		}

		if password == "" {
			writeError(w, http.StatusBadRequest, errors.New("密码不能为空"))
			return
		}

		if nickname == "" {
			nickname = username
		}

		// Hash the password
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// Create the admin user
		if err := repo.CreateUser(r.Context(), username, email, nickname, string(hash), storage.RoleAdmin, ""); err != nil {
			if errors.Is(err, storage.ErrUserExists) {
				writeError(w, http.StatusConflict, errors.New("用户已存在"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// Ensure the user is set as admin and active
		_ = repo.UpdateUserRole(r.Context(), username, storage.RoleAdmin)
		_ = repo.UpdateUserStatus(r.Context(), username, true)

		if avatarURL != "" || email != "" || nickname != "" {
			_ = repo.UpdateUserProfile(r.Context(), username, storage.UserProfileUpdate{
				Email:     email,
				Nickname:  nickname,
				AvatarURL: avatarURL,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(setupResponse{
			Username: username,
			Nickname: nickname,
			Email:    email,
		})
	})
}
