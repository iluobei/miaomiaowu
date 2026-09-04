package taskrun

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"miaomiaowu/internal/storage"
)

func TestRecorderThrottlesSuccessButAlwaysRecordsFailure(t *testing.T) {
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	recorder := New(repo, map[string]time.Duration{"sync": time.Hour})
	ctx := context.Background()
	recorder.Wrap(ctx, "sync", func() (string, error) { return "first", nil })
	recorder.Wrap(ctx, "sync", func() (string, error) { return "throttled", nil })
	recorder.Wrap(ctx, "sync", func() (string, error) { return "", errors.New("failed") })
	runs, err := repo.ListTaskRuns(ctx, "sync", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Status != "error" || runs[1].Status != "ok" {
		t.Fatalf("unexpected task runs: %+v", runs)
	}
}
