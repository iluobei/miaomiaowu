package captcha

import (
	"context"
	"path/filepath"
	"testing"

	"miaomiaowu/internal/storage"
)

func TestTurnstileDynamicSettingsAndMissingToken(t *testing.T) {
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "turnstile.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	verifier := New(repo)
	ctx := context.Background()
	if verifier.Enabled(ctx) {
		t.Fatal("Turnstile should be disabled without keys")
	}
	if err := repo.SetSystemSetting(ctx, settingKeySiteKey, "1x00000000000000000000AA"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSystemSetting(ctx, settingKeySecretKey, "1x0000000000000000000000000000000AA"); err != nil {
		t.Fatal(err)
	}
	if !verifier.Enabled(ctx) || verifier.SiteKey(ctx) != "1x00000000000000000000AA" {
		t.Fatal("Turnstile did not pick up settings dynamically")
	}
	result, err := verifier.VerifyDetailed(ctx, "", "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || len(result.ErrorCodes) != 1 || result.ErrorCodes[0] != "missing-input-response" {
		t.Fatalf("unexpected missing-token result: %+v", result)
	}
}
