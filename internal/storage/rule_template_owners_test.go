package storage

import (
	"context"
	"testing"
)

func TestRuleTemplateOwnershipAndVisibility(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if err := repo.SetRuleTemplateOwner(ctx, "alice.yaml", "alice"); err != nil {
		t.Fatal(err)
	}
	if owner, err := repo.GetRuleTemplateOwner(ctx, "alice.yaml"); err != nil || owner != "alice" {
		t.Fatalf("owner=%q err=%v", owner, err)
	}
	if repo.IsRuleTemplatePublic(ctx, "alice.yaml") {
		t.Fatal("user template should be private by default")
	}
	if err := repo.SetRuleTemplatePublic(ctx, "alice.yaml", true); err != nil {
		t.Fatal(err)
	}
	if !repo.IsRuleTemplatePublic(ctx, "alice.yaml") {
		t.Fatal("template visibility was not persisted")
	}
	if err := repo.RenameRuleTemplateOwner(ctx, "alice.yaml", "renamed.yaml"); err != nil {
		t.Fatal(err)
	}
	if owner, _ := repo.GetRuleTemplateOwner(ctx, "renamed.yaml"); owner != "alice" {
		t.Fatalf("renamed owner=%q", owner)
	}
}
