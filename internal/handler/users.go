package handler

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type userEntry struct {
	Username            string `json:"username"`
	Email               string `json:"email"`
	Nickname            string `json:"nickname"`
	Avatar              string `json:"avatar_url"`
	Role                string `json:"role"`
	IsActive            bool   `json:"is_active"`
	Remark              string `json:"remark"`
	CustomUserShortCode string `json:"custom_user_short_code,omitempty"`
}

type userStatusRequest struct {
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type userResetRequest struct {
	Username    string `json:"username"`
	NewPassword string `json:"new_password"`
}

type userResetResponse struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userCreateRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Remark   string `json:"remark"`
}

type userCreateResponse struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

func NewUserListHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user list handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		users, err := repo.ListUsers(r.Context(), 1000)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		entries := make([]userEntry, 0, len(users))
		for _, user := range users {
			customCode, _ := repo.GetUserCustomShortCode(r.Context(), user.Username)
			entries = append(entries, userEntry{
				Username:            user.Username,
				Email:               user.Email,
				Nickname:            user.Nickname,
				Avatar:              user.AvatarURL,
				Role:                user.Role,
				IsActive:            user.IsActive,
				Remark:              user.Remark,
				CustomUserShortCode: customCode,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"users": entries})
	})
}

func NewUserStatusHandler(repo *storage.TrafficRepository, store *auth.TokenStore) http.Handler {
	if repo == nil {
		panic("user status handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload userStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		if username == "" {
			writeError(w, http.StatusBadRequest, errors.New("username is required"))
			return
		}

		// Check if target user is admin
		targetUser, err := repo.GetUser(r.Context(), username)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if targetUser.Role == storage.RoleAdmin {
			writeError(w, http.StatusBadRequest, errors.New("不能修改管理员状态"))
			return
		}

		if err := repo.UpdateUserStatus(r.Context(), username, payload.IsActive); err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// [安全] 停用用户后立即吊销其全部会话 —— RequireToken 只查内存不复查用户状态,
		// 不吊销的话被停用的用户 token 仍能一直用到过期。
		if !payload.IsActive && store != nil {
			store.RevokeByUsername(username)
			_ = repo.DeleteUserSessions(r.Context(), username)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
	})
}

func NewUserResetPasswordHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user reset handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload userResetRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		if username == "" {
			writeError(w, http.StatusBadRequest, errors.New("username is required"))
			return
		}

		// Check if target user is admin
		targetUser, err := repo.GetUser(r.Context(), username)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if targetUser.Role == storage.RoleAdmin {
			writeError(w, http.StatusBadRequest, errors.New("不能重置管理员密码"))
			return
		}

		newPassword := strings.TrimSpace(payload.NewPassword)
		if newPassword == "" {
			generated, err := generateRandomPassword(12)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			newPassword = generated
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if err := repo.UpdateUserPassword(r.Context(), username, string(hash)); err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userResetResponse{Username: username, Password: newPassword})
	})
}

func NewUserCreateHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user create handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload userCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		email := strings.TrimSpace(payload.Email)
		nickname := strings.TrimSpace(payload.Nickname)
		password := strings.TrimSpace(payload.Password)
		remark := strings.TrimSpace(payload.Remark)

		if username == "" {
			writeError(w, http.StatusBadRequest, errors.New("username is required"))
			return
		}

		if password == "" {
			random, err := generateRandomPassword(12)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			password = random
		}
		if nickname == "" {
			nickname = username
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// New users are created as regular users, not admins
		role := storage.RoleUser

		if err := repo.CreateUser(r.Context(), username, email, nickname, string(hash), role, remark); err != nil {
			if errors.Is(err, storage.ErrUserExists) {
				writeError(w, http.StatusConflict, errors.New("用户已存在"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userCreateResponse{
			Username: username,
			Email:    email,
			Nickname: nickname,
			Role:     role,
			Password: password,
		})
	})
}

type userDeleteRequest struct {
	Username string `json:"username"`
}

func NewUserDeleteHandler(repo *storage.TrafficRepository, store *auth.TokenStore) http.Handler {
	if repo == nil {
		panic("user delete handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload userDeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		if username == "" {
			writeError(w, http.StatusBadRequest, errors.New("username is required"))
			return
		}

		// Check if target user is admin
		targetUser, err := repo.GetUser(r.Context(), username)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if targetUser.Role == storage.RoleAdmin {
			writeError(w, http.StatusBadRequest, errors.New("不能删除管理员账号"))
			return
		}

		if err := repo.DeleteUser(r.Context(), username); err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// [安全] 删除用户后立即吊销其全部会话,否则已登录的 token 仍能访问 /api/user/* 到过期。
		if store != nil {
			store.RevokeByUsername(username)
			_ = repo.DeleteUserSessions(r.Context(), username)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	})
}

func generateRandomPassword(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if length <= 0 {
		length = 12
	}
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i, b := range bytes {
		bytes[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(bytes), nil
}

type userRemarkRequest struct {
	Username string `json:"username"`
	Remark   string `json:"remark"`
}

func NewUserRemarkHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user remark handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload userRemarkRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		if username == "" {
			writeError(w, http.StatusBadRequest, errors.New("username is required"))
			return
		}

		if err := repo.UpdateUserRemark(r.Context(), username, payload.Remark); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
	})
}

type userCustomShortCodeRequest struct {
	Username        string `json:"username"`
	CustomShortCode string `json:"custom_short_code"`
}

func NewUserCustomShortCodeHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user custom short code handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload userCustomShortCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		if username == "" {
			writeError(w, http.StatusBadRequest, errors.New("username is required"))
			return
		}

		code := strings.TrimSpace(payload.CustomShortCode)

		// Validate: alphanumeric only
		for _, c := range code {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				writeError(w, http.StatusBadRequest, errors.New("自定义连接只能包含字母和数字"))
				return
			}
		}

		// 同表唯一性：不能与其他用户的 user_short_code 或 custom_user_short_code 冲突
		if code != "" {
			userCodes, err := repo.GetAllUserShortCodes(r.Context())
			if err == nil {
				if un, exists := userCodes[code]; exists && un != username {
					writeError(w, http.StatusConflict, errors.New("该自定义连接已被其他用户使用"))
					return
				}
			}
		}

		if err := repo.UpdateUserCustomShortCode(r.Context(), username, code); err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusConflict, errors.New(err.Error()))
			return
		}

		if m := GetSilentModeManager(); m != nil {
			m.InvalidateShortLinkCache()
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
	})
}
