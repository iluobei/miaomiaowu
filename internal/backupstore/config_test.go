package backupstore

import (
	"strings"
	"testing"
)

var seed = []byte("0123456789abcdef0123456789abcdef")

func sample() Config {
	return Config{
		Enabled: true, Kind: "s3", IntervalHours: 24, Keep: 7,
		WebDAV: WebDAVConfig{URL: "https://dav.example.com/x", Username: "u", Password: "dav-pw"},
		S3:     S3Config{Endpoint: "https://e", Bucket: "b", AccessKey: "AK", SecretKey: "s3-secret-key"},
		GDrive: GDriveConfig{ClientID: "cid", ClientSecret: "csec", RefreshToken: "rtok"},
	}
}

// 凭据入库必须是密文,且能原样解回来。
func TestMarshalEncryptsSecretsAndRoundTrips(t *testing.T) {
	raw, err := sample().Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	// 明文一个都不许出现在入库串里
	for _, secret := range []string{"dav-pw", "s3-secret-key", "csec", "rtok"} {
		if strings.Contains(raw, secret) {
			t.Errorf("凭据 %q 明文落库了:\n%s", secret, raw)
		}
	}
	// 非凭据字段仍是明文(排障时要看得懂)
	for _, plain := range []string{"AK", "cid", "u", "dav.example.com"} {
		if !strings.Contains(raw, plain) {
			t.Errorf("非凭据字段 %q 不该被加密", plain)
		}
	}
	back, err := Unmarshal(raw, seed)
	if err != nil {
		t.Fatal(err)
	}
	if back.WebDAV.Password != "dav-pw" || back.S3.SecretKey != "s3-secret-key" ||
		back.GDrive.ClientSecret != "csec" || back.GDrive.RefreshToken != "rtok" {
		t.Fatalf("解密回来的凭据不对: %+v", back)
	}
}

// 换了 master key(或密钥丢了)时,凭据留空而不是让整个配置读不出来 ——
// 管理员应该能进页面看到配置并重填,而不是面板直接报错。
func TestUnmarshalWithWrongKeyBlanksSecretsButKeepsConfig(t *testing.T) {
	raw, _ := sample().Marshal(seed)
	back, err := Unmarshal(raw, []byte("totally-different-master-seed-32b"))
	if err != nil {
		t.Fatalf("换密钥后不该整体报错: %v", err)
	}
	if back.S3.SecretKey != "" || back.WebDAV.Password != "" {
		t.Error("解不开的凭据应留空")
	}
	if back.S3.Bucket != "b" || back.Kind != "s3" || !back.Enabled {
		t.Error("非凭据配置应当保留")
	}
}

// 读接口绝不回显凭据。
func TestRedactedHidesEverySecret(t *testing.T) {
	r := sample().Redacted()
	for _, got := range []string{r.WebDAV.Password, r.S3.SecretKey, r.GDrive.ClientSecret, r.GDrive.RefreshToken} {
		if got != SecretPlaceholder {
			t.Errorf("凭据没被打码: %q", got)
		}
	}
	if r.S3.AccessKey != "AK" {
		t.Error("非凭据字段不该被打码")
	}
}

// 保存时前端回传占位符 = 不修改该项;填了新值 = 覆盖。
func TestMergeSecretsKeepsUnchangedAndAcceptsNew(t *testing.T) {
	cur := sample()
	in := sample()
	in.S3.SecretKey = SecretPlaceholder // 不改
	in.WebDAV.Password = "new-dav-pw"   // 改
	merged := MergeSecrets(in, cur)
	if merged.S3.SecretKey != "s3-secret-key" {
		t.Errorf("占位符应保留原值,实际 %q", merged.S3.SecretKey)
	}
	if merged.WebDAV.Password != "new-dav-pw" {
		t.Errorf("新值应生效,实际 %q", merged.WebDAV.Password)
	}
}

// 重复 Marshal 不能把密文再加密一遍(否则读回来就是一层密文)。
func TestMarshalIsIdempotentOverAlreadyEncrypted(t *testing.T) {
	raw1, _ := sample().Marshal(seed)
	mid, _ := Unmarshal(raw1, seed)
	raw2, _ := mid.Marshal(seed)
	back, err := Unmarshal(raw2, seed)
	if err != nil {
		t.Fatal(err)
	}
	if back.S3.SecretKey != "s3-secret-key" {
		t.Errorf("二次 Marshal 后解不回原值: %q", back.S3.SecretKey)
	}
}

func TestNormalizeClampsInterval(t *testing.T) {
	c := Config{IntervalHours: 0, Keep: -5}
	c.Normalize()
	if c.IntervalHours != 24 {
		t.Errorf("间隔 0 应收敛到 24,实际 %d", c.IntervalHours)
	}
	if c.Keep != 0 {
		t.Errorf("负数保留份数应收敛到 0,实际 %d", c.Keep)
	}
	c2 := Config{IntervalHours: 99999}
	c2.Normalize()
	if c2.IntervalHours != 24*30 {
		t.Errorf("超大间隔应封顶,实际 %d", c2.IntervalHours)
	}
}

// 没有密钥种子时必须明确报错,而不是把凭据明文写进库。
func TestMarshalRefusesWithoutMasterSeed(t *testing.T) {
	if _, err := sample().Marshal(nil); err == nil {
		t.Fatal("没有密钥种子时应当报错,不能明文落库")
	}
}
