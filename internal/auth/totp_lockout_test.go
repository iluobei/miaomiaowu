package auth

import (
	"testing"
	"time"
)

// 安全 M2:2FA/恢复码验证达到失败上限后,待验证令牌立即作废(迫使重新登录),防止无限爆破。
func TestTwoFactorPendingStore_LockoutAfterMaxAttempts(t *testing.T) {
	s := NewTwoFactorPendingStore(time.Minute)
	tok, err := s.Issue("bob", false)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	// 上限前的失败:不锁定,令牌仍有效。
	for i := 0; i < maxTwoFactorAttempts-1; i++ {
		if locked := s.RecordFailure(tok); locked {
			t.Fatalf("第 %d 次失败就锁定了,过早", i+1)
		}
		if _, _, ok := s.Validate(tok); !ok {
			t.Fatalf("第 %d 次失败后令牌不应失效", i+1)
		}
	}

	// 第 maxTwoFactorAttempts 次失败:锁定 + 令牌作废。
	if locked := s.RecordFailure(tok); !locked {
		t.Fatal("达到上限应返回锁定")
	}
	if _, _, ok := s.Validate(tok); ok {
		t.Fatal("锁定后令牌应已失效")
	}
	// 已作废令牌再失败仍报锁定。
	if locked := s.RecordFailure(tok); !locked {
		t.Fatal("已作废令牌应始终视为锁定")
	}
}
