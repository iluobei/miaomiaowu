package backupstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// Config 是「自动备份」的完整配置,整体存在 system_settings 的一个键里。
type Config struct {
	Enabled bool `json:"enabled"`
	// Kind: webdav / s3 / gdrive
	Kind string `json:"kind"`
	// IntervalHours 备份间隔(小时)。最小 1。
	IntervalHours int `json:"interval_hours"`
	// PreferredHour 每天固定备份时刻(本地时 0-23)。非 nil 时改为「每天该整点跑一次」(如 3=凌晨3点),
	// 忽略 IntervalHours;nil(默认/存量配置)沿用旧的按间隔行为。用指针而非 int 是为了区分「设成 0 点」
	// 与「字段缺失」—— 后者绝不能被当成凌晨 0 点误触发。用于把备份放到最空闲的时段。
	PreferredHour *int `json:"preferred_hour,omitempty"`
	// Keep 远端保留份数,<=0 表示不清理。
	Keep int `json:"keep"`

	WebDAV WebDAVConfig `json:"webdav"`
	S3     S3Config     `json:"s3"`
	GDrive GDriveConfig `json:"gdrive"`
}

// Destination 按当前配置构造投递目标。
func (c Config) Destination() (Destination, error) {
	switch strings.ToLower(strings.TrimSpace(c.Kind)) {
	case "webdav":
		return NewWebDAV(c.WebDAV)
	case "s3":
		return NewS3(c.S3)
	case "gdrive":
		return NewGDrive(c.GDrive)
	case "":
		return nil, fmt.Errorf("%w: 未选择备份目标", ErrNotConfigured)
	default:
		return nil, fmt.Errorf("%w: 未知备份目标 %q", ErrNotConfigured, c.Kind)
	}
}

// Normalize 把不合法的取值收进有效范围。
func (c *Config) Normalize() {
	c.Kind = strings.ToLower(strings.TrimSpace(c.Kind))
	if c.IntervalHours < 1 {
		c.IntervalHours = 24
	}
	if c.IntervalHours > 24*30 {
		c.IntervalHours = 24 * 30
	}
	if c.Keep < 0 {
		c.Keep = 0
	}
	if c.PreferredHour != nil && (*c.PreferredHour < 0 || *c.PreferredHour > 23) {
		c.PreferredHour = nil // 非法小时视为未设,退回按间隔
	}
}

// secretFields 列出所有需要加密存储的字段指针。新增后端时**必须**在这里登记,
// 否则那个凭据会明文落库。
func (c *Config) secretFields() []*string {
	return []*string{
		&c.WebDAV.Password,
		&c.S3.SecretKey,
		&c.GDrive.ClientSecret,
		&c.GDrive.RefreshToken,
	}
}

const secretPrefix = "enc:v1:"

// Marshal 把配置序列化成可入库的 JSON,凭据字段用 masterSeed 派生的密钥加密。
//
// 加密解决的是「**只拿到数据库**」那类泄露:库文件发给别人排障、只读库权限、
// 备份里的 SQLite 被翻。它**不解决**「连 data/ 一起拿走」—— 密钥就在
// data/backup_secret.key 里。这不是自欺:两种泄露的实际发生率差很多,而防住前者的成本
// 只是几十行。要防后者得引入用户口令或 KMS,那是另一个量级的产品决策。
func (c Config) Marshal(masterSeed []byte) (string, error) {
	dup := c
	for _, p := range dup.secretFields() {
		if *p == "" || strings.HasPrefix(*p, secretPrefix) {
			continue
		}
		enc, err := encryptSecret(masterSeed, *p)
		if err != nil {
			return "", err
		}
		*p = enc
	}
	raw, err := json.Marshal(dup)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Unmarshal 从库里读回配置并解密凭据。解不开的字段留空而不是报错 ——
// 换过密钥的部署应该能看到并重填配置,而不是整个页面打不开。
func Unmarshal(raw string, masterSeed []byte) (Config, error) {
	var c Config
	if strings.TrimSpace(raw) == "" {
		return c, nil
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, err
	}
	for _, p := range c.secretFields() {
		if !strings.HasPrefix(*p, secretPrefix) {
			continue
		}
		dec, err := decryptSecret(masterSeed, *p)
		if err != nil {
			*p = ""
			continue
		}
		*p = dec
	}
	c.Normalize()
	return c, nil
}

// Redacted 返回一份可以安全下发给前端的副本:凭据一律换成占位符。
//
// 读接口绝不回显密钥 —— 面板一旦被 XSS 或越权读到,等于连云盘一起丢了。
// 前端保存时把占位符原样回传,表示「不修改这一项」。
const SecretPlaceholder = "********"

func (c Config) Redacted() Config {
	dup := c
	for _, p := range dup.secretFields() {
		if *p != "" {
			*p = SecretPlaceholder
		}
	}
	return dup
}

// MergeSecrets 把 incoming 里仍是占位符的凭据字段换成 current 的真实值。
func MergeSecrets(incoming, current Config) Config {
	in, cur := incoming, current
	ip, cp := in.secretFields(), cur.secretFields()
	for i := range ip {
		if *ip[i] == SecretPlaceholder {
			*ip[i] = *cp[i]
		}
	}
	return in
}

// deriveSecretKey 从密钥种子派生一把 AES-256 密钥。
func deriveSecretKey(masterSeed []byte) ([]byte, error) {
	if len(masterSeed) == 0 {
		return nil, errors.New("备份凭据密钥不可用,无法加解密备份凭据")
	}
	r := hkdf.New(sha256.New, masterSeed, nil, []byte("miaomiaowu-backup-destination-secret-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptSecret(masterSeed []byte, plain string) (string, error) {
	key, err := deriveSecretKey(masterSeed)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(plain), nil)
	return secretPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptSecret(masterSeed []byte, stored string) (string, error) {
	key, err := deriveSecretKey(masterSeed)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, secretPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("密文过短")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
