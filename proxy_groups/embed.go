package proxygroups

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultFileName  = "proxy-groups.json"
	DefaultSourceURL = "https://gh-proxy.com/https://raw.githubusercontent.com/Jimleerx/miaomiaowu/refs/heads/main/proxy_groups/proxy-groups.default.json"
)

var (
	ErrInvalidConfig  = errors.New("proxy groups config is invalid")
	ErrDownloadFailed = errors.New("proxy groups config download failed")
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// 打包保留默认配置文件
func Ensure(targetDir string) (string, error) {
	if targetDir == "" {
		targetDir = "."
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(targetDir, defaultFileName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := SyncFromSource(path, ""); err != nil {
			return "", fmt.Errorf(
				"failed to bootstrap proxy groups config at %s: %w. "+
					"You can set PROXY_GROUPS_SOURCE_URL environment variable or manually place the file",
				path,
				err,
			)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	return path, nil
}

// 加载配置文件
func Load(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if v == nil {
		return nil
	}

	return json.Unmarshal(data, v)
}

// 保存替换配置文件
func Save(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

// 使用远程配置替换本地配置
func SyncFromSource(path string, overrideURL string) error {
	if path == "" {
		return errors.New("target path must be provided")
	}

	resolvedURL := ResolveSourceURL(overrideURL)

	data, err := downloadConfig(resolvedURL)
	if err != nil {
		return err
	}

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure config directory: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // Clean up temp file on error
		return fmt.Errorf("replace config: %w", err)
	}

	return nil
}

// 支持通过环境变量覆盖下载地址
func ResolveSourceURL(overrideURL string) string {
	if overrideURL != "" {
		return overrideURL
	}

	if env := os.Getenv("PROXY_GROUPS_SOURCE_URL"); env != "" {
		return env
	}

	return DefaultSourceURL
}

// 下载github配置
func downloadConfig(sourceURL string) ([]byte, error) {
	resp, err := httpClient.Get(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status %d from %s", ErrDownloadFailed, resp.StatusCode, sourceURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}

	return data, nil
}
