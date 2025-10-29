package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type userSettingsRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

type userSettingsResponse struct {
	Profile profileResponse `json:"profile"`
}

func NewUserSettingsHandler(repo *storage.TrafficRepository, tokens *auth.TokenStore) http.Handler {
	if repo == nil || tokens == nil {
		panic("user settings handler requires repository and token store")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only PUT is supported"))
			return
		}

		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		var payload userSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		currentUser, err := repo.GetUser(r.Context(), username)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusUnauthorized, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		desiredUsername := strings.TrimSpace(payload.Username)
		if desiredUsername == "" {
			desiredUsername = username
		}

		if desiredUsername != username {
			if currentUser.Role == storage.RoleAdmin {
				writeError(w, http.StatusBadRequest, errors.New("管理员用户名不可修改"))
				return
			}

			if err := repo.RenameUser(r.Context(), username, desiredUsername); err != nil {
				lower := strings.ToLower(err.Error())
				if strings.Contains(lower, "unique") {
					writeError(w, http.StatusConflict, errors.New("用户名已存在"))
					return
				}
				writeError(w, http.StatusInternalServerError, err)
				return
			}

			tokens.UpdateUsername(username, desiredUsername)
			username = desiredUsername
		}

		update := storage.UserProfileUpdate{
			Email:     payload.Email,
			Nickname:  payload.Nickname,
			AvatarURL: payload.AvatarURL,
		}

		if err := repo.UpdateUserProfile(r.Context(), username, update); err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		resp := profileResponse{
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Avatar:   user.AvatarURL,
			Role:     user.Role,
			IsAdmin:  user.Role == storage.RoleAdmin,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(userSettingsResponse{Profile: resp})
	})
}
