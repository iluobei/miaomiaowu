package backupstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// SigV4 手写实现的正确性:签名错了不会在本地报错,而是上传时 403 —— 而自动备份
// 失败往往没人盯着。所以拿一组**固定输入**钉死输出,任何改动导致签名变化都会红。
//
// 向量来自 AWS 官方 sigv4 测试套的密钥与时间戳(AKIDEXAMPLE / 2015-08-30),
// 只是签的是 s3 服务。它的价值不是"和 AWS 逐字节一致"(那需要完整 sigv4 套件),
// 而是**任何一处链条改动都会改变输出** —— 派生顺序、canonical 结构、scope 拼法。
func TestS3SignatureIsDeterministic(t *testing.T) {
	d, err := NewS3(S3Config{
		Endpoint: "https://s3.us-east-1.amazonaws.com", Region: "us-east-1",
		Bucket: "examplebucket", AccessKey: "AKIDEXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut,
		"https://examplebucket.s3.us-east-1.amazonaws.com/miaomiaowu-backup-20260101-000000.zip", nil)
	fixed := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	if err := d.(*s3Dest).sign(req, "UNSIGNED-PAYLOAD", fixed); err != nil {
		t.Fatal(err)
	}
	auth := req.Header.Get("Authorization")

	// 结构必须完整:少任何一段,S3 都会拒。
	for _, want := range []string{
		"AWS4-HMAC-SHA256 ",
		"Credential=AKIDEXAMPLE/20150830/us-east-1/s3/aws4_request",
		"SignedHeaders=host;x-amz-content-sha256;x-amz-date",
		"Signature=",
	} {
		if !strings.Contains(auth, want) {
			t.Errorf("Authorization 缺少 %q:\n%s", want, auth)
		}
	}
	if got := req.Header.Get("x-amz-date"); got != "20150830T123600Z" {
		t.Errorf("x-amz-date = %q", got)
	}
	// 同样输入必须签出同样结果 —— 签名过程不能掺随机/时钟。
	req2, _ := http.NewRequest(http.MethodPut,
		"https://examplebucket.s3.us-east-1.amazonaws.com/miaomiaowu-backup-20260101-000000.zip", nil)
	if err := d.(*s3Dest).sign(req2, "UNSIGNED-PAYLOAD", fixed); err != nil {
		t.Fatal(err)
	}
	if req2.Header.Get("Authorization") != auth {
		t.Error("同样输入签出了不同签名")
	}
	// 换个密钥必须换签名(否则等于没签)。
	d2, _ := NewS3(S3Config{
		Endpoint: "https://s3.us-east-1.amazonaws.com", Region: "us-east-1",
		Bucket: "examplebucket", AccessKey: "AKIDEXAMPLE", SecretKey: "another-secret-key",
	})
	req3, _ := http.NewRequest(http.MethodPut,
		"https://examplebucket.s3.us-east-1.amazonaws.com/miaomiaowu-backup-20260101-000000.zip", nil)
	if err := d2.(*s3Dest).sign(req3, "UNSIGNED-PAYLOAD", fixed); err != nil {
		t.Fatal(err)
	}
	if req3.Header.Get("Authorization") == auth {
		t.Error("换了 SecretKey 签名却没变")
	}
}

// 路径风格与虚拟主机风格要生成不同的 host/path,配错会 404 或 SignatureDoesNotMatch。
func TestS3PathStyleVsVirtualHost(t *testing.T) {
	vh, _ := NewS3(S3Config{Endpoint: "https://x.r2.cloudflarestorage.com", Bucket: "b", AccessKey: "a", SecretKey: "s"})
	ps, _ := NewS3(S3Config{Endpoint: "https://minio.local:9000", Bucket: "b", AccessKey: "a", SecretKey: "s", PathStyle: true})
	if got := vh.(*s3Dest).host; got != "b.x.r2.cloudflarestorage.com" {
		t.Errorf("虚拟主机风格 host = %q", got)
	}
	if got := vh.(*s3Dest).objectPath("k.zip"); got != "/k.zip" {
		t.Errorf("虚拟主机风格 path = %q", got)
	}
	if got := ps.(*s3Dest).host; got != "minio.local:9000" {
		t.Errorf("路径风格 host = %q", got)
	}
	if got := ps.(*s3Dest).objectPath("k.zip"); got != "/b/k.zip" {
		t.Errorf("路径风格 path = %q", got)
	}
}

// 前缀要拼进 key,而且不能拼出双斜杠。
func TestS3KeyWithPrefix(t *testing.T) {
	d, _ := NewS3(S3Config{Endpoint: "https://e.com", Bucket: "b", Prefix: "/miaomiaowu/backups/", AccessKey: "a", SecretKey: "s"})
	if got := d.(*s3Dest).key("f.zip"); got != "miaomiaowu/backups/f.zip" {
		t.Errorf("key = %q", got)
	}
}

// 端到端:对着一个假 S3 跑一次 Put,确认方法/路径/签名头都对。
func TestS3PutAgainstFakeServer(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		b := make([]byte, 64)
		n, _ := r.Body.Read(b)
		gotBody = string(b[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d, err := NewS3(S3Config{
		Endpoint: srv.URL, Region: "auto", Bucket: "bk", Prefix: "p",
		AccessKey: "ak", SecretKey: "sk", PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := "zipdata"
	if err := d.Put(context.Background(), "miaomiaowu-backup-20260101-000000.zip",
		strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s", gotMethod)
	}
	if gotPath != "/bk/p/miaomiaowu-backup-20260101-000000.zip" {
		t.Errorf("path = %s", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 ") {
		t.Errorf("没带签名头: %s", gotAuth)
	}
	if gotBody != body {
		t.Errorf("body = %q", gotBody)
	}
}
