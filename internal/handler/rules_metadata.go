package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/storage"
)

type ruleMetadataHandler struct {
	repo    *storage.TrafficRepository
	baseDir string
}

// NewRuleMetadataHandler exposes latest rule version information for select files.
func NewRuleMetadataHandler(baseDir string, repo *storage.TrafficRepository) http.Handler {
	if baseDir == "" {
		panic("rule metadata handler requires base directory")
	}
	return &ruleMetadataHandler{repo: repo, baseDir: baseDir}
}

func (h *ruleMetadataHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "仅支持 GET 请求", http.StatusMethodNotAllowed)
		return
	}

	files := r.URL.Query()["file"]
	if len(files) == 0 {
		files = []string{"subscribe.yaml", "subscribe-openclash-redirhost.yaml", "subscribe-openclash-fakeip.yaml"}
	}

	type item struct {
		Name          string `json:"name"`
		LatestVersion int64  `json:"latest_version"`
		UpdatedAt     string `json:"updated_at"`
		ModTime       int64  `json:"mod_time"`
	}

	items := make([]item, 0, len(files))

	for _, name := range files {
		cleanName, ok := sanitizeRuleFilename(name)
		if !ok {
			continue
		}

		absolute := filepath.Join(h.baseDir, cleanName)
		info, err := os.Stat(absolute)
		if err != nil {
			continue
		}

		entry := item{
			Name:    cleanName,
			ModTime: info.ModTime().Unix(),
		}

		if h.repo != nil {
			if latest, err := h.repo.LatestRuleVersion(r.Context(), cleanName); err == nil {
				entry.LatestVersion = latest.Version
				entry.UpdatedAt = latest.CreatedAt.UTC().Format(time.RFC3339)
			}
		}

		items = append(items, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"rules": items})
}

func sanitizeRuleFilename(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}

	clean := filepath.Clean(name)
	if clean != name {
		return "", false
	}

	if filepath.Base(clean) != clean {
		return "", false
	}

	if strings.Contains(clean, "..") {
		return "", false
	}

	lower := strings.ToLower(clean)
	if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
		return "", false
	}

	return clean, true
}
