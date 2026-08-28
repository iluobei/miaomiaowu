package storage

import (
	"context"
	"testing"
)

// system_config 是一张宽表,读写全靠**位置对齐**:SELECT 列清单、Scan 目标、UPDATE 参数、
// INSERT 列+VALUES+参数,五处必须同步。少改一处不会编译失败,只会把某个开关的值
// 静默串到隔壁字段上 —— 这条 round-trip 就是防这个。
func TestSystemConfigNodeProbeNotifyRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	cfg, err := repo.GetSystemConfig(ctx)
	if err != nil {
		t.Fatalf("读初始配置: %v", err)
	}
	if cfg.NotifyNodeProbeOffline || cfg.NotifyNodeProbeOnline {
		t.Fatal("节点探测通知默认必须是关的")
	}

	// 只开「离线」,不开「上线」—— 两个字段若串位,这里就会露馅
	cfg.NotifyNodeProbeOffline = true
	cfg.NotifyNodeProbeOnline = false
	// 同时改一个相邻字段,确认没有把它挤掉
	cfg.NotifyExpiry = true
	if err := repo.UpdateSystemConfig(ctx, cfg); err != nil {
		t.Fatalf("保存: %v", err)
	}

	got, err := repo.GetSystemConfig(ctx)
	if err != nil {
		t.Fatalf("回读: %v", err)
	}
	if !got.NotifyNodeProbeOffline {
		t.Error("节点离线通知没存住")
	}
	if got.NotifyNodeProbeOnline {
		t.Error("节点上线通知被误开 —— 疑似字段串位")
	}
	if !got.NotifyExpiry {
		t.Error("相邻的 NotifyExpiry 被挤掉了 —— 疑似列清单没对齐")
	}

	// 反向再来一次,确认不是「永远返回 true」
	got.NotifyNodeProbeOffline = false
	got.NotifyNodeProbeOnline = true
	if err := repo.UpdateSystemConfig(ctx, got); err != nil {
		t.Fatalf("二次保存: %v", err)
	}
	again, _ := repo.GetSystemConfig(ctx)
	if again.NotifyNodeProbeOffline || !again.NotifyNodeProbeOnline {
		t.Fatalf("二次回读不符: offline=%v online=%v", again.NotifyNodeProbeOffline, again.NotifyNodeProbeOnline)
	}
}
