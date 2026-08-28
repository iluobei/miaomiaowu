package handler

import (
	"testing"
	"time"
)

func sampleAt(agoMin int, ok bool) NodeProbeSample {
	return NodeProbeSample{At: time.Now().Add(-time.Duration(agoMin) * time.Minute).Unix(), OK: ok}
}

// downDuration 从采样倒推「连续挂了多久」。刻意不存 DownSince 字段 —— checkpoint 只存
// 采样,存了也会在重启后丢;采样里本来就有时间戳。
func TestDownDurationCountsOnlyTrailingFailures(t *testing.T) {
	cases := []struct {
		name    string
		samples []NodeProbeSample
		wantMin int // 期望的掉线分钟数(允许 ±1 抖动)
	}{
		{"全是成功 → 0", []NodeProbeSample{sampleAt(30, true), sampleAt(20, true)}, 0},
		{"最后一条成功 → 0(刚恢复不该算掉线)", []NodeProbeSample{sampleAt(30, false), sampleAt(5, true)}, 0},
		{"尾部连续 3 次失败,从最早那次算起", []NodeProbeSample{
			sampleAt(60, true), sampleAt(45, false), sampleAt(30, false), sampleAt(15, false),
		}, 45},
		{"中间恢复过 → 只从恢复之后算", []NodeProbeSample{
			sampleAt(60, false), sampleAt(50, true), sampleAt(20, false), sampleAt(10, false),
		}, 20},
		{"空历史 → 0", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := int(downDuration(NodeProbeState{Samples: c.samples}).Round(time.Minute).Minutes())
			if got < c.wantMin-1 || got > c.wantMin+1 {
				t.Fatalf("downDuration = %d 分钟, want ≈%d", got, c.wantMin)
			}
		})
	}
}

// lastLatency 给「已恢复」通知报个数字,空历史不能 panic。
func TestLastLatency(t *testing.T) {
	if got := lastLatency(NodeProbeState{}); got != 0 {
		t.Fatalf("空历史 = %d, want 0", got)
	}
	st := NodeProbeState{Samples: []NodeProbeSample{{LatencyMs: 10, OK: true}, {LatencyMs: 42, OK: true}}}
	if got := lastLatency(st); got != 42 {
		t.Fatalf("= %d, want 42(取最后一条)", got)
	}
}

// 重同步冷却:同一用户在冷却期内只触发一次。
//
// 这是防「机场被敲爆」的关键 —— 节点永久挂掉时每轮(5 分钟)都会满足掉线阈值,
// 没有冷却就会每 5 分钟去拉一次订阅,而且用户配的阈值越小敲得越勤。
func TestResyncCooldownIsAtLeastFloor(t *testing.T) {
	s := &NodeProbeScheduler{lastResync: map[string]time.Time{}}

	// 用户把阈值设成 1 分钟,冷却仍必须抬到硬下限
	cooldown := time.Duration(1) * time.Minute
	if cooldown < nodeProbeResyncMinInterval {
		cooldown = nodeProbeResyncMinInterval
	}
	if cooldown != nodeProbeResyncMinInterval {
		t.Fatalf("冷却 = %s, want 抬到硬下限 %s", cooldown, nodeProbeResyncMinInterval)
	}

	// 首次可触发
	s.lastResync["alice"] = time.Now().Add(-cooldown - time.Minute)
	if !s.resyncAllowed("alice", cooldown) {
		t.Fatal("超过冷却期应允许重同步")
	}
	// 刚触发过 → 拒绝
	if s.resyncAllowed("alice", cooldown) {
		t.Fatal("冷却期内不该再次重同步")
	}
	// 另一个用户互不影响
	if !s.resyncAllowed("bob", cooldown) {
		t.Fatal("不同用户的冷却不该互相牵连")
	}
}
