package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/storage"
)

var globalBruteForceProtector *BruteForceProtector

type bruteForceRecord struct {
	count      int
	firstTime  time.Time
	blockUntil time.Time
	// permanent 表示永久封禁。不能用 blockUntil 零值表示——零值的既有语义是「未封禁」。
	permanent bool
}

type BruteForceProtector struct {
	mu            sync.RWMutex
	attempts      sync.Map // IP -> *bruteForceRecord
	enabled       bool
	maxFailures   int
	window        time.Duration
	blockDuration time.Duration
	// skipLocalIP 命中 loopback/私有/link-local 网段时跳过记账与封禁,
	// 防反代/docker 未正确转发 XFF 时一封封死所有用户。默认 true。
	skipLocalIP bool
	// repo 用于把封禁/探测事件双写到 DB（内存态重启即失，持久化才能跨重启、支持永久封禁）。
	// 可为 nil（老的构造路径或测试）——所有 DB 写入都先判空，nil 时退化为纯内存行为，不回归。
	repo *storage.TrafficRepository
}

// SetRepo 注入持久化仓库。main.go 在构造后调用；不调用则退化为纯内存（向后兼容）。
func (p *BruteForceProtector) SetRepo(repo *storage.TrafficRepository) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.repo = repo
}

func (p *BruteForceProtector) getRepo() *storage.TrafficRepository {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.repo
}

// NewBruteForceProtector 用 hardcoded 默认值构造。
// 加严:24h 窗口内 5 次失败 → 封 24h(同步自 mmw v0.7.3 #89,防订阅 token 枚举)。
// 启动时若 system_settings 里有自定义阈值,main.go 会改用 NewBruteForceProtectorWithConfig。
func NewBruteForceProtector() *BruteForceProtector {
	p := &BruteForceProtector{
		enabled:       true,
		maxFailures:   5,
		window:        24 * time.Hour,
		blockDuration: 24 * time.Hour,
		skipLocalIP:   true,
	}
	globalBruteForceProtector = p
	return p
}

// NewBruteForceProtectorWithConfig 用 system_settings 里读出的自定义阈值构造。
// windowMinutes / blockMinutes 用分钟为单位,因为前端配置面板按分钟输入更直观。
func NewBruteForceProtectorWithConfig(enabled bool, maxFailures, windowMinutes, blockMinutes int) *BruteForceProtector {
	p := &BruteForceProtector{
		enabled:       enabled,
		maxFailures:   maxFailures,
		window:        time.Duration(windowMinutes) * time.Minute,
		blockDuration: time.Duration(blockMinutes) * time.Minute,
		skipLocalIP:   true,
	}
	globalBruteForceProtector = p
	return p
}

// SetSkipLocalIP 切换"是否跳过本地/私有 IP"。
// security_settings handler 启动初始化 + PUT 热更新时调用。
func (p *BruteForceProtector) SetSkipLocalIP(skip bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.skipLocalIP = skip
}

// shouldSkip 返回是否应跳过该 IP — 当 skipLocalIP 开启且 IP 落在本地/私有网段。
func (p *BruteForceProtector) shouldSkip(ip string) bool {
	p.mu.RLock()
	skip := p.skipLocalIP
	p.mu.RUnlock()
	return skip && IsLocalOrPrivateIP(ip)
}

// UpdateConfig 热更新参数 — security_settings handler 收到 PUT 后调用,无需重启主控。
func (p *BruteForceProtector) UpdateConfig(enabled bool, maxFailures, windowMinutes, blockMinutes int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
	p.maxFailures = maxFailures
	p.window = time.Duration(windowMinutes) * time.Minute
	p.blockDuration = time.Duration(blockMinutes) * time.Minute
}

func (p *BruteForceProtector) getConfig() (bool, int, time.Duration, time.Duration) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.enabled, p.maxFailures, p.window, p.blockDuration
}

func GetBruteForceProtector() *BruteForceProtector {
	return globalBruteForceProtector
}

func (p *BruteForceProtector) IsBlocked(ip, path string) bool {
	enabled, _, _, _ := p.getConfig()
	if !enabled {
		return false
	}
	if p.shouldSkip(ip) {
		return false
	}

	val, ok := p.attempts.Load(ip)
	if !ok {
		return false
	}
	rec := val.(*bruteForceRecord)

	now := time.Now()
	// 永久封禁：不看 blockUntil，始终拦截。
	if rec.permanent {
		logger.Warn("🚫🚫🚫 [BRUTE_FORCE] 永久封禁IP尝试访问，已拦截", "ip", ip, "访问路径", path)
		return true
	}
	if !rec.blockUntil.IsZero() && now.Before(rec.blockUntil) {
		logger.Warn("🚫🚫🚫 [BRUTE_FORCE] 已封禁IP尝试访问，已拦截",
			"ip", ip,
			"访问路径", path,
			"封禁剩余", rec.blockUntil.Sub(now).Round(time.Second).String(),
		)
		return true
	}

	// 封禁已过期，清除
	if !rec.blockUntil.IsZero() {
		logger.Info("✅ [BRUTE_FORCE] IP封禁已过期，已自动解除",
			"ip", ip,
		)
		p.attempts.Delete(ip)
	}
	return false
}

func (p *BruteForceProtector) RecordFailure(ip, path string) {
	enabled, maxFailures, window, blockDuration := p.getConfig()
	if !enabled {
		return
	}
	if p.shouldSkip(ip) {
		return
	}

	now := time.Now()

	val, loaded := p.attempts.Load(ip)
	if !loaded {
		logger.Warn("⚠️ [BRUTE_FORCE] 订阅探测失败",
			"ip", ip,
			"访问路径", path,
			"次数", fmt.Sprintf("1/%d", maxFailures),
		)
		p.attempts.Store(ip, &bruteForceRecord{
			count:     1,
			firstTime: now,
		})
		p.recordEvent(ip, "probe", path, fmt.Sprintf("1/%d", maxFailures))
		return
	}

	rec := val.(*bruteForceRecord)

	if rec.permanent || (!rec.blockUntil.IsZero() && now.Before(rec.blockUntil)) {
		return
	}

	if now.Sub(rec.firstTime) > window {
		logger.Warn("⚠️ [BRUTE_FORCE] 订阅探测失败（窗口重置）",
			"ip", ip,
			"访问路径", path,
			"次数", fmt.Sprintf("1/%d", maxFailures),
		)
		p.attempts.Store(ip, &bruteForceRecord{
			count:     1,
			firstTime: now,
		})
		p.recordEvent(ip, "probe", path, fmt.Sprintf("1/%d", maxFailures))
		return
	}

	rec.count++
	if rec.count >= maxFailures {
		rec.blockUntil = now.Add(blockDuration)
		logger.Warn("🚫🚫🚫 [BRUTE_FORCE] IP 已被封禁！",
			"ip", ip,
			"触发路径", path,
			"失败次数", rec.count,
			"封禁至", rec.blockUntil.Format("2006-01-02 15:04:05"),
		)
		// 持久化封禁态 + 事件流：重启后靠 ip_bans 回填内存，否则封禁一重启就没了。
		p.persistBan(ip, path, rec.count, rec.blockUntil, false, "")

		if n := GetNotifier(); n != nil {
			go n.Send(context.Background(), notify.Event{
				Type:    notify.EventIPBan,
				Title:   "IP 封禁",
				Message: fmt.Sprintf("IP `%s` 已被封禁\n触发路径: `%s`\n失败次数: %d\n封禁至: %s", ip, path, rec.count, rec.blockUntil.Format("2006-01-02 15:04:05")),
			})
		}
	} else {
		logger.Warn("⚠️ [BRUTE_FORCE] 订阅探测失败",
			"ip", ip,
			"访问路径", path,
			"次数", fmt.Sprintf("%d/%d", rec.count, maxFailures),
		)
		p.recordEvent(ip, "probe", path, fmt.Sprintf("%d/%d", rec.count, maxFailures))
	}
}

func (p *BruteForceProtector) RecordSuccess(ip string) {
	p.attempts.Delete(ip)
}

// recordEvent 把一条安全事件写进 DB（best-effort，失败静默——日志记录不该影响主流程）。
func (p *BruteForceProtector) recordEvent(ip, kind, path, detail string) {
	repo := p.getRepo()
	if repo == nil {
		return
	}
	_ = repo.InsertSecurityEvent(context.Background(), storage.SecurityEvent{
		IP: ip, Kind: kind, Path: path, Detail: detail,
	})
}

// persistBan 把封禁写入 ip_bans + 一条 ban 事件。permanent=true 时 expiresAt 忽略。
func (p *BruteForceProtector) persistBan(ip, path string, failCount int, blockUntil time.Time, permanent bool, actor string) {
	repo := p.getRepo()
	if repo == nil {
		return
	}
	ctx := context.Background()
	var expires *time.Time
	if !permanent {
		expires = &blockUntil
	}
	_ = repo.UpsertIPBan(ctx, storage.IPBan{
		IP:        ip,
		Reason:    "brute_force",
		BannedAt:  time.Now(),
		ExpiresAt: expires,
		Permanent: permanent,
		FailCount: failCount,
		Actor:     actor,
	})
	kind := "ban"
	if actor != "" {
		kind = "ban_manual"
	}
	_ = repo.InsertSecurityEvent(ctx, storage.SecurityEvent{
		IP: ip, Kind: kind, Path: path, Detail: fmt.Sprintf("fail=%d", failCount), Actor: actor,
	})
}

// BanIP 手动封禁一个 IP（admin 操作）。permanent=true 为永久封禁，否则按 blockDuration 定时。
func (p *BruteForceProtector) BanIP(ip string, permanent bool, actor string) {
	_, _, _, blockDuration := p.getConfig()
	now := time.Now()
	rec := &bruteForceRecord{count: 0, firstTime: now, permanent: permanent}
	if !permanent {
		rec.blockUntil = now.Add(blockDuration)
	}
	p.attempts.Store(ip, rec)
	p.persistBan(ip, "", 0, rec.blockUntil, permanent, actor)
	logger.Warn("🚫 [BRUTE_FORCE] 管理员手动封禁 IP", "ip", ip, "永久", permanent, "操作者", actor)
}

// UnbanIP 解封一个 IP（admin 操作）：清内存 + DB 留痕。
func (p *BruteForceProtector) UnbanIP(ip, actor string) {
	p.attempts.Delete(ip)
	if repo := p.getRepo(); repo != nil {
		_ = repo.ReleaseIPBan(context.Background(), ip, actor)
		_ = repo.InsertSecurityEvent(context.Background(), storage.SecurityEvent{
			IP: ip, Kind: "unban", Actor: actor,
		})
	}
	logger.Info("✅ [BRUTE_FORCE] 管理员手动解封 IP", "ip", ip, "操作者", actor)
}

// RestoreFromDB 启动时把 DB 里仍生效的封禁灌回内存。
// 永久封禁靠这个才能跨重启生效 —— 没有它，重启后 sync.Map 空空如也，等于没封。
func (p *BruteForceProtector) RestoreFromDB(ctx context.Context) {
	repo := p.getRepo()
	if repo == nil {
		return
	}
	bans, err := repo.ListRestorableIPBans(ctx)
	if err != nil {
		logger.Warn("[BRUTE_FORCE] 启动回填封禁失败", "error", err)
		return
	}
	n := 0
	for _, b := range bans {
		rec := &bruteForceRecord{count: b.FailCount, firstTime: b.BannedAt, permanent: b.Permanent}
		if !b.Permanent && b.ExpiresAt != nil {
			rec.blockUntil = *b.ExpiresAt
		}
		p.attempts.Store(b.IP, rec)
		n++
	}
	if n > 0 {
		logger.Info("[BRUTE_FORCE] 已从 DB 回填封禁记录", "数量", n)
	}
}

// StartCleanup 定期清理内存里已过期的封禁记录，避免 sync.Map 无限增长
// （原本只在 IsBlocked 命中时惰性清，未再访问的 IP 会永久驻留）。永久封禁不清。
func (p *BruteForceProtector) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			p.attempts.Range(func(k, v any) bool {
				rec := v.(*bruteForceRecord)
				if rec.permanent {
					return true
				}
				// 已过封禁期、且已过统计窗口 → 内存可清（DB 行保留作历史）
				if rec.blockUntil.IsZero() {
					if now.Sub(rec.firstTime) > p.window {
						p.attempts.Delete(k)
					}
				} else if now.After(rec.blockUntil) {
					p.attempts.Delete(k)
				}
				return true
			})
		}
	}
}

// StatusRecorder wraps http.ResponseWriter to capture the status code.
type StatusRecorder struct {
	http.ResponseWriter
	StatusCode int
}

func (r *StatusRecorder) WriteHeader(code int) {
	r.StatusCode = code
	r.ResponseWriter.WriteHeader(code)
}
