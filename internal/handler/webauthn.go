package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
)

// passkey(WebAuthn)。与 2FA 并存,互不取代:
//   - 密码 + 2FA 仍是兜底登录方式。passkey 绑定域名(RP ID 参与签名),换域名后全部失效,
//     没有兜底就等于一次域名迁移把所有人锁在门外。
//   - 注册时要求 UserVerification,所以一把 passkey 已等价"设备 + 本人",登录时不再叠 2FA。

// webAuthnUserFor 组装库要的 User:用户本身 + 他名下已注册的全部 passkey。
func webAuthnUserFor(r *http.Request, repo *storage.TrafficRepository, user storage.User) (auth.WebAuthnUser, error) {
	stored, err := repo.ListWebAuthnCredentials(r.Context(), user.Username)
	if err != nil {
		return auth.WebAuthnUser{}, err
	}
	creds := make([]webauthn.Credential, 0, len(stored))
	for _, s := range stored {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(s.Credential), &c); err != nil {
			// 单条坏数据不该让整个用户登不进去,跳过并留痕。
			logger.Info("[passkey] 凭据解析失败,已跳过", "username", user.Username, "credential_id", s.CredentialID, "error", err)
			continue
		}
		creds = append(creds, c)
	}
	display := user.Nickname
	if display == "" {
		display = user.Username
	}
	return auth.WebAuthnUser{Name: user.Username, DisplayName: display, Credentials: creds}, nil
}

func encodeCredentialID(id []byte) string { return base64.RawURLEncoding.EncodeToString(id) }

// NewPasskeyRegisterHandler 已登录用户注册一把新 passkey。
// POST /api/user/passkey/register/begin  → 返回 challenge
// POST /api/user/passkey/register/finish → 校验并落库
func NewPasskeyRegisterHandler(repo *storage.TrafficRepository, sessions *auth.WebAuthnSessionStore) http.Handler {
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
		wa, err := auth.WebAuthnFor(r, "")
		if err != nil {
			writeBadRequest(w, err.Error())
			return
		}
		waUser, err := webAuthnUserFor(r, repo, user)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/begin") {
			// excludeCredentials 让浏览器提前挡住"同一把钥匙重复注册",
			// 否则要等落库时才被唯一约束拒掉,用户已经白按了一次指纹。
			exclusions := make([]protocol.CredentialDescriptor, 0, len(waUser.Credentials))
			for i := range waUser.Credentials {
				exclusions = append(exclusions, waUser.Credentials[i].Descriptor())
			}
			opts := append(auth.WebAuthnRegistrationOptions(), webauthn.WithExclusions(exclusions))
			creation, sessionData, err := wa.BeginRegistration(waUser, opts...)
			if err != nil {
				writeBadRequest(w, "发起 passkey 注册失败: "+err.Error())
				return
			}
			token, err := sessions.Issue(sessionData, username, false)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"session": token, "options": creation.Response})
			return
		}

		// finish:session token 走 query,body 保持为浏览器产出的原始 attestation,
		// 好让库直接读 r.Body。
		sessionToken := r.URL.Query().Get("session")
		sessionData, sessionUser, _, ok := sessions.Consume(sessionToken)
		if !ok || sessionUser != username {
			writeError(w, http.StatusBadRequest, errors.New("注册会话已过期,请重试"))
			return
		}
		credential, err := wa.FinishRegistration(waUser, sessionData, r)
		if err != nil {
			writeBadRequest(w, "passkey 注册校验失败: "+err.Error())
			return
		}

		raw, err := json.Marshal(credential)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			name = "Passkey"
		}
		if err := repo.CreateWebAuthnCredential(r.Context(), storage.WebAuthnCredential{
			Username:     username,
			CredentialID: encodeCredentialID(credential.ID),
			Credential:   string(raw),
			Name:         name,
			AAGUID:       base64.RawURLEncoding.EncodeToString(credential.Authenticator.AAGUID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		logger.Info("[passkey] 注册成功", "username", username, "name", name)
		respondJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
}

// NewPasskeyManageHandler 列表 / 改名 / 删除。
// GET|PUT|DELETE /api/user/passkey
func NewPasskeyManageHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		switch r.Method {
		case http.MethodGet:
			list, err := repo.ListWebAuthnCredentials(r.Context(), username)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"passkeys": list})
		case http.MethodPut:
			var body struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeBadRequest(w, "请求格式不正确")
				return
			}
			name := strings.TrimSpace(body.Name)
			if name == "" {
				writeBadRequest(w, "名称不能为空")
				return
			}
			if err := repo.RenameWebAuthnCredential(r.Context(), username, body.ID, name); err != nil {
				writeBadRequest(w, err.Error())
				return
			}
			respondJSON(w, http.StatusOK, map[string]bool{"success": true})
		case http.MethodDelete:
			id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
			if err != nil {
				writeBadRequest(w, "缺少 id")
				return
			}
			if err := repo.DeleteWebAuthnCredential(r.Context(), username, id); err != nil {
				writeBadRequest(w, err.Error())
				return
			}
			logger.Info("[passkey] 已删除", "username", username, "id", id)
			respondJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("unsupported method"))
		}
	})
}

// NewPasskeyLoginHandler 无密码登录。
// POST /api/login/passkey/begin  → 返回 challenge(不需要用户名)
// POST /api/login/passkey/finish → 校验并签发登录态
// 校验的是认证器签名而非口令,不存在弱口令爆破面;但 finish 仍接入登录限流器,
// 与密码登录同口径记录成功/失败,免得成为绕过限流的探测入口。
func NewPasskeyLoginHandler(tokens *auth.TokenStore, repo *storage.TrafficRepository, sessions *auth.WebAuthnSessionStore, rateLimiter *LoginRateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}
		wa, err := auth.WebAuthnFor(r, "")
		if err != nil {
			writeBadRequest(w, err.Error())
			return
		}

		if strings.HasSuffix(r.URL.Path, "/begin") {
			var body struct {
				RememberMe bool `json:"remember_me"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			// Discoverable(可发现)登录:不传用户名,浏览器直接列出本机可用的 passkey
			// 让用户选。这是 passkey 相对 2FA 的核心体验差别。
			assertion, sessionData, err := wa.BeginDiscoverableLogin()
			if err != nil {
				writeBadRequest(w, "发起 passkey 登录失败: "+err.Error())
				return
			}
			token, err := sessions.Issue(sessionData, "", body.RememberMe)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"session": token, "options": assertion.Response})
			return
		}

		clientIP := GetClientIP(r)
		// finish 前先过 IP 限流(此时还不知道用户名)
		if rateLimiter != nil {
			if err := rateLimiter.Check(clientIP, ""); err != nil {
				writeError(w, http.StatusTooManyRequests, errors.New("too many login attempts, please try again later"))
				return
			}
		}

		// session token 走 query,body 保持为浏览器产出的原始 assertion。
		sessionToken := r.URL.Query().Get("session")
		sessionData, _, rememberMe, ok := sessions.Consume(sessionToken)
		if !ok {
			writeError(w, http.StatusBadRequest, errors.New("登录会话已过期,请重试"))
			return
		}

		// handler 把浏览器回传的 credential id 换成用户。userHandle 我们签的是
		// sha256(username),不可逆,所以一律按 credential id 反查。
		var matched storage.User
		discoverable := func(rawID, _ []byte) (webauthn.User, error) {
			stored, err := repo.GetWebAuthnCredentialByID(r.Context(), encodeCredentialID(rawID))
			if err != nil {
				return nil, err
			}
			if stored == nil {
				return nil, errors.New("passkey 未注册")
			}
			user, err := repo.GetUser(r.Context(), stored.Username)
			if err != nil {
				return nil, err
			}
			matched = user
			return webAuthnUserFor(r, repo, user)
		}

		_, credential, err := wa.FinishPasskeyLogin(discoverable, sessionData, r)
		if err != nil || matched.Username == "" {
			if rateLimiter != nil {
				rateLimiter.RecordFailure(clientIP, matched.Username)
			}
			logger.Warn("🔐 [LOGIN_FAIL] passkey 登录校验失败",
				"username", matched.Username,
				"client_ip", clientIP,
				"error", err,
				"time", time.Now().Format("2006-01-02 15:04:05"))
			writeError(w, http.StatusUnauthorized, errors.New("passkey 校验失败"))
			return
		}
		if !matched.IsActive {
			writeError(w, http.StatusForbidden, errors.New("账号已被禁用"))
			return
		}

		// 登录成功,清除速率限制计数
		if rateLimiter != nil {
			rateLimiter.RecordSuccess(clientIP, matched.Username)
		}

		// 回写 sign count(克隆检测依据)与 last_used_at。失败不阻断登录 —— 人已经验过了。
		if raw, err := json.Marshal(credential); err == nil {
			if err := repo.UpdateWebAuthnCredentialAfterLogin(r.Context(), encodeCredentialID(credential.ID), string(raw)); err != nil {
				logger.Info("[passkey] 回写凭据状态失败", "username", matched.Username, "error", err)
			}
		}
		if _, err := repo.GetOrCreateUserToken(r.Context(), matched.Username); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		logger.Info("[passkey] 登录成功", "username", matched.Username)
		issueLoginSession(w, r, tokens, repo, matched, rememberMe)
	})
}
