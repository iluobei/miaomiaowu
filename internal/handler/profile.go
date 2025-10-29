package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type profileResponse struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar_url"`
	Role     string `json:"role"`
	IsAdmin  bool   `json:"is_admin"`
}

var errUnauthorized = errors.New("unauthorized")

func NewProfileHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("profile handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if username == "" {
			writeError(w, http.StatusUnauthorized, errUnauthorized)
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
		_ = json.NewEncoder(w).Encode(resp)
	})
}
