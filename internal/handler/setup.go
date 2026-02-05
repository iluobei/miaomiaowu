package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"miaomiaowu/internal/logger"
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
		logger.Info("[初始化检查] 收到初始化状态检查请求",
			"method", r.Method,
			"remote_addr", r.RemoteAddr,
		)

		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		users, err := repo.ListUsers(r.Context(), 10)
		if err != nil {
			logger.Error("[初始化检查] 查询用户列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		needsSetup := len(users) == 0

		// 详细记录用户列表信息
		if len(users) > 0 {
			usernames := make([]string, len(users))
			for i, u := range users {
				usernames[i] = u.Username
			}
			logger.Info("[初始化检查] 数据库中已存在用户",
				"user_count", len(users),
				"usernames", usernames,
				"needs_setup", needsSetup,
			)
		} else {
			logger.Info("[初始化检查] 数据库中没有用户，需要初始化",
				"user_count", 0,
				"needs_setup", needsSetup,
			)
		}

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
		logger.Info("[初始化] 收到初始化请求",
			"method", r.Method,
			"remote_addr", r.RemoteAddr,
		)

		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// Check if setup is still needed
		users, err := repo.ListUsers(r.Context(), 1)
		if err != nil {
			logger.Error("[初始化] 查询用户列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if len(users) > 0 {
			logger.Warn("[初始化] 系统已初始化，拒绝重复初始化",
				"existing_user_count", len(users),
				"first_user", users[0].Username,
			)
			writeError(w, http.StatusConflict, errors.New("系统已初始化，无法再次注册"))
			return
		}

		var payload setupRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			logger.Error("[初始化] 解析请求体失败", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username := strings.TrimSpace(payload.Username)
		password := strings.TrimSpace(payload.Password)
		nickname := strings.TrimSpace(payload.Nickname)
		email := strings.TrimSpace(payload.Email)
		avatarURL := strings.TrimSpace(payload.AvatarURL)

		logger.Info("[初始化] 准备创建管理员用户",
			"username", username,
			"nickname", nickname,
			"email", email,
		)

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
			logger.Error("[初始化] 密码哈希失败", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// Create the admin user
		if err := repo.CreateUser(r.Context(), username, email, nickname, string(hash), storage.RoleAdmin, ""); err != nil {
			if errors.Is(err, storage.ErrUserExists) {
				logger.Warn("[初始化] 用户已存在", "username", username)
				writeError(w, http.StatusConflict, errors.New("用户已存在"))
				return
			}
			logger.Error("[初始化] 创建用户失败", "username", username, "error", err)
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

		logger.Info("[初始化] 管理员用户创建成功",
			"username", username,
			"nickname", nickname,
			"role", storage.RoleAdmin,
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(setupResponse{
			Username: username,
			Nickname: nickname,
			Email:    email,
		})
	})
}
