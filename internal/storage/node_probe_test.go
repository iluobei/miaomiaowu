package storage

import (
	"context"
	"testing"
)

func mkProbeNode(t *testing.T, repo *TrafficRepository, name, originalServer string, enabled bool) int64 {
	t.Helper()
	n := Node{
		Username: "admin", NodeName: name, Protocol: "vless",
		ParsedConfig: "{}", ClashConfig: `{"name":"` + name + `","type":"vless"}`,
		Enabled: enabled, Tag: "personal", OriginalServer: originalServer,
	}
	created, err := repo.CreateNode(context.Background(), n)
	if err != nil {
		t.Fatalf("建节点 %s: %v", name, err)
	}
	return created.ID
}

// 待探测清单的筛选:勾选了 + 启用中。
//
// **不能**按 original_server 过滤。妙妙屋X 那边用 original_server='' 筛外部节点,
// 因为它那里存的是自建服务器名;本项目里这一列是「手动改地址前的旧地址备份」,
// 照抄会把改过地址的节点莫名排除。这条用例专门钉住这个差异。
func TestListProbeEnabledNodesFiltering(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	plain := mkProbeNode(t, repo, "普通-勾选", "", true)
	_ = mkProbeNode(t, repo, "普通-未勾选", "", true)
	// original_server 非空 = 用户手动改过服务器地址。这种节点**照样要探测**。
	edited := mkProbeNode(t, repo, "改过地址的-勾选", "1.2.3.4", true)
	disabled := mkProbeNode(t, repo, "已禁用", "", false)

	for _, id := range []int64{plain, edited, disabled} {
		if err := repo.SetNodeProbeEnabled(ctx, id, true); err != nil {
			t.Fatalf("开探测 %d: %v", id, err)
		}
	}

	got, err := repo.ListProbeEnabledNodes(ctx)
	if err != nil {
		t.Fatalf("取清单: %v", err)
	}
	var names []string
	for _, g := range got {
		names = append(names, g.NodeName)
	}
	if len(got) != 2 {
		t.Fatalf("清单 = %v, want 「普通-勾选」+「改过地址的-勾选」两个", names)
	}
	seen := map[int64]bool{}
	for _, g := range got {
		seen[g.ID] = true
		if g.ClashConfig == "" {
			t.Errorf("%q 的 ClashConfig 为空 —— 探测需要它来起 mihomo", g.NodeName)
		}
	}
	if !seen[plain] || !seen[edited] {
		t.Fatalf("清单 = %v, 缺少应有的节点", names)
	}
}

// 开关能来回切,且计数跟着变。
func TestSetNodeProbeEnabledToggles(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	id := mkProbeNode(t, repo, "n1", "", true)

	if n, _ := repo.CountProbeEnabledNodes(ctx); n != 0 {
		t.Fatalf("初始应为 0,实得 %d —— 探测不该对导入节点默认开启", n)
	}
	if err := repo.SetNodeProbeEnabled(ctx, id, true); err != nil {
		t.Fatalf("开启: %v", err)
	}
	if n, _ := repo.CountProbeEnabledNodes(ctx); n != 1 {
		t.Fatalf("开启后 = %d, want 1", n)
	}
	if err := repo.SetNodeProbeEnabled(ctx, id, false); err != nil {
		t.Fatalf("关闭: %v", err)
	}
	if n, _ := repo.CountProbeEnabledNodes(ctx); n != 0 {
		t.Fatalf("关闭后 = %d, want 0", n)
	}
}

// 不存在的节点要报错而不是静默成功 —— 否则前端点了开关看着成功,实际没生效。
func TestSetNodeProbeEnabledRejectsMissingNode(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.SetNodeProbeEnabled(context.Background(), 999999, true); err == nil {
		t.Fatal("对不存在的节点开探测竟然成功了")
	}
}

// probe_enabled 要能跟着节点一起被读出来(列表页要显示开关状态)。
func TestNodeProbeEnabledSurvivesRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	id := mkProbeNode(t, repo, "n1", "", true)
	if err := repo.SetNodeProbeEnabled(ctx, id, true); err != nil {
		t.Fatalf("开启: %v", err)
	}
	nodes, err := repo.ListNodes(ctx, "admin")
	if err != nil {
		t.Fatalf("列表: %v", err)
	}
	for _, n := range nodes {
		if n.ID == id {
			if !n.ProbeEnabled {
				t.Fatal("ProbeEnabled 没被读出来 —— 列表页开关会一直显示关闭")
			}
			return
		}
	}
	t.Fatal("列表里找不到该节点")
}

// 计数和清单必须同口径。这两个查询的 WHERE 是分开写的,真机上就漏改过一处:
// 清单去掉了 original_server 过滤而计数没去,面板上「已勾选 1 / 2」和实际探测的
// 节点数对不上,而两个数字都来自同一个接口,用户只会觉得开关坏了。
func TestCountMatchesListProbeEnabled(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	ids := []int64{
		mkProbeNode(t, repo, "普通", "", true),
		mkProbeNode(t, repo, "改过地址的", "9.9.9.9", true),
	}
	for _, id := range ids {
		if err := repo.SetNodeProbeEnabled(ctx, id, true); err != nil {
			t.Fatalf("开探测 %d: %v", id, err)
		}
	}

	list, err := repo.ListProbeEnabledNodes(ctx)
	if err != nil {
		t.Fatalf("取清单: %v", err)
	}
	count, err := repo.CountProbeEnabledNodes(ctx)
	if err != nil {
		t.Fatalf("计数: %v", err)
	}
	if count != len(list) {
		t.Fatalf("计数 %d ≠ 清单 %d —— 两个查询的 WHERE 不同口径", count, len(list))
	}
	if count != 2 {
		t.Fatalf("= %d, want 2(改过地址的节点也要算)", count)
	}
}
