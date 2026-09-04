package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func GenerateTOTPKey(username, issuer string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
}

func ValidateTOTPCode(secret, code string) bool {
	return totp.Validate(code, secret)
}

func GenerateRecoveryCodes(count int) (plain, hashed []string, err error) {
	plain = make([]string, count)
	hashed = make([]string, count)
	for i := range count {
		buf := make([]byte, 6)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		code := strings.ToLower(hex.EncodeToString(buf))[:8]
		plain[i] = code
		h := sha256.Sum256([]byte(code))
		hashed[i] = hex.EncodeToString(h[:])
	}
	return plain, hashed, nil
}

func ValidateRecoveryCode(code string, hashedCodes []string) (bool, []string) {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(code))))
	target := hex.EncodeToString(h[:])
	for i, hc := range hashedCodes {
		if hc == target {
			remaining := make([]string, 0, len(hashedCodes)-1)
			remaining = append(remaining, hashedCodes[:i]...)
			remaining = append(remaining, hashedCodes[i+1:]...)
			return true, remaining
		}
	}
	return false, hashedCodes
}

type twoFactorPending struct {
	username   string
	rememberMe bool
	expiry     time.Time
	attempts   int
}

// maxTwoFactorAttempts 单个 2FA 待验证令牌允许的错误次数上限。超过即作废该令牌,
// 迫使攻击者重新走密码登录(而登录接口是有限流的)—— 防止在 5 分钟窗口内无限爆破 6 位 TOTP。
const maxTwoFactorAttempts = 5

type TwoFactorPendingStore struct {
	mu      sync.RWMutex
	pending map[string]twoFactorPending
	ttl     time.Duration
}

func NewTwoFactorPendingStore(ttl time.Duration) *TwoFactorPendingStore {
	return &TwoFactorPendingStore{
		pending: make(map[string]twoFactorPending),
		ttl:     ttl,
	}
}

func (s *TwoFactorPendingStore) Issue(username string, rememberMe bool) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)

	s.mu.Lock()
	s.pending[token] = twoFactorPending{
		username:   username,
		rememberMe: rememberMe,
		expiry:     time.Now().Add(s.ttl),
	}
	s.mu.Unlock()
	return token, nil
}

func (s *TwoFactorPendingStore) Validate(token string) (username string, rememberMe bool, ok bool) {
	s.mu.RLock()
	p, exists := s.pending[token]
	s.mu.RUnlock()
	if !exists || time.Now().After(p.expiry) {
		return "", false, false
	}
	return p.username, p.rememberMe, true
}

func (s *TwoFactorPendingStore) Consume(token string) {
	s.mu.Lock()
	delete(s.pending, token)
	s.mu.Unlock()
}

// RecordFailure 记一次该令牌的验证失败。达到上限即作废该令牌并返回 true(表示已锁定,
// 调用方应让用户重新登录)。
func (s *TwoFactorPendingStore) RecordFailure(token string) (locked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, exists := s.pending[token]
	if !exists {
		return true
	}
	p.attempts++
	if p.attempts >= maxTwoFactorAttempts {
		delete(s.pending, token)
		return true
	}
	s.pending[token] = p
	return false
}
