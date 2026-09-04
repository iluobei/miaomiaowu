package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/storage"
)

func NewTwoFactorLoginHandler(tokens *auth.TokenStore, repo *storage.TrafficRepository, tfStore *auth.TwoFactorPendingStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload struct {
			TwoFactorToken string `json:"two_factor_token"`
			Code           string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username, rememberMe, ok := tfStore.Validate(payload.TwoFactorToken)
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("invalid or expired 2FA token"))
			return
		}

		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if !auth.ValidateTOTPCode(user.TOTPSecret, strings.TrimSpace(payload.Code)) {
			// [安全] 限制爆破:达到上限即作废该 2FA 令牌,迫使重新走(有限流的)密码登录。
			if tfStore.RecordFailure(payload.TwoFactorToken) {
				writeError(w, http.StatusUnauthorized, errors.New("尝试次数过多,请重新登录"))
				return
			}
			writeError(w, http.StatusUnauthorized, errors.New("invalid 2FA code"))
			return
		}

		tfStore.Consume(payload.TwoFactorToken)
		issueLoginSession(w, r, tokens, repo, user, rememberMe)
	})
}

func NewRecoveryLoginHandler(tokens *auth.TokenStore, repo *storage.TrafficRepository, tfStore *auth.TwoFactorPendingStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var payload struct {
			TwoFactorToken string `json:"two_factor_token"`
			RecoveryCode   string `json:"recovery_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		username, rememberMe, ok := tfStore.Validate(payload.TwoFactorToken)
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("invalid or expired 2FA token"))
			return
		}

		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		hashedCodes, err := parseRecoveryCodes(user.RecoveryCodes)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		valid, _ := auth.ValidateRecoveryCode(payload.RecoveryCode, hashedCodes)
		if !valid {
			// [安全] 同 2FA:限制爆破,达到上限作废令牌。
			if tfStore.RecordFailure(payload.TwoFactorToken) {
				writeError(w, http.StatusUnauthorized, errors.New("尝试次数过多,请重新登录"))
				return
			}
			writeError(w, http.StatusUnauthorized, errors.New("invalid recovery code"))
			return
		}

		tfStore.Consume(payload.TwoFactorToken)

		if err := repo.DisableUserTOTP(r.Context(), username); err != nil {
			logger.Warn("[2FA] 恢复码重设失败", "username", username, "error", err)
		}

		issueLoginSession(w, r, tokens, repo, user, rememberMe)
	})
}

func NewTwoFactorStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		username := auth.UsernameFromContext(r.Context())
		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{
			"enabled": user.TOTPEnabled,
		})
	})
}

func NewTwoFactorSetupHandler(manager *auth.Manager, repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		username := auth.UsernameFromContext(r.Context())

		var payload struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if err := manager.ValidatePassword(r.Context(), username, payload.Password); err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("invalid password"))
			return
		}

		key, err := auth.GenerateTOTPKey(username, "妙妙屋")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if err := repo.SetUserTOTPSecret(r.Context(), username, key.Secret()); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"secret": key.Secret(),
			"url":    key.URL(),
		})
	})
}

func NewTwoFactorVerifySetupHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		username := auth.UsernameFromContext(r.Context())
		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if user.TOTPSecret == "" {
			writeError(w, http.StatusBadRequest, errors.New("2FA setup not initiated"))
			return
		}

		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if !auth.ValidateTOTPCode(user.TOTPSecret, strings.TrimSpace(payload.Code)) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid 2FA code"))
			return
		}

		plain, hashed, err := auth.GenerateRecoveryCodes(8)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		hashedJSON, _ := json.Marshal(hashed)
		if err := repo.EnableUserTOTP(r.Context(), username, string(hashedJSON)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]string{
			"recovery_codes": plain,
		})
	})
}

func NewTwoFactorDisableHandler(manager *auth.Manager, repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		username := auth.UsernameFromContext(r.Context())
		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if !user.TOTPEnabled {
			writeError(w, http.StatusBadRequest, errors.New("2FA is not enabled"))
			return
		}

		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if !auth.ValidateTOTPCode(user.TOTPSecret, strings.TrimSpace(payload.Code)) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid 2FA code"))
			return
		}

		if err := repo.DisableUserTOTP(r.Context(), username); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "disabled"})
	})
}

func issueLoginSession(w http.ResponseWriter, r *http.Request, tokens *auth.TokenStore, repo *storage.TrafficRepository, user storage.User, rememberMe bool) {
	var ttl time.Duration
	if rememberMe {
		ttl = 30 * 24 * time.Hour
	} else {
		ttl = 24 * time.Hour
	}

	token, expiry, err := tokens.IssueWithTTL(user.Username, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if repo != nil {
		if err := repo.CreateSession(r.Context(), token, user.Username, expiry); err != nil {
			logger.Warn("[认证] 会话持久化失败", "username", user.Username, "error", err)
		}
	}

	clientIP := GetClientIP(r)
	logger.Info("🔐 [LOGIN_OK] 登录成功",
		"username", user.Username,
		"client_ip", clientIP,
		"remember_me", rememberMe,
		"expires_at", expiry.Format("2006-01-02 15:04:05"))

	if n := GetNotifier(); n != nil {
		go n.Send(r.Context(), notify.Event{
			Type:    notify.EventLogin,
			Title:   "登录通知",
			Message: fmt.Sprintf("用户 `%s` 登录成功\nIP: `%s`", user.Username, clientIP),
		})
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
}

func parseRecoveryCodes(raw string) ([]string, error) {
	var codes []string
	if err := json.Unmarshal([]byte(raw), &codes); err != nil {
		return nil, fmt.Errorf("parse recovery codes: %w", err)
	}
	return codes, nil
}
