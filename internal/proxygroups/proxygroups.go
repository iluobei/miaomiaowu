package proxygroups

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// DefaultSourceURL 默认的远程配置地址
	DefaultSourceURL = "https://gh-proxy.com/https://raw.githubusercontent.com/Jimleerx/miaomiaowu/refs/heads/main/proxy_groups/proxy_groups.json"
)

var (
	ErrInvalidConfig  = errors.New("proxy groups config is invalid")
	ErrDownloadFailed = errors.New("proxy groups config download failed")
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// ResolveSourceURL 解析配置源地址
// 优先级: 传入参数 > 环境变量 > 默认地址
func ResolveSourceURL(overrideURL string) string {
	if overrideURL != "" {
		return overrideURL
	}

	if env := os.Getenv("PROXY_GROUPS_SOURCE_URL"); env != "" {
		return env
	}

	return DefaultSourceURL
}

// FetchConfig 从远程下载并验证代理组配置
// 数据仅保存在内存中,不写入磁盘
// 返回值:
//   - []byte: 配置数据
//   - string: 解析后的实际 URL
//   - error: 错误信息
func FetchConfig(overrideURL string) ([]byte, string, error) {
	resolvedURL := ResolveSourceURL(overrideURL)

	data, err := downloadConfig(resolvedURL)
	if err != nil {
		return nil, resolvedURL, err
	}

	// 验证配置有效性
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, resolvedURL, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	return data, resolvedURL, nil
}

// downloadConfig 从远程地址下载配置
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
