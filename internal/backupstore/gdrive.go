package backupstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GDriveConfig 是 Google Drive 目标的配置。
//
// 用 refresh_token 而不是 service account:个人网盘的配额挂在个人账号上,
// service account 传上去的文件占的是它自己的(几乎为 0 的)配额,传大备份必然失败。
type GDriveConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	// FolderID 是目标文件夹(Drive 网页地址栏里 folders/ 后面那段)。空 = 云盘根目录。
	FolderID string
}

type gdriveDest struct {
	cfg GDriveConfig

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// 三个 Google 端点。**是 var 不是 const**:测试要把它们指向本地假服务器,
// 否则这几条路径只能靠读代码判断对错。生产运行时不会改动它们。
var (
	gdriveTokenURL  = "https://oauth2.googleapis.com/token"
	gdriveUploadURL = "https://www.googleapis.com/upload/drive/v3/files"
	gdriveFilesURL  = "https://www.googleapis.com/drive/v3/files"
)

// NewGDrive 建一个 Google Drive 目标。
func NewGDrive(cfg GDriveConfig) (Destination, error) {
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, fmt.Errorf("%w: Google Drive 缺少 client_id / client_secret", ErrNotConfigured)
	}
	if strings.TrimSpace(cfg.RefreshToken) == "" {
		return nil, fmt.Errorf("%w: Google Drive 缺少 refresh_token", ErrNotConfigured)
	}
	return &gdriveDest{cfg: cfg}, nil
}

func (d *gdriveDest) Kind() string { return "gdrive" }

// token 取一个可用的 access token,过期前 1 分钟就提前换,避免长上传中途失效。
func (d *gdriveDest) token(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.accessToken != "" && time.Now().Before(d.expiresAt.Add(-time.Minute)) {
		return d.accessToken, nil
	}
	form := url.Values{
		"client_id":     {d.cfg.ClientID},
		"client_secret": {d.cfg.ClientSecret},
		"refresh_token": {d.cfg.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gdriveTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("换取 Google 访问令牌失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", errorFromResponse("换取 Google 访问令牌", resp)
	}
	defer drainAndClose(resp)
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("解析 Google 令牌响应失败: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("Google 未返回访问令牌(refresh_token 可能已失效)")
	}
	d.accessToken = out.AccessToken
	d.expiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return d.accessToken, nil
}

func (d *gdriveDest) authorized(ctx context.Context, method, urlStr string, body io.Reader) (*http.Request, error) {
	tok, err := d.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return req, nil
}

func (d *gdriveDest) Put(ctx context.Context, name string, r io.Reader, size int64) error {
	// multipart/related:第一段是元数据 JSON,第二段是文件内容。
	// **元数据必须在前**,Drive 按顺序解析;而且这里用 io.Pipe 流式拼,
	// 不把几个 GB 的备份读进内存(标准库 multipart.Writer 同样是流式的)。
	meta := map[string]any{"name": name}
	if strings.TrimSpace(d.cfg.FolderID) != "" {
		meta["parents"] = []string{strings.TrimSpace(d.cfg.FolderID)}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	const boundary = "miaomiaowu-backup-boundary-7f3a1c"
	head := "--" + boundary + "\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n" +
		string(metaJSON) + "\r\n--" + boundary + "\r\nContent-Type: application/zip\r\n\r\n"
	tail := "\r\n--" + boundary + "--\r\n"

	body := io.MultiReader(strings.NewReader(head), r, strings.NewReader(tail))
	req, err := d.authorized(ctx, http.MethodPost, gdriveUploadURL+"?uploadType=multipart&fields=id", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)
	if size >= 0 {
		req.ContentLength = int64(len(head)) + size + int64(len(tail))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传到 Google Drive 失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromResponse("上传到 Google Drive", resp)
	}
	drainAndClose(resp)
	return nil
}

func (d *gdriveDest) List(ctx context.Context) ([]Object, error) {
	var out []Object
	pageToken := ""
	for {
		q := url.Values{}
		// trashed=false:回收站里的不算,否则保留策略会把已删的也数进份数。
		query := "trashed=false and name contains '" + backupNamePrefix + "'"
		if strings.TrimSpace(d.cfg.FolderID) != "" {
			query += " and '" + strings.TrimSpace(d.cfg.FolderID) + "' in parents"
		}
		q.Set("q", query)
		q.Set("fields", "nextPageToken,files(id,name,size,modifiedTime)")
		q.Set("pageSize", "200")
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		req, err := d.authorized(ctx, http.MethodGet, gdriveFilesURL+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("列出 Google Drive 文件失败: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, errorFromResponse("列出 Google Drive 文件", resp)
		}
		var parsed struct {
			NextPageToken string `json:"nextPageToken"`
			Files         []struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				Size         string `json:"size"`
				ModifiedTime string `json:"modifiedTime"`
			} `json:"files"`
		}
		derr := json.NewDecoder(resp.Body).Decode(&parsed)
		drainAndClose(resp)
		if derr != nil {
			return nil, fmt.Errorf("解析 Google Drive 响应失败: %w", derr)
		}
		for _, f := range parsed.Files {
			if !IsManagedBackup(f.Name) {
				continue
			}
			// Drive 按文件 id 删除,不是按名字 —— id 放独立字段。
			// **不能拼进 Name**:Name 拼了 id 就不再以 .zip 结尾,ApplyRetention
			// 里的 IsManagedBackup 会把它们全判成"别人的文件",保留策略静默失效。
			obj := Object{Name: f.Name, ID: f.ID}
			if t, err := time.Parse(time.RFC3339, f.ModifiedTime); err == nil {
				obj.Modified = t
			}
			out = append(out, obj)
		}
		if parsed.NextPageToken == "" {
			return out, nil
		}
		pageToken = parsed.NextPageToken
	}
}

func (d *gdriveDest) Delete(ctx context.Context, o Object) error {
	id := strings.TrimSpace(o.ID)
	if id == "" {
		// 没带 id 就按名字查一次。保留策略走的是 List → Delete,总是带 id 的;
		// 但连通性探针(Probe)上传的是临时文件、拿不到 id,而 Drive 只能按 id 删 ——
		// 没有这条回退,「测试连接」在 Google Drive 上必然报删除失败。
		found, err := d.findIDByName(ctx, o.Name)
		if err != nil {
			return err
		}
		if found == "" {
			return nil // 本来就不在,删除的目的已达成
		}
		id = found
	}
	req, err := d.authorized(ctx, http.MethodDelete, gdriveFilesURL+"/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("删除 Google Drive 文件失败: %w", err)
	}
	if resp.StatusCode != http.StatusNotFound && (resp.StatusCode < 200 || resp.StatusCode > 299) {
		return errorFromResponse("删除 Google Drive 文件", resp)
	}
	drainAndClose(resp)
	return nil
}

// findIDByName 按文件名查 Drive 文件 id,查不到返回空串。
func (d *gdriveDest) findIDByName(ctx context.Context, name string) (string, error) {
	q := url.Values{}
	query := "trashed=false and name = '" + strings.ReplaceAll(name, "'", "\\'") + "'"
	if strings.TrimSpace(d.cfg.FolderID) != "" {
		query += " and '" + strings.TrimSpace(d.cfg.FolderID) + "' in parents"
	}
	q.Set("q", query)
	q.Set("fields", "files(id,name)")
	q.Set("pageSize", "10")
	req, err := d.authorized(ctx, http.MethodGet, gdriveFilesURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询 Google Drive 文件失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", errorFromResponse("查询 Google Drive 文件", resp)
	}
	var parsed struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	derr := json.NewDecoder(resp.Body).Decode(&parsed)
	drainAndClose(resp)
	if derr != nil {
		return "", fmt.Errorf("解析 Google Drive 响应失败: %w", derr)
	}
	for _, f := range parsed.Files {
		if f.Name == name {
			return f.ID, nil
		}
	}
	return "", nil
}
