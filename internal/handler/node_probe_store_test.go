package handler

import (
	"path/filepath"
	"testing"
)

// ring 必须真的封顶:探测每 5 分钟一轮跑很久,不封顶的话内存只增不减。
func TestNodeProbeRingIsBounded(t *testing.T) {
	s := NewNodeProbeStore(3)
	for i := 0; i < 10; i++ {
		s.Record(1, int64(i), true, "master")
	}
	st, ok := s.Snapshot(1)
	if !ok {
		t.Fatal("没记录到")
	}
	if len(st.Samples) != 3 {
		t.Fatalf("采样数 = %d, want 3(ring 没封顶,内存会一直涨)", len(st.Samples))
	}
	// 保留的必须是最新的三个
	if st.Samples[0].LatencyMs != 7 || st.Samples[2].LatencyMs != 9 {
		t.Fatalf("留下的不是最新的三个: %+v", st.Samples)
	}
}

// 连续失败要累计、成功要清零 —— 告警的「连续 K 次」去抖全靠它。
func TestNodeProbeFailStreak(t *testing.T) {
	s := NewNodeProbeStore(10)
	if st := s.Record(1, 0, false, "master"); st.FailStreak != 1 {
		t.Fatalf("首次失败 streak = %d, want 1", st.FailStreak)
	}
	if st := s.Record(1, 0, false, "master"); st.FailStreak != 2 {
		t.Fatalf("二次失败 streak = %d, want 2", st.FailStreak)
	}
	if st := s.Record(1, 50, true, "master"); st.FailStreak != 0 {
		t.Fatalf("成功后 streak = %d, want 0 —— 不清零会导致恢复后仍被判不可用", st.FailStreak)
	}
}

// 取消勾选/删除节点后必须能清掉历史,否则面板上挂着一条永不更新的旧曲线,
// 且 map 只增不减。
func TestNodeProbeRetainOnly(t *testing.T) {
	s := NewNodeProbeStore(10)
	s.Record(1, 10, true, "master")
	s.Record(2, 20, true, "master")
	s.Record(3, 30, true, "master")

	s.RetainOnly(map[int64]struct{}{1: {}, 3: {}})

	if _, ok := s.Snapshot(2); ok {
		t.Error("节点 2 已不在待探测清单,历史却还在")
	}
	if _, ok := s.Snapshot(1); !ok {
		t.Error("节点 1 仍在清单里,历史不该被清")
	}
	if len(s.All()) != 2 {
		t.Errorf("剩余 %d 个, want 2", len(s.All()))
	}
}

// checkpoint 是重启恢复点:存下去要能原样读回来,否则历史每次重启都清零。
func TestNodeProbeCheckpointRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node-probe.json")
	s := NewNodeProbeStore(10)
	s.Record(7, 123, true, "tester#2")
	s.Record(7, 0, false, "tester#2")
	s.MarkAnnounced(7, true)

	if err := s.saveCheckpoint(path); err != nil {
		t.Fatalf("保存: %v", err)
	}

	s2 := NewNodeProbeStore(10)
	if err := s2.loadCheckpoint(path); err != nil {
		t.Fatalf("读取: %v", err)
	}
	st, ok := s2.Snapshot(7)
	if !ok {
		t.Fatal("恢复后拿不到节点 7")
	}
	if len(st.Samples) != 2 || st.Samples[0].LatencyMs != 123 {
		t.Fatalf("采样没恢复: %+v", st.Samples)
	}
	if st.FailStreak != 1 {
		t.Fatalf("失败计数没恢复: %d", st.FailStreak)
	}
	if !st.Announced {
		t.Fatal("告警标记没恢复 —— 会导致重启后对同一故障重复发公告")
	}
}

// Snapshot / All 必须返回副本:否则调用方拿到的是内部切片,
// 下一轮 Record 追加时会在调用方背后改数据(甚至并发读写崩溃)。
func TestNodeProbeSnapshotIsCopy(t *testing.T) {
	s := NewNodeProbeStore(10)
	s.Record(1, 10, true, "master")
	st, _ := s.Snapshot(1)
	st.Samples[0].LatencyMs = 999

	again, _ := s.Snapshot(1)
	if again.Samples[0].LatencyMs != 10 {
		t.Fatal("Snapshot 返回的是内部切片 —— 调用方改一下就污染了 store")
	}
}
