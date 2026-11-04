package storage

import (
	"context"
	"testing"
)

func TestExternalSubscriptionNamePrefixRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	id, err := repo.CreateExternalSubscription(ctx, ExternalSubscription{
		Username:              "alice",
		Name:                  "机场A",
		URL:                   "https://example.com/a",
		NamePrefix:            "〖机场A〗",
		AutoUpdate:            true,
		UpdateIntervalMinutes: 60,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetExternalSubscription(ctx, id, "alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.NamePrefix != "〖机场A〗" {
		t.Fatalf("prefix = %q, want 〖机场A〗", got.NamePrefix)
	}
	if got.AppliedNamePrefix != "" {
		t.Fatalf("applied prefix = %q, want empty until sync", got.AppliedNamePrefix)
	}
	got.AppliedNamePrefix = "【aa】"
	if !got.AutoUpdate || got.UpdateIntervalMinutes != 60 {
		t.Fatalf("auto update fields changed: %+v", got)
	}

	// 同步路径会改节点数和流量后再整行更新，前缀必须留在结构体里写回去。
	got.NodeCount = 3
	got.Upload = 10
	if err := repo.UpdateExternalSubscription(ctx, got); err != nil {
		t.Fatalf("update traffic fields: %v", err)
	}
	got, err = repo.GetExternalSubscription(ctx, id, "alice")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.NamePrefix != "〖机场A〗" || got.NodeCount != 3 || got.Upload != 10 {
		t.Fatalf("sync-style update dropped fields: prefix=%q count=%d upload=%d", got.NamePrefix, got.NodeCount, got.Upload)
	}
	if got.AppliedNamePrefix != "【aa】" {
		t.Fatalf("applied prefix dropped on sync-style update: %q", got.AppliedNamePrefix)
	}

	got.NamePrefix = "[机场A] "
	if err := repo.UpdateExternalSubscription(ctx, got); err != nil {
		t.Fatalf("update prefix: %v", err)
	}
	byURL, err := repo.GetExternalSubscriptionByURL(ctx, "alice", "https://example.com/a")
	if err != nil {
		t.Fatalf("get by url: %v", err)
	}
	if byURL.NamePrefix != "[机场A] " {
		t.Fatalf("prefix with trailing space = %q", byURL.NamePrefix)
	}

	listed, err := repo.ListExternalSubscriptions(ctx, "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].NamePrefix != "[机场A] " {
		t.Fatalf("list prefix = %+v", listed)
	}

	all, err := repo.ListAllExternalSubscriptions(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	var found bool
	for _, sub := range all {
		if sub.ID == id {
			found = true
			if sub.NamePrefix != "[机场A] " {
				t.Fatalf("list all prefix = %q", sub.NamePrefix)
			}
		}
	}
	if !found {
		t.Fatal("list all missed subscription")
	}

	byURL.NamePrefix = ""
	if err := repo.UpdateExternalSubscription(ctx, byURL); err != nil {
		t.Fatalf("clear prefix: %v", err)
	}
	cleared, err := repo.GetExternalSubscription(ctx, id, "alice")
	if err != nil {
		t.Fatalf("get cleared: %v", err)
	}
	if cleared.NamePrefix != "" {
		t.Fatalf("cleared prefix = %q", cleared.NamePrefix)
	}
	if cleared.AppliedNamePrefix != "【aa】" {
		t.Fatalf("clearing the desired prefix also cleared the applied prefix: %q", cleared.AppliedNamePrefix)
	}

	// 未写入 name_prefix 的旧行应读出空字符串，同步行为与升级前一致。
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO external_subscriptions (username, name, url) VALUES (?, ?, ?)`, "alice", "旧订阅", "https://example.com/old"); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	legacy, err := repo.GetExternalSubscriptionByURL(ctx, "alice", "https://example.com/old")
	if err != nil {
		t.Fatalf("get legacy: %v", err)
	}
	if legacy.NamePrefix != "" {
		t.Fatalf("legacy prefix = %q, want empty", legacy.NamePrefix)
	}
	if legacy.AppliedNamePrefix != "" {
		t.Fatalf("legacy applied prefix = %q, want empty", legacy.AppliedNamePrefix)
	}
}
