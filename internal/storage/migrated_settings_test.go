package storage

import (
	"context"
	"testing"
)

func TestMigratedDefaultTemplatesPersist(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	settings := UserSettings{
		Username:                     "alice",
		MatchRule:                    "node_name",
		SyncScope:                    "saved_only",
		KeepNodeName:                 true,
		TemplateVersion:              "v2",
		DefaultTemplateFilename:      "alice.yaml",
		DefaultSurgeTemplateFilename: "alice.conf",
	}
	if err := repo.UpsertUserSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetUserSettings(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTemplateFilename != "alice.yaml" || got.DefaultSurgeTemplateFilename != "alice.conf" {
		t.Fatalf("defaults were not persisted: %+v", got)
	}
}

func TestCheckpointBestEffort(t *testing.T) {
	repo := newTestRepo(t)
	defer repo.Close()
	truncated, remaining, err := repo.CheckpointBestEffort()
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || remaining != 0 {
		t.Fatalf("checkpoint result truncated=%v remaining=%d", truncated, remaining)
	}
}
