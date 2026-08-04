package handler

import (
	"context"
	"path/filepath"
	"testing"

	"miaomiaowu/internal/storage"
)

func TestBruteForceBanPersistsAndRestores(t *testing.T) {
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "security.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	first := NewBruteForceProtector()
	first.SetSkipLocalIP(false)
	first.SetRepo(repo)
	first.BanIP("203.0.113.9", true, "admin")

	restored := NewBruteForceProtector()
	restored.SetSkipLocalIP(false)
	restored.SetRepo(repo)
	restored.RestoreFromDB(context.Background())
	if !restored.IsBlocked("203.0.113.9", "/probe") {
		t.Fatal("permanent ban was not restored")
	}

	restored.UnbanIP("203.0.113.9", "admin")
	if restored.IsBlocked("203.0.113.9", "/probe") {
		t.Fatal("IP remains blocked after unban")
	}
	bans, err := repo.ListActiveIPBans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bans) != 0 {
		t.Fatalf("active bans after unban: %+v", bans)
	}
	events, err := repo.ListSecurityEvents(context.Background(), "", "203.0.113.9", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatalf("expected ban and unban events, got %+v", events)
	}
}
