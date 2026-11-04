package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

// 隐藏只是管理列表的显示开关:设上能读回、能取消,且不改动节点本身(列表接口仍返回全部节点)。
func TestNodeHiddenSetAndList(t *testing.T) {
	repo, err := NewTrafficRepository(filepath.Join(t.TempDir(), "hidden.db"))
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()
	if err := repo.CreateUser(ctx, "admin", "", "", "h", "admin", ""); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, name := range []string{"a", "b", "c"} {
		n, err := repo.CreateNode(ctx, Node{
			Username: "admin", NodeName: name, Protocol: "vless",
			ClashConfig: `{"server":"1.2.3.4","port":443}`, ParsedConfig: "{}", Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}

	if got, _ := repo.ListHiddenNodeIDs(ctx); len(got) != 0 {
		t.Fatalf("新节点默认不隐藏,得到 %v", got)
	}
	if n, err := repo.SetNodesHidden(ctx, []int64{ids[0], ids[2]}, true); err != nil || n != 2 {
		t.Fatalf("隐藏两个节点: n=%d err=%v", n, err)
	}
	got, err := repo.ListHiddenNodeIDs(ctx)
	if err != nil || !reflect.DeepEqual(got, []int64{ids[0], ids[2]}) {
		t.Fatalf("隐藏列表 = %v, err=%v", got, err)
	}
	all, err := repo.ListNodes(ctx, "admin")
	if err != nil || len(all) != 3 {
		t.Fatalf("隐藏不应影响节点列表: len=%d err=%v", len(all), err)
	}
	if _, err := repo.SetNodesHidden(ctx, []int64{ids[0]}, false); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.ListHiddenNodeIDs(ctx)
	if !reflect.DeepEqual(got, []int64{ids[2]}) {
		t.Fatalf("取消隐藏后 = %v", got)
	}
	if n, err := repo.SetNodesHidden(ctx, nil, true); err != nil || n != 0 {
		t.Fatalf("空列表应为 no-op: n=%d err=%v", n, err)
	}
}
