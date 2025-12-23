package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	currentVersion = "0.3.1"
	githubRepo     = "Jimleerx/miaomiaowu"
	githubAPIURL   = "https://api.github.com/repos/%s/releases/latest"
)

// UpdateInfo contains version update information
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseURL     string `json:"release_url"`
	DownloadURL    string `json:"download_url"`
	ReleaseNotes   string `json:"release_notes"`
}

// GitHubRelease represents the GitHub API response for a release
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// NewUpdateCheckHandler returns a handler that checks for updates
func NewUpdateCheckHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeUpdateError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		info, err := checkLatestVersion()
		if err != nil {
			writeUpdateError(w, http.StatusInternalServerError, fmt.Errorf("检查更新失败: %w", err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	})
}

// NewUpdateApplyHandler returns a handler that applies updates
func NewUpdateApplyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeUpdateError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 1. Get latest version info
		info, err := checkLatestVersion()
		if err != nil {
			writeUpdateError(w, http.StatusInternalServerError, fmt.Errorf("检查更新失败: %w", err))
			return
		}

		if !info.HasUpdate {
			writeUpdateError(w, http.StatusBadRequest, errors.New("已是最新版本"))
			return
		}

		if info.DownloadURL == "" {
			writeUpdateError(w, http.StatusInternalServerError, errors.New("未找到适合当前系统的下载链接"))
			return
		}

		// 2. Download new binary to temp file
		log.Printf("开始下载更新: %s", info.DownloadURL)
		tempFile, err := downloadBinary(info.DownloadURL)
		if err != nil {
			writeUpdateError(w, http.StatusInternalServerError, fmt.Errorf("下载失败: %w", err))
			return
		}
		defer os.Remove(tempFile)

		// 3. Get target path for the binary
		targetPath, err := getUpdateTargetPath()
		if err != nil {
			writeUpdateError(w, http.StatusInternalServerError, fmt.Errorf("获取程序路径失败: %w", err))
			return
		}

		// 4. Backup current version (only for non-Docker)
		if !isDocker() {
			backupPath := targetPath + ".bak"
			if err := copyFile(targetPath, backupPath); err != nil {
				log.Printf("备份当前版本失败 (非致命错误): %v", err)
			}
		}

		// 5. Replace binary
		log.Printf("正在替换二进制文件: %s -> %s", tempFile, targetPath)
		if err := replaceBinary(tempFile, targetPath); err != nil {
			writeUpdateError(w, http.StatusInternalServerError, fmt.Errorf("替换失败: %w", err))
			return
		}

		// 6. Set execute permission
		if err := os.Chmod(targetPath, 0755); err != nil {
			writeUpdateError(w, http.StatusInternalServerError, fmt.Errorf("设置权限失败: %w", err))
			return
		}

		log.Printf("更新成功，准备重启...")

		// 7. Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "更新完成，正在重启...",
		})

		// 8. Restart asynchronously (give client time to receive response)
		go func() {
			time.Sleep(500 * time.Millisecond)
			restartSelf(targetPath)
		}()
	})
}

// checkLatestVersion fetches the latest release info from GitHub
func checkLatestVersion() (*UpdateInfo, error) {
	url := fmt.Sprintf(githubAPIURL, githubRepo)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "miaomiaowu-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回状态码: %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}

	// Select download URL based on current OS/arch
	arch := runtime.GOARCH
	osName := runtime.GOOS
	binaryName := fmt.Sprintf("mmw-%s-%s", osName, arch)

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == binaryName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	hasUpdate := compareVersions(currentVersion, latestVersion)

	return &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      hasUpdate,
		ReleaseURL:     release.HTMLURL,
		DownloadURL:    downloadURL,
		ReleaseNotes:   release.Body,
	}, nil
}

// compareVersions returns true if latest > current
func compareVersions(current, latest string) bool {
	currentParts := parseVersion(current)
	latestParts := parseVersion(latest)

	for i := 0; i < len(latestParts) || i < len(currentParts); i++ {
		var cp, lp int
		if i < len(currentParts) {
			cp = currentParts[i]
		}
		if i < len(latestParts) {
			lp = latestParts[i]
		}

		if lp > cp {
			return true
		}
		if lp < cp {
			return false
		}
	}
	return false
}

// parseVersion splits version string into integer parts
func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	result := make([]int, len(parts))
	for i, p := range parts {
		var num int
		fmt.Sscanf(p, "%d", &num)
		result[i] = num
	}
	return result
}

// downloadBinary downloads the binary to a temp file
func downloadBinary(url string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回状态码: %d", resp.StatusCode)
	}

	tempFile, err := os.CreateTemp("", "mmw-update-*")
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", err
	}

	tempFile.Close()
	return tempFile.Name(), nil
}

// getUpdateTargetPath returns the path where the binary should be placed
func getUpdateTargetPath() (string, error) {
	if isDocker() {
		// In Docker, write to persistent data directory
		targetPath := "/app/data/server"
		// Ensure data directory exists
		if err := os.MkdirAll("/app/data", 0755); err != nil {
			return "", err
		}
		return targetPath, nil
	}

	// Non-Docker: get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", err
	}
	return execPath, nil
}

// isDocker checks if running inside a Docker container
func isDocker() bool {
	// Check for /.dockerenv file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check for DOCKER environment variable
	if os.Getenv("DOCKER") == "1" {
		return true
	}

	// Check cgroup for docker
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil && strings.Contains(string(data), "docker") {
		return true
	}

	return false
}

// replaceBinary replaces the target with the new binary
func replaceBinary(src, dst string) error {
	// Try rename first (fastest if on same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Rename failed, try copy instead
	return copyFile(src, dst)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination file (truncate if exists)
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// restartSelf restarts the current process
func restartSelf(execPath string) {
	log.Printf("正在重启: %s", execPath)

	// Use syscall.Exec to replace current process (PID stays the same)
	// This is important for Docker where PID 1 must stay alive
	err := syscall.Exec(execPath, os.Args, os.Environ())
	if err != nil {
		log.Printf("syscall.Exec 失败: %v, 尝试启动新进程", err)

		// Fallback: start new process and exit
		cmd := exec.Command(execPath, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Start(); err != nil {
			log.Printf("启动新进程失败: %v", err)
			return
		}

		log.Printf("新进程已启动 (PID: %d), 退出当前进程", cmd.Process.Pid)
		os.Exit(0)
	}
}

func writeUpdateError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
}
