package backupstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Drive 的 List 必须返回**纯文件名**,file id 走 Object.ID。
//
// 曾经把 id 拼进 Name(name+"\x00"+id),结果 ApplyRetention 里的 IsManagedBackup
// 判不出 .zip 后缀 → 所有 Drive 备份都被当成"别人的文件" → 保留策略静默失效,
// 云盘会一直涨到配额满。这条用例就是钉死这件事。
func TestGDriveListReturnsCleanNameAndID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"files":[
			{"id":"ID-1","name":"miaomiaowu-backup-20260101-000000.zip","modifiedTime":"2026-01-01T00:00:00Z"},
			{"id":"ID-2","name":"别人的文件.zip","modifiedTime":"2026-01-02T00:00:00Z"}]}`))
	}))
	defer srv.Close()
	withGoogleEndpoints(t, srv.URL)

	d, err := NewGDrive(GDriveConfig{ClientID: "c", ClientSecret: "s", RefreshToken: "r"})
	if err != nil {
		t.Fatal(err)
	}
	objs, err := d.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Fatalf("应只认出 1 份本系统备份,实际 %d: %+v", len(objs), objs)
	}
	if objs[0].Name != "miaomiaowu-backup-20260101-000000.zip" {
		t.Errorf("Name 应是纯文件名,实际 %q", objs[0].Name)
	}
	if objs[0].ID != "ID-1" {
		t.Errorf("ID 应为 ID-1,实际 %q", objs[0].ID)
	}
	// 关键:Name 必须还能被 IsManagedBackup 认出来,否则保留策略不会动它。
	if !IsManagedBackup(objs[0].Name) {
		t.Error("List 产出的 Name 过不了 IsManagedBackup —— 保留策略会静默失效")
	}
}

// 删除必须按 id 走,没有 id 要明确报错而不是删错文件。
func TestGDriveDeleteUsesID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "token"):
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case r.Method == http.MethodGet:
			// 按名字查 id(没带 ID 的删除会先走这里)
			_, _ = w.Write([]byte(`{"files":[{"id":"ID-BY-NAME","name":"x.zip"}]}`))
		default:
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	withGoogleEndpoints(t, srv.URL)

	d, _ := NewGDrive(GDriveConfig{ClientID: "c", ClientSecret: "s", RefreshToken: "r"})
	if err := d.Delete(context.Background(), Object{Name: "x.zip", ID: "ID-9"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/ID-9") {
		t.Errorf("应按 id 删除,实际请求 %s", gotPath)
	}
	// 没带 id 时按名字查一次 —— 连通性探针上传的临时文件拿不到 id,
	// 没有这条回退,「测试连接」在 Google Drive 上必然报删除失败。
	if err := d.Delete(context.Background(), Object{Name: "x.zip"}); err != nil {
		t.Errorf("没带 id 时应按名字查找后删除,实际报错: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/ID-BY-NAME") {
		t.Errorf("应当用查到的 id 删除,实际请求 %s", gotPath)
	}
}

// withGoogleEndpoints 把三个 Google 端点指向测试服务器。
func withGoogleEndpoints(t *testing.T, base string) {
	t.Helper()
	oldToken, oldUpload, oldFiles := gdriveTokenURL, gdriveUploadURL, gdriveFilesURL
	gdriveTokenURL = base + "/token"
	gdriveUploadURL = base + "/upload"
	gdriveFilesURL = base + "/files"
	t.Cleanup(func() {
		gdriveTokenURL, gdriveUploadURL, gdriveFilesURL = oldToken, oldUpload, oldFiles
	})
}
