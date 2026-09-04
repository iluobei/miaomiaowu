package auth

import (
	"testing"
	"time"
)

// 安全 M1:停用/删除用户时按用户名吊销会话,该用户全部 token 立即失效,其他用户不受影响。
func TestTokenStore_RevokeByUsername(t *testing.T) {
	s := NewTokenStore(time.Hour)

	tok1, _, err := s.Issue("bob")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	tok2, _, _ := s.Issue("bob")
	tokAlice, _, _ := s.Issue("alice")

	s.RevokeByUsername("bob")

	if _, ok := s.Lookup(tok1); ok {
		t.Error("bob 的 token1 吊销后仍有效")
	}
	if _, ok := s.Lookup(tok2); ok {
		t.Error("bob 的 token2 吊销后仍有效")
	}
	if _, ok := s.Lookup(tokAlice); !ok {
		t.Error("alice 的会话不应被吊销")
	}
}
