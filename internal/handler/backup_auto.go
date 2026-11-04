package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"miaomiaowu/internal/backupstore"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/taskrun"
)

// 自动备份:按间隔打一份完整备份,投递到 WebDAV / S3 兼容 / Google Drive,并按份数清理旧的。

const (
	autoBackupConfigKey  = "auto_backup_config"
	autoBackupLastRunKey = "auto_backup_last_run"
	// autoBackupTaskName 用于 taskrun 记录(失败必记、成功节流)。
	autoBackupTaskName = "auto_backup"
)

// AutoBackupHandler 提供自动备份的配置读写、连通性测试与「立即备份」。
type AutoBackupHandler struct {
	repo       *storage.TrafficRepository
	masterSeed []byte
}

func NewAutoBackupHandler(repo *storage.TrafficRepository, masterSeed []byte) *AutoBackupHandler {
	return &AutoBackupHandler{repo: repo, masterSeed: masterSeed}
}

func (h *AutoBackupHandler) load(ctx context.Context) (backupstore.Config, error) {
	raw, _ := h.repo.GetSystemSetting(ctx, autoBackupConfigKey)
	return backupstore.Unmarshal(raw, h.masterSeed)
}

// ServeHTTP 分发 /api/admin/backup/auto[/test|/run]。
func (h *AutoBackupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/admin/backup/auto" && r.Method == http.MethodGet:
		h.handleGet(w, r)
	case r.URL.Path == "/api/admin/backup/auto" && (r.Method == http.MethodPut || r.Method == http.MethodPost):
		h.handleSave(w, r)
	case r.URL.Path == "/api/admin/backup/auto/test" && r.Method == http.MethodPost:
		h.handleTest(w, r)
	case r.URL.Path == "/api/admin/backup/auto/run" && r.Method == http.MethodPost:
		h.handleRunNow(w, r)
	default:
		writeError(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *AutoBackupHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("读取自动备份配置失败: %w", err))
		return
	}
	lastRun, _ := h.repo.GetSystemSetting(r.Context(), autoBackupLastRunKey)
	respondJSON(w, http.StatusOK, map[string]any{
		"success": true,
		// 凭据一律打码 —— 读接口绝不回显密钥。
		"config":   cfg.Redacted(),
		"last_run": lastRun,
	})
}

func (h *AutoBackupHandler) handleSave(w http.ResponseWriter, r *http.Request) {
	var incoming backupstore.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeBadRequest(w, "请求格式错误")
		return
	}
	current, err := h.load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 前端把没改动的凭据原样回传占位符,这里换回真实值。
	merged := backupstore.MergeSecrets(incoming, current)
	merged.Normalize()
	// 启用时必须能构造出目标,否则等于存了个永远失败的定时任务。
	if merged.Enabled {
		if _, derr := merged.Destination(); derr != nil {
			writeBadRequest(w, derr.Error())
			return
		}
	}
	raw, err := merged.Marshal(h.masterSeed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := h.repo.SetSystemSetting(r.Context(), autoBackupConfigKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("保存失败: %w", err))
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "config": merged.Redacted()})
}

// handleTest 只验连通性与权限:上传一个小探针文件再删掉,不打整份备份。
func (h *AutoBackupHandler) handleTest(w http.ResponseWriter, r *http.Request) {
	var incoming backupstore.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeBadRequest(w, "请求格式错误")
		return
	}
	current, _ := h.load(r.Context())
	cfg := backupstore.MergeSecrets(incoming, current)
	dest, err := cfg.Destination()
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := backupstore.Probe(ctx, dest); err != nil {
		respondJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "message": "连接正常,读写权限均可用"})
}

func (h *AutoBackupHandler) handleRunNow(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 手工触发不看 Enabled —— 想先跑一次看看结果再开定时是很自然的操作。
	if _, derr := cfg.Destination(); derr != nil {
		writeBadRequest(w, derr.Error())
		return
	}
	// 后台跑:备份可能很大、上传要几十分钟,不能挂在 HTTP 请求上。
	go func() {
		if err := h.RunOnce(context.Background(), cfg); err != nil {
			logger.Warn("[自动备份] 手动触发失败", "error", err)
		}
	}()
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已在后台开始备份,完成后可在任务记录里查看"})
}

// RunOnce 打一份备份、投递、清理旧的。
func (h *AutoBackupHandler) RunOnce(ctx context.Context, cfg backupstore.Config) error {
	dest, err := cfg.Destination()
	if err != nil {
		return err
	}
	archive, err := BuildBackupArchive(ctx, h.repo)
	if err != nil {
		return fmt.Errorf("打包备份失败: %w", err)
	}
	defer archive.Close()

	if err := dest.Put(ctx, archive.Filename, archive.File, archive.Size); err != nil {
		return err
	}
	logger.Info("[自动备份] 已上传", "file", archive.Filename,
		"size_mib", fmt.Sprintf("%.1f", float64(archive.Size)/(1<<20)), "kind", dest.Kind())

	// 保留策略失败不算整次备份失败 —— 备份本身已经安全落地了,清理只是省空间。
	if deleted, rerr := backupstore.ApplyRetention(ctx, dest, cfg.Keep); rerr != nil {
		logger.Warn("[自动备份] 清理旧备份出错(备份本身已成功)", "error", rerr)
	} else if len(deleted) > 0 {
		logger.Info("[自动备份] 已清理旧备份", "count", len(deleted))
	}
	_ = h.repo.SetSystemSetting(ctx, autoBackupLastRunKey, time.Now().Format(time.RFC3339))
	return nil
}

// StartScheduler 起自动备份的定时循环。返回后台运行,随 ctx 结束。
func (h *AutoBackupHandler) StartScheduler(ctx context.Context) {
	go func() {
		// 启动后先等一会儿:服务刚起来时 DB 还在初始化,这时打包既慢又容易撞上。
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
		for {
			h.tick(ctx)
			select {
			case <-ctx.Done():
				return
			// 每 10 分钟醒一次看看「离上次备份是否已超过间隔」,而不是按间隔 sleep:
			// 后者在改配置或重启后要等满一个周期才生效,间隔设 7 天就等于一周不备份。
			case <-time.After(10 * time.Minute):
			}
		}
	}()
}

func (h *AutoBackupHandler) tick(ctx context.Context) {
	cfg, err := h.load(ctx)
	if err != nil || !cfg.Enabled {
		return
	}
	if _, derr := cfg.Destination(); derr != nil {
		return
	}
	lastRaw, _ := h.repo.GetSystemSetting(ctx, autoBackupLastRunKey)
	if cfg.PreferredHour != nil {
		// 「每天固定整点」模式:只在该小时内触发,且今天该整点之后没跑过才跑。
		// tick 每 10 分钟醒一次,会在该小时窗口内触发一次,之后同小时的 tick 因 last>=今天触发点而跳过。
		now := time.Now()
		if now.Hour() != *cfg.PreferredHour {
			return
		}
		todayTrigger := time.Date(now.Year(), now.Month(), now.Day(), *cfg.PreferredHour, 0, 0, 0, now.Location())
		if last, perr := time.Parse(time.RFC3339, lastRaw); perr == nil && !last.Before(todayTrigger) {
			return // 今天该整点已备份过
		}
	} else if last, perr := time.Parse(time.RFC3339, lastRaw); perr == nil {
		if time.Since(last) < time.Duration(cfg.IntervalHours)*time.Hour {
			return
		}
	}
	taskrun.Record(ctx, autoBackupTaskName, func() (string, error) {
		if err := h.RunOnce(ctx, cfg); err != nil {
			logger.Warn("[自动备份] 失败", "error", err)
			return "", err
		}
		return fmt.Sprintf("已备份到 %s", cfg.Kind), nil
	})
}
