package backupstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3Config 是 S3 兼容目标的配置。覆盖 AWS S3 / Cloudflare R2 / Backblaze B2 /
// MinIO / 阿里云 OSS 等所有支持 SigV4 的服务。
type S3Config struct {
	Endpoint  string // 如 https://<account>.r2.cloudflarestorage.com;留空则用 AWS 官方端点
	Region    string // R2 用 auto
	Bucket    string
	Prefix    string // 桶内目录前缀,可空
	AccessKey string
	SecretKey string
	// PathStyle 强制路径风格(MinIO / 自建常用)。false = 虚拟主机风格。
	PathStyle bool
}

type s3Dest struct {
	cfg  S3Config
	host string
	base *url.URL
}

// NewS3 建一个 S3 兼容目标。
func NewS3(cfg S3Config) (Destination, error) {
	cfg.Bucket = strings.Trim(strings.TrimSpace(cfg.Bucket), "/")
	cfg.Prefix = strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	cfg.Region = strings.TrimSpace(cfg.Region)
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("%w: S3 bucket 为空", ErrNotConfigured)
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("%w: S3 密钥为空", ErrNotConfigured)
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		endpoint = "https://s3." + cfg.Region + ".amazonaws.com"
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%w: S3 endpoint 无效", ErrNotConfigured)
	}
	d := &s3Dest{cfg: cfg, base: u, host: u.Host}
	if !cfg.PathStyle {
		// 虚拟主机风格:bucket 进域名。R2/AWS 都推荐这个。
		d.host = cfg.Bucket + "." + u.Host
	}
	return d, nil
}

func (d *s3Dest) Kind() string { return "s3" }

func (d *s3Dest) key(name string) string {
	if d.cfg.Prefix == "" {
		return name
	}
	return d.cfg.Prefix + "/" + name
}

// objectPath 返回请求路径(SigV4 里要用的 canonical URI)。
func (d *s3Dest) objectPath(key string) string {
	if d.cfg.PathStyle {
		return "/" + d.cfg.Bucket + "/" + key
	}
	return "/" + key
}

func (d *s3Dest) endpointURL(path, rawQuery string) string {
	u := url.URL{Scheme: d.base.Scheme, Host: d.host, Path: path, RawQuery: rawQuery}
	return u.String()
}

func (d *s3Dest) Put(ctx context.Context, name string, r io.Reader, size int64) error {
	// S3 要在签名里带 body 的 sha256。备份是几个 GB 的文件,不能读进内存算 —— 用
	// UNSIGNED-PAYLOAD:SigV4 明确支持,https 下 body 完整性由 TLS 保证。
	path := d.objectPath(d.key(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, d.endpointURL(path, ""), r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/zip")
	if size >= 0 {
		req.ContentLength = size
	}
	if err := d.sign(req, "UNSIGNED-PAYLOAD", time.Now().UTC()); err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传到 S3 失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromResponse("上传到 S3", resp)
	}
	drainAndClose(resp)
	return nil
}

type s3ListResult struct {
	Contents []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

func (d *s3Dest) List(ctx context.Context) ([]Object, error) {
	var out []Object
	token := ""
	for {
		q := url.Values{}
		q.Set("list-type", "2")
		if d.cfg.Prefix != "" {
			q.Set("prefix", d.cfg.Prefix+"/")
		}
		if token != "" {
			q.Set("continuation-token", token)
		}
		path := "/"
		if d.cfg.PathStyle {
			path = "/" + d.cfg.Bucket
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.endpointURL(path, encodeCanonicalQuery(q)), nil)
		if err != nil {
			return nil, err
		}
		if err := d.sign(req, emptyPayloadHash, time.Now().UTC()); err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("列出 S3 对象失败: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, errorFromResponse("列出 S3 对象", resp)
		}
		var parsed s3ListResult
		derr := xml.NewDecoder(resp.Body).Decode(&parsed)
		drainAndClose(resp)
		if derr != nil {
			return nil, fmt.Errorf("解析 S3 响应失败: %w", derr)
		}
		for _, c := range parsed.Contents {
			base := c.Key
			if i := strings.LastIndex(base, "/"); i >= 0 {
				base = base[i+1:]
			}
			if !IsManagedBackup(base) {
				continue
			}
			obj := Object{Name: base, Size: c.Size}
			if t, err := time.Parse(time.RFC3339, c.LastModified); err == nil {
				obj.Modified = t
			}
			out = append(out, obj)
		}
		if !parsed.IsTruncated || parsed.NextContinuationToken == "" {
			return out, nil
		}
		token = parsed.NextContinuationToken
	}
}

func (d *s3Dest) Delete(ctx context.Context, o Object) error {
	path := d.objectPath(d.key(o.Name))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, d.endpointURL(path, ""), nil)
	if err != nil {
		return err
	}
	if err := d.sign(req, emptyPayloadHash, time.Now().UTC()); err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("删除 S3 对象失败: %w", err)
	}
	// S3 删除不存在的对象也回 204,这里一并把 404 当成功。
	if resp.StatusCode != http.StatusNotFound && (resp.StatusCode < 200 || resp.StatusCode > 299) {
		return errorFromResponse("删除 S3 对象", resp)
	}
	drainAndClose(resp)
	return nil
}

// emptyPayloadHash 是空 body 的 sha256(SigV4 规范里的固定值)。
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// sign 按 AWS SigV4 给请求加 Authorization 头。
//
// 手写而不是引 SDK:签名是一段确定性的 HMAC 链,规范公开且有官方测试向量可对;
// 为此引一整套 aws-sdk-go-v2(几十个间接依赖)对一个跑在别人服务器上的二进制
// 是不划算的供应链开销。正确性由 s3_sigv4_test.go 里的官方向量守住。
func (d *s3Dest) sign(req *http.Request, payloadHash string, now time.Time) error {
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.Header.Set("Host", req.URL.Host)
	req.Host = req.URL.Host
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	canonicalHeaders, signedHeaders := canonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := strings.Join([]string{dateStamp, d.cfg.Region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(sha256Sum([]byte(canonicalRequest))),
	}, "\n")

	key := hmacSHA256([]byte("AWS4"+d.cfg.SecretKey), dateStamp)
	key = hmacSHA256(key, d.cfg.Region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		d.cfg.AccessKey, scope, signedHeaders, signature))
	return nil
}

// canonicalHeaders 生成 SigV4 的 canonical headers 与 signed headers 列表。
func canonicalHeaders(req *http.Request) (string, string) {
	names := make([]string, 0, len(req.Header)+1)
	values := map[string]string{}
	for name, vs := range req.Header {
		lower := strings.ToLower(name)
		// 只签必要的头:Content-Type 等由中间代理改写的头进签名会导致间歇性 403。
		if lower != "host" && lower != "x-amz-date" && lower != "x-amz-content-sha256" {
			continue
		}
		names = append(names, lower)
		values[lower] = strings.TrimSpace(strings.Join(vs, ","))
	}
	if _, ok := values["host"]; !ok {
		names = append(names, "host")
		values["host"] = req.URL.Host
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, n := range names {
		sb.WriteString(n)
		sb.WriteString(":")
		sb.WriteString(values[n])
		sb.WriteString("\n")
	}
	return sb.String(), strings.Join(names, ";")
}

// encodeCanonicalQuery 按 SigV4 要求的顺序编码查询串(键排序、RFC3986 转义)。
// url.Values.Encode 已经按键排序,但它把空格编成 "+",而 SigV4 要 "%20"。
func encodeCanonicalQuery(q url.Values) string {
	return strings.ReplaceAll(q.Encode(), "+", "%20")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}
