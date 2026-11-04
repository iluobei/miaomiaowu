package backupstore

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type fakeDest struct {
	objects []Object
	deleted []string
	kind    string
}

func (f *fakeDest) Kind() string                           { return f.kind }
func (f *fakeDest) List(context.Context) ([]Object, error) { return f.objects, nil }
func (f *fakeDest) Delete(_ context.Context, o Object) error {
	f.deleted = append(f.deleted, o.Name)
	return nil
}

func day(n int) time.Time { return time.Date(2026, 1, n, 0, 0, 0, 0, time.UTC) }

func obj(name string, d int) Object {
	return Object{Name: name, Modified: day(d)}
}

// 保留策略删最旧的,只留 keep 份。
func TestApplyRetentionKeepsNewest(t *testing.T) {
	f := &fakeDest{objects: []Object{
		obj("miaomiaowu-backup-20260103-000000.zip", 3),
		obj("miaomiaowu-backup-20260101-000000.zip", 1),
		obj("miaomiaowu-backup-20260102-000000.zip", 2),
		obj("miaomiaowu-backup-20260104-000000.zip", 4),
	}}
	deleted, err := applyRetentionOn(t, f, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 || deleted[0] != "miaomiaowu-backup-20260101-000000.zip" ||
		deleted[1] != "miaomiaowu-backup-20260102-000000.zip" {
		t.Fatalf("应删最旧两份,实际删了 %v", deleted)
	}
}

// **绝不能碰不是本系统产生的文件**。目标目录/桶经常不是专用的,
// 误删用户自己的东西比少删几份备份严重得多。
func TestApplyRetentionNeverTouchesForeignFiles(t *testing.T) {
	f := &fakeDest{objects: []Object{
		obj("我的照片.zip", 1),
		obj("important-database.zip", 1),
		obj("miaomiaowu-backup-20260101-000000.zip", 1),
		obj("miaomiaowu-backup-20260102-000000.zip", 2),
		obj("miaomiaowu-backup-20260103-000000.zip", 3),
	}}
	deleted, err := applyRetentionOn(t, f, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range deleted {
		if !IsManagedBackup(name) {
			t.Fatalf("删了非本系统的文件: %s", name)
		}
	}
	if len(deleted) != 2 {
		t.Fatalf("应只删 2 份本系统备份,实际 %v", deleted)
	}
}

// keep<=0 = 不清理;份数不超过 keep 也不删。
func TestApplyRetentionNoops(t *testing.T) {
	for _, keep := range []int{0, -1} {
		f := &fakeDest{objects: []Object{obj("miaomiaowu-backup-20260101-000000.zip", 1)}}
		if d, _ := applyRetentionOn(t, f, keep); len(d) != 0 {
			t.Errorf("keep=%d 不该删任何东西,实际 %v", keep, d)
		}
	}
	f := &fakeDest{objects: []Object{obj("miaomiaowu-backup-20260101-000000.zip", 1)}}
	if d, _ := applyRetentionOn(t, f, 3); len(d) != 0 {
		t.Errorf("份数少于 keep 不该删,实际 %v", d)
	}
}

// Google Drive 的对象带 ID、Name 仍是纯文件名 —— 拼进 Name 会让
// IsManagedBackup 判不出来,保留策略静默失效。
func TestApplyRetentionWorksWithIDBearingObjects(t *testing.T) {
	f := &fakeDest{kind: "gdrive", objects: []Object{
		{Name: "miaomiaowu-backup-20260101-000000.zip", ID: "drive-id-1", Modified: day(1)},
		{Name: "miaomiaowu-backup-20260102-000000.zip", ID: "drive-id-2", Modified: day(2)},
	}}
	deleted, err := applyRetentionOn(t, f, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "miaomiaowu-backup-20260101-000000.zip" {
		t.Fatalf("带 ID 的对象没被正确清理: %v", deleted)
	}
}

func TestIsManagedBackup(t *testing.T) {
	yes := []string{
		"miaomiaowu-backup-20260101-000000.zip",
		"/some/dir/miaomiaowu-backup-20260101-000000.zip",
	}
	no := []string{
		"", "backup.zip", "miaomiaowu-backup-20260101-000000.zip.tmp",
		"my-miaomiaowu-backup-20260101-000000.zip", "miaomiaowu-backup-20260101-000000",
	}
	for _, n := range yes {
		if !IsManagedBackup(n) {
			t.Errorf("%q 应判为本系统备份", n)
		}
	}
	for _, n := range no {
		if IsManagedBackup(n) {
			t.Errorf("%q 不该判为本系统备份", n)
		}
	}
}

func applyRetentionOn(t *testing.T, f *fakeDest, keep int) ([]string, error) {
	t.Helper()
	d, err := asDestination(f)
	if err != nil {
		return nil, err
	}
	return ApplyRetention(context.Background(), d, keep)
}

// asDestination 把 fakeDest 适配成 Destination(Put 签名不同,测试里用不到)。
func asDestination(f *fakeDest) (Destination, error) {
	if f == nil {
		return nil, errors.New("nil")
	}
	return &fakeAdapter{f}, nil
}

type fakeAdapter struct{ f *fakeDest }

func (a *fakeAdapter) Kind() string                                        { return a.f.Kind() }
func (a *fakeAdapter) Put(context.Context, string, io.Reader, int64) error { return nil }
func (a *fakeAdapter) List(ctx context.Context) ([]Object, error)          { return a.f.List(ctx) }
func (a *fakeAdapter) Delete(ctx context.Context, o Object) error          { return a.f.Delete(ctx, o) }
