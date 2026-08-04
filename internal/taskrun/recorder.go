// Package taskrun 为主控的定时任务提供一个最小侵入的运行记录器。
//
// 放在独立中立包（而非 internal/handler）的原因：定时任务分散在 internal/traffic、
// internal/ddns、internal/handler 三个包里，它们不能 import internal/handler（会循环）。
// 本包只依赖 internal/storage，三方都能安全 import。
package taskrun

import (
	"context"
	"sync"
	"time"

	"miaomiaowu/internal/storage"
)

// Recorder 包住一次任务执行并把结果写进 task_runs。
//
// 写入量控制（关键设计）：Speed Collector 3s/次、Traffic Collector 1min/次，全记会写爆表。
// 规则是「失败永远记，成功按各任务的 minSuccessInterval 节流」—— 高频 collector 设 5min，
// 低频任务设 0（每次都记）。节流间隔按任务名配置（intervals），未配置的任务默认 0。
type Recorder struct {
	repo          *storage.TrafficRepository
	intervals     map[string]time.Duration
	mu            sync.Mutex
	lastSuccessAt map[string]time.Time
}

// New 构造一个 Recorder。repo 为 nil 时所有写入静默跳过（退化为纯执行，不回归）。
// intervals 给出各任务名的成功节流间隔；不在表里的任务节流间隔为 0（每次成功都记）。
func New(repo *storage.TrafficRepository, intervals map[string]time.Duration) *Recorder {
	if intervals == nil {
		intervals = map[string]time.Duration{}
	}
	return &Recorder{
		repo:          repo,
		intervals:     intervals,
		lastSuccessAt: make(map[string]time.Time),
	}
}

// Wrap 执行 fn 并记录一次运行。fn 返回 (摘要, error)：
//   - error != nil → status=error，detail=error 文本，**永远记录**
//   - error == nil → status=ok，detail=摘要；若距上次同名成功不足该任务的节流间隔则**跳过写入**
//
// fn 内部的 panic 不在此拦截（任务本就各自负责），Wrap 只负责计时与落库。
func (r *Recorder) Wrap(ctx context.Context, taskName string, fn func() (string, error)) {
	start := time.Now()
	detail, err := fn()
	dur := time.Since(start)

	if err == nil && r.throttled(taskName, start) {
		return
	}

	status := "ok"
	if err != nil {
		status = "error"
		detail = err.Error()
	}
	if r.repo != nil {
		_ = r.repo.InsertTaskRun(ctx, taskName, start, dur.Milliseconds(), status, detail)
	}
}

// throttled 返回 true 表示「这次成功应被节流跳过」。返回 false 时更新 lastSuccessAt。
func (r *Recorder) throttled(taskName string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	interval := r.intervals[taskName]
	if interval <= 0 {
		return false
	}
	last, ok := r.lastSuccessAt[taskName]
	if ok && now.Sub(last) < interval {
		return true
	}
	r.lastSuccessAt[taskName] = now
	return false
}

// ---- 包级单例：让分散在多个包里的任务无需各自持有 Recorder ----

var defaultRecorder *Recorder

// Init 由 main.go 调用一次，设定全局 Recorder。未调用时 Record 退化为「只执行不记录」。
func Init(r *Recorder) { defaultRecorder = r }

// Record 用全局 Recorder 包住一次执行。全局未初始化时直接跑 fn（丢弃返回值），保证不回归。
func Record(ctx context.Context, taskName string, fn func() (string, error)) {
	if defaultRecorder == nil {
		_, _ = fn()
		return
	}
	defaultRecorder.Wrap(ctx, taskName, fn)
}
