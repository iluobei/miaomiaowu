package handler

import (
	"errors"
	"sync"
	"time"

	"miaomiaowu/internal/logger"
)

// 使用英文错误消息, 防止老外看不懂
var ErrRateLimited = errors.New("rate limit exceeded")

type attemptInfo struct {
	count     int
	firstTime time.Time
	lockUntil time.Time
}

type LoginRateLimiter struct {
	ipAttempts      sync.Map // IP -> *attemptInfo
	accountAttempts sync.Map // username -> *attemptInfo
	maxAttempts     int
	windowDuration  time.Duration
	lockDuration    time.Duration
}

func NewLoginRateLimiter() *LoginRateLimiter {
	// 1小时5次
	return &LoginRateLimiter{
		maxAttempts:    5,
		windowDuration: time.Hour,
		lockDuration:   time.Hour,
	}
}

func (l *LoginRateLimiter) Check(ip, username string) error {
	now := time.Now()

	if err := l.checkAttempts(&l.ipAttempts, ip, now); err != nil {
		logger.Warn("🚫🚫🚫 [RATE_LIMIT] 登录被限制（IP）",
			"ip", ip,
			"username", username,
		)
		return err
	}

	if username != "" {
		if err := l.checkAttempts(&l.accountAttempts, username, now); err != nil {
			logger.Warn("🚫🚫🚫 [RATE_LIMIT] 登录被限制（账户）",
				"ip", ip,
				"username", username,
			)
			return err
		}
	}

	return nil
}

func (l *LoginRateLimiter) checkAttempts(store *sync.Map, key string, now time.Time) error {
	val, _ := store.Load(key)
	if val == nil {
		return nil
	}

	info := val.(*attemptInfo)

	if !info.lockUntil.IsZero() && now.Before(info.lockUntil) {
		return ErrRateLimited
	}

	if !info.lockUntil.IsZero() && now.After(info.lockUntil) {
		store.Delete(key)
		return nil
	}

	if now.Sub(info.firstTime) > l.windowDuration {
		store.Delete(key)
		return nil
	}

	if info.count >= l.maxAttempts {
		// Lock the key
		info.lockUntil = now.Add(l.lockDuration)
		return ErrRateLimited
	}

	return nil
}

func (l *LoginRateLimiter) RecordFailure(ip, username string) {
	now := time.Now()

	l.recordAttempt(&l.ipAttempts, ip, now)
	if username != "" {
		l.recordAttempt(&l.accountAttempts, username, now)
	}
}

func (l *LoginRateLimiter) recordAttempt(store *sync.Map, key string, now time.Time) {
	val, loaded := store.Load(key)
	if !loaded {
		store.Store(key, &attemptInfo{
			count:     1,
			firstTime: now,
		})
		return
	}

	info := val.(*attemptInfo)

	if now.Sub(info.firstTime) > l.windowDuration {
		store.Store(key, &attemptInfo{
			count:     1,
			firstTime: now,
		})
		return
	}

	info.count++
}

func (l *LoginRateLimiter) RecordSuccess(ip, username string) {
	l.ipAttempts.Delete(ip)
	if username != "" {
		l.accountAttempts.Delete(username)
	}
}
