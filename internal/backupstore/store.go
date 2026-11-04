// Package backupstore 把备份产物投递到远端(WebDAV / S3 兼容 / Google Drive)。
//
// 三种后端都只用标准库实现,不引 SDK:S3 的 SigV4 是一段确定性的 HMAC 链,
// Google Drive 走 refresh_token 换 access_token + multipart 上传,WebDAV 就是
// HTTP PUT/PROPFIND/DELETE。为此多引三个 SDK 会显著扩大供应链面,而这个二进制
// 是要跑在别人服务器上的。
package backupstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Object 是远端上的一份备份。
type Object struct {
	Name     string
	Size     int64
	Modified time.Time
	// ID 是后端自己的对象标识(Google Drive 按 file id 删除,不是按名字)。
	// 其余后端留空,按 Name 删。**不要把 id 拼进 Name** —— Name 要保持是纯文件名,
	// IsManagedBackup 和保留策略都按它判断。
	ID string
}

// Destination 是一个投递目标。实现必须是并发安全的(调度器与「立即备份」可能同时用)。
type Destination interface {
	// Kind 返回后端类型(webdav / s3 / gdrive),用于日志与错误信息。
	Kind() string
	// Put 上传一份备份。size < 0 表示长度未知。
	Put(ctx context.Context, name string, r io.Reader, size int64) error
	// List 列出该目标下由本系统产生的备份(按修改时间升序)。
	List(ctx context.Context) ([]Object, error)
	// Delete 删除一份备份。传整个 Object 而不是名字:Google Drive 要用 ID。
	Delete(ctx context.Context, o Object) error
}

// ErrNotConfigured 表示这个目标没配置完整,不该被调度。
var ErrNotConfigured = errors.New("backup destination not configured")

// httpClient 给三个后端共用。超时给得宽:备份动辄几个 GB,上传慢是常态,
// 但也不能没有上限 —— 卡死的上传会让调度器永远进不到下一轮。
var httpClient = &http.Client{Timeout: 2 * time.Hour}

// SetHTTPClient 供测试替换。
func SetHTTPClient(c *http.Client) { httpClient = c }

// backupNamePrefix 是本系统产生的备份文件名前缀。保留策略只动带这个前缀的文件 ——
// 目标目录/桶可能不是专用的,误删用户自己的东西比少删几份备份严重得多。
const backupNamePrefix = "miaomiaowu-backup-"

// IsManagedBackup 判断某个文件名是否由本系统产生。
func IsManagedBackup(name string) bool {
	base := name
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return strings.HasPrefix(base, backupNamePrefix) && strings.HasSuffix(base, ".zip")
}

// ApplyRetention 删除超出保留份数的旧备份,返回被删掉的文件名。
//
// keep <= 0 表示不清理。只考虑 IsManagedBackup 的文件。
func ApplyRetention(ctx context.Context, d Destination, keep int) ([]string, error) {
	if keep <= 0 {
		return nil, nil
	}
	objects, err := d.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("列出远端备份失败: %w", err)
	}
	managed := make([]Object, 0, len(objects))
	for _, o := range objects {
		if IsManagedBackup(o.Name) {
			managed = append(managed, o)
		}
	}
	if len(managed) <= keep {
		return nil, nil
	}
	// 按时间升序,删最旧的。时间相同时按名字排,保证结果稳定可预期
	// (文件名里带时间戳,字典序即时间序)。
	sort.Slice(managed, func(i, j int) bool {
		if !managed[i].Modified.Equal(managed[j].Modified) {
			return managed[i].Modified.Before(managed[j].Modified)
		}
		return managed[i].Name < managed[j].Name
	})
	var deleted []string
	for _, o := range managed[:len(managed)-keep] {
		if err := d.Delete(ctx, o); err != nil {
			// 删一份失败不该中断其余清理:磁盘/配额问题往往只影响个别对象。
			return deleted, fmt.Errorf("删除旧备份 %s 失败: %w", o.Name, err)
		}
		deleted = append(deleted, o.Name)
	}
	return deleted, nil
}

// drainAndClose 读干并关闭响应体 —— 不读干净会让连接无法复用。
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

// errorFromResponse 把非 2xx 响应转成带响应体片段的错误(截断,避免把整个页面塞进日志)。
func errorFromResponse(action string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()
	snippet := strings.TrimSpace(string(body))
	if snippet == "" {
		return fmt.Errorf("%s 失败: HTTP %d", action, resp.StatusCode)
	}
	return fmt.Errorf("%s 失败: HTTP %d — %s", action, resp.StatusCode, snippet)
}

// Probe 验证一个目标是否真的可用:上传一个小探针文件、列一次、再删掉。
//
// 只测「能连上」是不够的 —— 最常见的配错是**能连但没有写权限**(WebDAV 目录只读、
// S3 密钥只给了 GetObject、Drive 的 folder 不属于这个账号)。那类问题如果留到
// 定时任务里才暴露,往往是几天后才发现一份备份都没有。所以探针要走完整的写—读—删。
func Probe(ctx context.Context, d Destination) error {
	// 探针名字刻意**不带** backupNamePrefix:带了会被保留策略当成一份真备份计数,
	// 万一删除失败还会挤掉一份真的。
	name := fmt.Sprintf("miaomiaowu-probe-%d.txt", time.Now().UnixNano())
	payload := []byte("miaomiaowu backup destination probe")
	if err := d.Put(ctx, name, strings.NewReader(string(payload)), int64(len(payload))); err != nil {
		return fmt.Errorf("写入测试失败: %w", err)
	}
	// 列一次确认读权限也在(有些后端 PUT 通过但 LIST 被拒,保留策略届时会一直报错)。
	if _, err := d.List(ctx); err != nil {
		// 读不了不阻断:写成功已经说明备份能落地,列目录只影响清理旧备份。
		_ = d.Delete(ctx, Object{Name: name})
		return fmt.Errorf("写入成功但列目录失败(备份能上传,但无法自动清理旧备份): %w", err)
	}
	if err := d.Delete(ctx, Object{Name: name}); err != nil {
		return fmt.Errorf("写入成功但删除测试文件失败(自动清理旧备份会受影响): %w", err)
	}
	return nil
}
