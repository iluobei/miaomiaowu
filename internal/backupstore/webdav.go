package backupstore

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebDAVConfig 是 WebDAV 目标的配置。覆盖 Nextcloud / 坚果云 / 群晖 / InfiniCloud 等。
type WebDAVConfig struct {
	// URL 是备份目录的完整地址,例如 https://dav.example.com/remote.php/dav/files/me/miaomiaowu-backup
	URL      string
	Username string
	Password string
}

type webdavDest struct{ cfg WebDAVConfig }

// NewWebDAV 建一个 WebDAV 目标。
func NewWebDAV(cfg WebDAVConfig) (Destination, error) {
	cfg.URL = strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if cfg.URL == "" {
		return nil, fmt.Errorf("%w: WebDAV 地址为空", ErrNotConfigured)
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: WebDAV 地址无效(需要 http/https 完整地址)", ErrNotConfigured)
	}
	return &webdavDest{cfg: cfg}, nil
}

func (d *webdavDest) Kind() string { return "webdav" }

func (d *webdavDest) objectURL(name string) string {
	return d.cfg.URL + "/" + url.PathEscape(name)
}

func (d *webdavDest) newRequest(ctx context.Context, method, urlStr string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, err
	}
	if d.cfg.Username != "" || d.cfg.Password != "" {
		req.SetBasicAuth(d.cfg.Username, d.cfg.Password)
	}
	return req, nil
}

func (d *webdavDest) Put(ctx context.Context, name string, r io.Reader, size int64) error {
	req, err := d.newRequest(ctx, http.MethodPut, d.objectURL(name), r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/zip")
	// 显式给长度:不给的话 Go 会用 chunked,而不少 WebDAV 服务端(尤其反代后面的)
	// 对 chunked 上传直接 411/501。
	if size >= 0 {
		req.ContentLength = size
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传到 WebDAV 失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromResponse("上传到 WebDAV", resp)
	}
	drainAndClose(resp)
	return nil
}

// propfindResponse 是 PROPFIND 的多状态响应。只取需要的三个字段。
type propfindResponse struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Prop struct {
				ContentLength int64  `xml:"getcontentlength"`
				LastModified  string `xml:"getlastmodified"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func (d *webdavDest) List(ctx context.Context) ([]Object, error) {
	req, err := d.newRequest(ctx, "PROPFIND", d.cfg.URL+"/", nil)
	if err != nil {
		return nil, err
	}
	// Depth: 1 = 只列本目录,不递归。不给这个头有些服务端会当成无限递归。
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("列出 WebDAV 目录失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errorFromResponse("列出 WebDAV 目录", resp)
	}
	defer drainAndClose(resp)

	var parsed propfindResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("解析 WebDAV 响应失败: %w", err)
	}
	out := make([]Object, 0, len(parsed.Responses))
	for _, r := range parsed.Responses {
		name := hrefBase(r.Href)
		if name == "" || !IsManagedBackup(name) {
			continue
		}
		obj := Object{Name: name}
		for _, ps := range r.Propstat {
			if ps.Prop.ContentLength > 0 {
				obj.Size = ps.Prop.ContentLength
			}
			if ps.Prop.LastModified != "" {
				if t, err := time.Parse(time.RFC1123, ps.Prop.LastModified); err == nil {
					obj.Modified = t
				}
			}
		}
		out = append(out, obj)
	}
	return out, nil
}

func (d *webdavDest) Delete(ctx context.Context, o Object) error {
	req, err := d.newRequest(ctx, http.MethodDelete, d.objectURL(o.Name), nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("删除 WebDAV 文件失败: %w", err)
	}
	// 404 当成功:目标本来就不在了,保留策略的目的已经达到。
	if resp.StatusCode == http.StatusNotFound {
		drainAndClose(resp)
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromResponse("删除 WebDAV 文件", resp)
	}
	drainAndClose(resp)
	return nil
}

// hrefBase 从 PROPFIND 返回的 href 里取文件名。
//
// href 可能是完整 URL 也可能是绝对路径,而且是 **URL 编码过的** —— 文件名里的
// 冒号/空格解不开就匹配不上前缀,保留策略会把所有备份都当成"别人的文件"而不清理。
func hrefBase(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if u, err := url.Parse(href); err == nil {
		href = u.Path
	}
	href = strings.TrimRight(href, "/")
	if i := strings.LastIndex(href, "/"); i >= 0 {
		href = href[i+1:]
	}
	if decoded, err := url.PathUnescape(href); err == nil {
		return decoded
	}
	return href
}
