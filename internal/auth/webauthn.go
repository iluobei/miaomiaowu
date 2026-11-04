package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// ErrWebAuthnInsecureContext 面板跑在裸 IP 或 http 上时返回。
//
// WebAuthn 只在 secure context(https,或 localhost)下可用 —— 浏览器侧
// navigator.credentials 直接不存在,不是降级。后端这里同样拒绝,免得前端拿到一份
// 永远用不了的 challenge、把失败推迟到看不懂的浏览器报错上。
var ErrWebAuthnInsecureContext = errors.New("passkey 需要通过域名 + HTTPS 访问面板才能使用")

// WebAuthnFor 构造这次仪式用的 RP。
//
// 按**当前请求的 Host** 推导 RP ID,不写进配置:面板域名不是固定的(可迁域名、用户侧可
// 独立部署),写死就意味着每次换域名都要改配置。跟着当前域名走,也与 WebAuthn 自身的
// 同源约束一致。代价是 passkey 不跨域名 —— 换域名后旧的全部失效,所以密码与 2FA 必须
// 保留为兜底。
func WebAuthnFor(r *http.Request, displayName string) (*webauthn.WebAuthn, error) {
	host := r.Host
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	if hostname == "" {
		return nil, ErrWebAuthnInsecureContext
	}

	// loopback 例外(spec 允许 localhost 用 http)只在**直连**时成立。经过反代时 Host
	// 很可能就是回源地址 127.0.0.1,那并不代表客户端在本机。
	behindProxy := r.Header.Get("X-Forwarded-Proto") != "" ||
		r.Header.Get("X-Forwarded-Host") != ""

	scheme := "https"
	if isLoopbackHost(hostname) && !behindProxy {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	// 非 loopback 的域名一律按 https 处理,不再强求 X-Forwarded-Proto。
	//
	// 浏览器只在 secure context 下才有 navigator.credentials,所以这个请求本身就证明了
	// 客户端侧是 https;后端再判一次,只会在反代没按约定加头时误伤(实测 Cloudflare +
	// nginx 的组合下主控就收不到该头)。真正必须挡住的是下面那条:RP ID 不能是裸 IP。

	// RP ID 必须是**可注册域名**:裸 IP 不行,反代改写出来的 upstream 名(常含下划线)
	// 也不行。nginx 若没写 proxy_set_header Host $host,转发过来的 Host 会是 upstream 名
	// (如 mmw_2ha_me),这里直接拒绝,前端会拿到明确的报错而不是浏览器的一句含糊错误。
	if !(isLoopbackHost(hostname) && !behindProxy) && !isRegistrableDomain(hostname) {
		return nil, ErrWebAuthnInsecureContext
	}

	if displayName == "" {
		displayName = "妙妙屋"
	}

	return webauthn.New(&webauthn.Config{
		RPID:          hostname,
		RPDisplayName: displayName,
		RPOrigins:     []string{scheme + "://" + host},
	})
}

// isRegistrableDomain 判断能否用作 RP ID。
//
// 排除三种:裸 IP(spec 明确不允许)、不含点的单标签(localhost 除外,它有 spec 例外)、
// 含下划线等非法字符的(nginx 改写出的 upstream 名就长这样)。
func isRegistrableDomain(hostname string) bool {
	if hostname == "" {
		return false
	}
	if net.ParseIP(hostname) != nil {
		return false
	}
	if !strings.Contains(hostname, ".") {
		return false
	}
	for _, r := range hostname {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-'
		if !isAllowed {
			return false
		}
	}
	return true
}

func isLoopbackHost(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// WebAuthnUser 把一个用户 + 他名下的 passkey 适配成库要的 webauthn.User。
type WebAuthnUser struct {
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
}

// WebAuthnID 用户句柄。
//
// 取 username 的 SHA-256 而不是自增 ID:这个值会被存进用户的认证器里(可能是别人的
// iCloud 钥匙串),换主控/换库都不该让它变;用自增 ID 则在恢复备份、迁移主控后可能错位。
// 用户名本身不适合直接当句柄 —— spec 上限 64 字节,且它是"可显示信息",不该外泄进句柄。
func (u WebAuthnUser) WebAuthnID() []byte {
	sum := sha256.Sum256([]byte(u.Name))
	return sum[:]
}

func (u WebAuthnUser) WebAuthnName() string { return u.Name }

func (u WebAuthnUser) WebAuthnDisplayName() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Name
}

func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

// WebAuthnSessionStore 存放一次仪式(注册或登录)进行中的 SessionData。
//
// 与 TwoFactorPendingStore 同构:进程内、带 TTL、一次性消费。challenge 天然是短命的
// 一次性值,没有落库的必要;主控重启导致进行中的仪式失效是可接受的(用户重点一次即可)。
type WebAuthnSessionStore struct {
	mu       sync.Mutex
	sessions map[string]webauthnSession
	ttl      time.Duration
}

type webauthnSession struct {
	data       webauthn.SessionData
	username   string
	rememberMe bool
	expiry     time.Time
}

func NewWebAuthnSessionStore(ttl time.Duration) *WebAuthnSessionStore {
	return &WebAuthnSessionStore{sessions: make(map[string]webauthnSession), ttl: ttl}
}

// Issue 存一份 SessionData,返回交给前端带回来的一次性 token。
func (s *WebAuthnSessionStore) Issue(data *webauthn.SessionData, username string, rememberMe bool) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)

	s.mu.Lock()
	s.pruneLocked()
	s.sessions[token] = webauthnSession{
		data:       *data,
		username:   username,
		rememberMe: rememberMe,
		expiry:     time.Now().Add(s.ttl),
	}
	s.mu.Unlock()
	return token, nil
}

// Consume 取出并立即删除。一次性:重放同一个 token 拿不到第二次。
func (s *WebAuthnSessionStore) Consume(token string) (data webauthn.SessionData, username string, rememberMe bool, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, exists := s.sessions[token]
	if !exists {
		return webauthn.SessionData{}, "", false, false
	}
	delete(s.sessions, token)
	if time.Now().After(sess.expiry) {
		return webauthn.SessionData{}, "", false, false
	}
	return sess.data, sess.username, sess.rememberMe, true
}

// pruneLocked 顺手清过期项。仪式量很小,不值得单开一个清理 goroutine。
func (s *WebAuthnSessionStore) pruneLocked() {
	now := time.Now()
	for token, sess := range s.sessions {
		if now.After(sess.expiry) {
			delete(s.sessions, token)
		}
	}
}

// WebAuthnRegistrationOptions 注册时的统一选项。
//
//   - ResidentKeyRequirementRequired:passkey 的核心,凭据存在认证器里,登录时不必先输
//     用户名(discoverable login)。不要求的话退化成"还得先报上名来"的普通二次因素。
//   - UserVerificationRequired:要求本地生物识别 / PIN,这样一把 passkey 本身就等于
//     "你有这台设备 + 你是本人",可以直接当完整登录凭据用,不必再叠密码。
func WebAuthnRegistrationOptions() []webauthn.RegistrationOption {
	return []webauthn.RegistrationOption{
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		}),
	}
}
