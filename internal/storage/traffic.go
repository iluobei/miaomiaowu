package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const (
	pragmaJournalMode = "PRAGMA journal_mode=WAL;"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

const (
	SubscriptionButtonQR     = "qr"
	SubscriptionButtonCopy   = "copy"
	SubscriptionButtonImport = "import"
)

// TrafficRecord represents an aggregated traffic snapshot for a specific date.
type TrafficRecord struct {
	Date           time.Time
	TotalLimit     int64
	TotalUsed      int64
	TotalRemaining int64
}

// TrafficRepository manages persistence of traffic usage snapshots.
type TrafficRepository struct {
	db *sql.DB
}

// SubscriptionLink represents a configurable subscription entry exposed to clients.
type SubscriptionLink struct {
	ID           int64
	Name         string
	Type         string
	Description  string
	RuleFilename string
	Buttons      []string
	ShortURL     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func normalizeSubscriptionButtons(input []string) []string {
	if len(input) == 0 {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	seen := make(map[string]struct{}, len(input))
	for _, button := range input {
		key := strings.ToLower(strings.TrimSpace(button))
		if _, ok := allowedSubscriptionButtons[key]; ok {
			seen[key] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	order := []string{SubscriptionButtonQR, SubscriptionButtonCopy, SubscriptionButtonImport}
	normalized := make([]string, 0, len(seen))
	for _, button := range order {
		if _, ok := seen[button]; ok {
			normalized = append(normalized, button)
		}
	}

	return normalized
}

func encodeSubscriptionButtons(input []string) (string, error) {
	normalized := normalizeSubscriptionButtons(input)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSubscriptionButtons(encoded string) []string {
	if strings.TrimSpace(encoded) == "" {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	var raw []string
	if err := json.Unmarshal([]byte(encoded), &raw); err != nil {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	return normalizeSubscriptionButtons(raw)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubscriptionLink(scanner rowScanner) (SubscriptionLink, error) {
	var (
		link    SubscriptionLink
		buttons string
	)

	if err := scanner.Scan(&link.ID, &link.Name, &link.Type, &link.Description, &link.RuleFilename, &buttons, &link.ShortURL, &link.CreatedAt, &link.UpdatedAt); err != nil {
		return SubscriptionLink{}, err
	}

	link.Buttons = decodeSubscriptionButtons(buttons)

	return link, nil
}

func scanProbeConfig(scanner rowScanner) (ProbeConfig, error) {
	var cfg ProbeConfig
	if err := scanner.Scan(&cfg.ID, &cfg.ProbeType, &cfg.Address, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
		return ProbeConfig{}, err
	}
	return cfg, nil
}

func scanProbeServer(scanner rowScanner) (ProbeServer, error) {
	var srv ProbeServer
	if err := scanner.Scan(&srv.ID, &srv.ConfigID, &srv.ServerID, &srv.Name, &srv.TrafficMethod, &srv.MonthlyTrafficBytes, &srv.Position, &srv.CreatedAt, &srv.UpdatedAt); err != nil {
		return ProbeServer{}, err
	}
	return srv, nil
}

var (
	ErrTokenNotFound                = errors.New("token not found")
	ErrUserNotFound                 = errors.New("user not found")
	ErrUserExists                   = errors.New("user already exists")
	ErrRuleVersionNotFound          = errors.New("rule version not found")
	ErrSubscriptionNotFound         = errors.New("subscription link not found")
	ErrSubscriptionExists           = errors.New("subscription link already exists")
	ErrProbeConfigNotFound          = errors.New("probe configuration not found")
	ErrNodeNotFound                 = errors.New("node not found")
	ErrSubscribeFileNotFound        = errors.New("subscribe file not found")
	ErrSubscribeFileExists          = errors.New("subscribe file already exists")
	ErrUserSettingsNotFound         = errors.New("user settings not found")
	ErrExternalSubscriptionNotFound = errors.New("external subscription not found")
	ErrExternalSubscriptionExists   = errors.New("external subscription already exists")
)

var (
	allowedSubscriptionButtons = map[string]struct{}{
		SubscriptionButtonQR:     {},
		SubscriptionButtonCopy:   {},
		SubscriptionButtonImport: {},
	}
	defaultSubscriptionButtons = []string{
		SubscriptionButtonQR,
		SubscriptionButtonCopy,
		SubscriptionButtonImport,
	}
)

const (
	ProbeTypeNezha   = "nezha"
	ProbeTypeNezhaV0 = "nezhav0"
	ProbeTypeDstatus = "dstatus"
	ProbeTypeKomari  = "komari"

	TrafficMethodUp   = "up"
	TrafficMethodDown = "down"
	TrafficMethodBoth = "both"
)

type ProbeConfig struct {
	ID        int64
	ProbeType string
	Address   string
	Servers   []ProbeServer
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ProbeServer struct {
	ID                  int64
	ConfigID            int64
	ServerID            string
	Name                string
	TrafficMethod       string
	MonthlyTrafficBytes int64
	Position            int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Node represents a proxy node stored in the database.
type Node struct {
	ID             int64
	Username       string
	RawURL         string
	NodeName       string
	Protocol       string
	ParsedConfig   string
	ClashConfig    string
	Enabled        bool
	Tag            string
	OriginalServer string
	ProbeServer    string // Probe server name for binding
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SubscribeFile represents a subscription file configuration.
type SubscribeFile struct {
	ID                   int64
	Name                 string
	Description          string
	URL                  string
	Type                 string
	Filename             string
	FileShortCode        string // 3-character code for file identification in composite short links
	AutoSyncCustomRules  bool   // Whether to automatically sync custom rules to this file
	ExpireAt             *time.Time // Optional expiration timestamp
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// UserSettings represents user-specific configuration.
type UserSettings struct {
	Username             string
	ForceSyncExternal    bool
	MatchRule            string // "node_name", "server_port", or "type_server_port"
	SyncScope            string // "saved_only" or "all"
	KeepNodeName         bool   // Keep current node name when syncing
	CacheExpireMinutes   int    // Cache expiration time in minutes
	SyncTraffic          bool   // Sync traffic info from external subscriptions
	EnableProbeBinding   bool   // Enable probe server binding for nodes
	CustomRulesEnabled   bool   // Enable custom rules feature
	EnableShortLink      bool   // Enable short link feature for subscriptions
	UseNewTemplateSystem bool   // Use new template system (database-based), default true
	EnableProxyProvider  bool      // Enable proxy provider feature
	NodeOrder            []int64   // Node display order (array of node IDs)
	DebugEnabled         bool      // Enable debug logging to file
	DebugLogPath         string    // Path to current debug log file
	DebugStartedAt       *time.Time // When debug logging was started
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// SystemConfig represents global system configuration shared across all users.
type SystemConfig struct {
	ProxyGroupsSourceURL     string // Remote URL for proxy groups configuration
	ClientCompatibilityMode  bool   // Auto-filter incompatible nodes for clients
}

// ExternalSubscription represents an external subscription URL imported by user.
type ExternalSubscription struct {
	ID          int64
	Username    string
	Name        string
	URL         string
	UserAgent   string // User-Agent 请求头
	NodeCount   int
	LastSyncAt  *time.Time
	Upload      int64      // 已上传流量（字节）
	Download    int64      // 已下载流量（字节）
	Total       int64      // 总流量（字节）
	Expire      *time.Time // 过期时间
	TrafficMode string     // 流量统计方式: "download", "upload", "both"
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CustomRule represents a custom rule for DNS, rules, or rule-providers.
type CustomRule struct {
	ID        int64
	Name      string
	Type      string // "dns", "rules", "rule-providers"
	Mode      string // "replace", "prepend"
	Content   string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CustomRuleApplication tracks what content was applied by custom rules to subscribe files
type CustomRuleApplication struct {
	ID              int64
	SubscribeFileID int64
	CustomRuleID    int64
	RuleType        string // "dns", "rules", "rule-providers"
	RuleMode        string // "replace", "prepend"
	AppliedContent  string // JSON-serialized content that was applied
	ContentHash     string // SHA256 hash of the content for quick comparison
	AppliedAt       time.Time
}

// ProxyProviderConfig represents a proxy-provider configuration for external subscription
type ProxyProviderConfig struct {
	ID                       int64
	Username                 string
	ExternalSubscriptionID   int64
	Name                     string // 代理集合名称
	Type                     string // http/file
	Interval                 int    // 更新间隔(秒)
	Proxy                    string // 下载代理
	SizeLimit                int    // 文件大小限制
	Header                   string // JSON: {"User-Agent": [...], "Authorization": [...]}
	HealthCheckEnabled       bool
	HealthCheckURL           string
	HealthCheckInterval      int
	HealthCheckTimeout       int
	HealthCheckLazy          bool
	HealthCheckExpectedStatus int
	Filter                   string // 正则: 保留匹配的节点
	ExcludeFilter            string // 正则: 排除匹配的节点
	ExcludeType              string // 排除的协议类型，逗号分隔
	GeoIPFilter              string // 地理位置过滤，国家代码如 "HK" 或 "HK,TW"（仅 MMW 模式生效）
	Override                 string // JSON: 覆写配置
	ProcessMode              string // 'client'=客户端处理, 'mmw'=妙妙屋处理
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

var (
	allowedProbeTypes = map[string]struct{}{
		ProbeTypeNezha:   {},
		ProbeTypeNezhaV0: {},
		ProbeTypeDstatus: {},
		ProbeTypeKomari:  {},
	}
	allowedTrafficMethods = map[string]struct{}{
		TrafficMethodUp:   {},
		TrafficMethodDown: {},
		TrafficMethodBoth: {},
	}
)

// NewTrafficRepository initializes a new SQLite-backed repository stored at the given path or DSN.
func NewTrafficRepository(path string) (*TrafficRepository, error) {
	if path == "" {
		return nil, errors.New("traffic repository path is empty")
	}

	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create traffic data directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(pragmaJournalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable wal: %w", err)
	}

	repo := &TrafficRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

// Close releases the underlying database resources.
func (r *TrafficRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// Checkpoint forces a WAL checkpoint to ensure all data is written to the main database file.
// This is useful before creating backups.
func (r *TrafficRepository) Checkpoint() error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

func (r *TrafficRepository) migrate() error {
	const trafficSchema = `
CREATE TABLE IF NOT EXISTS traffic_records (
    date TEXT PRIMARY KEY,
    total_limit INTEGER NOT NULL,
    total_used INTEGER NOT NULL,
    total_remaining INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

	if _, err := r.db.Exec(trafficSchema); err != nil {
		return fmt.Errorf("migrate traffic_records: %w", err)
	}

	const userTokenSchema = `
CREATE TABLE IF NOT EXISTS user_tokens (
    username TEXT PRIMARY KEY,
    token TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

	if _, err := r.db.Exec(userTokenSchema); err != nil {
		return fmt.Errorf("migrate user_tokens: %w", err)
	}

	// Add user_short_code column to user_tokens table if it doesn't exist (3-character code)
	if err := r.ensureUserTokenColumn("user_short_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Create unique index for user_short_code (only for non-empty values)
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tokens_user_short_code ON user_tokens(user_short_code) WHERE user_short_code != '';`); err != nil {
		return fmt.Errorf("create user_short_code index: %w", err)
	}

	// Generate user short codes for existing users that don't have one
	if err := r.generateMissingUserShortCodes(); err != nil {
		return fmt.Errorf("generate missing user short codes: %w", err)
	}

	const sessionSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sessions_username ON sessions(username);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`

	if _, err := r.db.Exec(sessionSchema); err != nil {
		return fmt.Errorf("migrate sessions: %w", err)
	}

	const userSchema = `
CREATE TABLE IF NOT EXISTS users (
    username TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    email TEXT,
    nickname TEXT,
    avatar_url TEXT,
    role TEXT NOT NULL DEFAULT 'user',
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

	if _, err := r.db.Exec(userSchema); err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}

	if err := r.ensureUserColumn("email", "TEXT"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("nickname", "TEXT"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("avatar_url", "TEXT"); err != nil {
		return err
	}

	if err := r.syncNicknames(); err != nil {
		return err
	}

	if err := r.ensureUserColumn("role", "TEXT NOT NULL DEFAULT 'user'"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("is_active", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("remark", "TEXT"); err != nil {
		return err
	}

	const historySchema = `
CREATE TABLE IF NOT EXISTS rule_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filename TEXT NOT NULL,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(filename, version)
);
`

	if _, err := r.db.Exec(historySchema); err != nil {
		return fmt.Errorf("migrate rule_versions: %w", err)
	}

	const subscriptionSchema = `
CREATE TABLE IF NOT EXISTS subscription_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT '',
    description TEXT,
    rule_filename TEXT NOT NULL,
    buttons TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);
`

	if _, err := r.db.Exec(subscriptionSchema); err != nil {
		return fmt.Errorf("migrate subscription_links: %w", err)
	}

	// Add short_url column to subscription_links table if it doesn't exist
	if err := r.ensureSubscriptionLinkColumn("short_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Create unique index for short_url (only for non-empty values)
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_links_short_url ON subscription_links(short_url) WHERE short_url != '';`); err != nil {
		return fmt.Errorf("create short_url index: %w", err)
	}

	// Migrate existing probe_configs table to add nezhav0 support BEFORE creating with IF NOT EXISTS
	if err := r.migrateProbeConfigsForNezhaV0(); err != nil {
		return fmt.Errorf("migrate probe_configs for nezhav0: %w", err)
	}

	const probeConfigSchema = `
CREATE TABLE IF NOT EXISTS probe_configs (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    probe_type TEXT NOT NULL CHECK (probe_type IN ('nezha','nezhav0','dstatus','komari')),
    address TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

	if _, err := r.db.Exec(probeConfigSchema); err != nil {
		return fmt.Errorf("migrate probe_configs: %w", err)
	}

	const probeServersSchema = `
CREATE TABLE IF NOT EXISTS probe_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_id INTEGER NOT NULL,
    server_id TEXT NOT NULL,
    name TEXT NOT NULL,
    traffic_method TEXT NOT NULL CHECK (traffic_method IN ('up','down','both')),
    monthly_traffic_bytes INTEGER NOT NULL DEFAULT 0 CHECK (monthly_traffic_bytes >= 0),
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(config_id) REFERENCES probe_configs(id) ON DELETE CASCADE,
    UNIQUE(config_id, server_id)
);
`

	if _, err := r.db.Exec(probeServersSchema); err != nil {
		return fmt.Errorf("migrate probe_servers: %w", err)
	}

	if err := r.ensureDefaultProbeConfig(); err != nil {
		return err
	}

	const nodesSchema = `
CREATE TABLE IF NOT EXISTS nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    raw_url TEXT NOT NULL,
    node_name TEXT NOT NULL,
    protocol TEXT NOT NULL,
    parsed_config TEXT NOT NULL,
    clash_config TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    tag TEXT NOT NULL DEFAULT '手动输入',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_nodes_username ON nodes(username);
CREATE INDEX IF NOT EXISTS idx_nodes_protocol ON nodes(protocol);
CREATE INDEX IF NOT EXISTS idx_nodes_enabled ON nodes(enabled);
`

	if _, err := r.db.Exec(nodesSchema); err != nil {
		return fmt.Errorf("migrate nodes: %w", err)
	}

	// Add tag column to existing nodes table if it doesn't exist
	if err := r.ensureNodeColumn("tag", "TEXT NOT NULL DEFAULT '手动输入'"); err != nil {
		return err
	}

	// Add original_server column to existing nodes table if it doesn't exist
	if err := r.ensureNodeColumn("original_server", "TEXT"); err != nil {
		return err
	}

	// Add probe_server column to existing nodes table if it doesn't exist
	if err := r.ensureNodeColumn("probe_server", "TEXT"); err != nil {
		return err
	}

	// Create tag index after ensuring column exists
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_tag ON nodes(tag);`); err != nil {
		return fmt.Errorf("create tag index: %w", err)
	}

	const subscribeFilesSchema = `
CREATE TABLE IF NOT EXISTS subscribe_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    url TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('create','import','upload')),
    filename TEXT NOT NULL,
    expire_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);
CREATE INDEX IF NOT EXISTS idx_subscribe_files_type ON subscribe_files(type);
`

	if _, err := r.db.Exec(subscribeFilesSchema); err != nil {
		return fmt.Errorf("migrate subscribe_files: %w", err)
	}

	// 用户-订阅关联表（多对多关系）
	// 关联到 subscribe_files 表
	const userSubscriptionsSchema = `
CREATE TABLE IF NOT EXISTS user_subscriptions (
    username TEXT NOT NULL,
    subscription_id INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (username, subscription_id),
    FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE,
    FOREIGN KEY(subscription_id) REFERENCES subscribe_files(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_username ON user_subscriptions(username);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_subscription_id ON user_subscriptions(subscription_id);
`

	if _, err := r.db.Exec(userSubscriptionsSchema); err != nil {
		return fmt.Errorf("migrate user_subscriptions: %w", err)
	}

	const userSettingsSchema = `
CREATE TABLE IF NOT EXISTS user_settings (
    username TEXT PRIMARY KEY,
    force_sync_external INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE
);
`

	if _, err := r.db.Exec(userSettingsSchema); err != nil {
		return fmt.Errorf("migrate user_settings: %w", err)
	}

	// Add match_rule column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("match_rule", "TEXT NOT NULL DEFAULT 'node_name'"); err != nil {
		return err
	}

	// Add cache_expire_minutes column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("cache_expire_minutes", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add sync_traffic column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("sync_traffic", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add enable_probe_binding column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("enable_probe_binding", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add sync_scope column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("sync_scope", "TEXT NOT NULL DEFAULT 'saved_only'"); err != nil {
		return err
	}

	// Add keep_node_name column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("keep_node_name", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}

	const externalSubscriptionsSchema = `
CREATE TABLE IF NOT EXISTS external_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    node_count INTEGER NOT NULL DEFAULT 0,
    last_sync_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE,
    UNIQUE(username, url)
);
CREATE INDEX IF NOT EXISTS idx_external_subscriptions_username ON external_subscriptions(username);
CREATE INDEX IF NOT EXISTS idx_external_subscriptions_url ON external_subscriptions(url);
`

	if _, err := r.db.Exec(externalSubscriptionsSchema); err != nil {
		return fmt.Errorf("migrate external_subscriptions: %w", err)
	}

	// Add traffic fields to external_subscriptions table
	if err := r.ensureExternalSubscriptionColumn("upload", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("download", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("total", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("expire", "TIMESTAMP"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("user_agent", "TEXT NOT NULL DEFAULT 'clash-meta/2.4.0'"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("traffic_mode", "TEXT NOT NULL DEFAULT 'both'"); err != nil {
		return err
	}

	// Add custom_rules_enabled to user_settings table
	if err := r.ensureUserSettingsColumn("custom_rules_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add enable_short_link to user_settings table
	if err := r.ensureUserSettingsColumn("enable_short_link", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add use_new_template_system to user_settings table (default true)
	if err := r.ensureUserSettingsColumn("use_new_template_system", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}

	// Add enable_proxy_provider to user_settings table
	if err := r.ensureUserSettingsColumn("enable_proxy_provider", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add node_order to user_settings table (JSON array of node IDs for display order)
	if err := r.ensureUserSettingsColumn("node_order", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}

	// Add debug logging fields to user_settings table
	if err := r.ensureUserSettingsColumn("debug_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureUserSettingsColumn("debug_log_path", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureUserSettingsColumn("debug_started_at", "TIMESTAMP"); err != nil {
		return err
	}

	// Add file_short_code column to subscribe_files table (3-character code)
	if err := r.ensureSubscribeFileColumn("file_short_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Add expire_at column to subscribe_files table
	if err := r.ensureSubscribeFileColumn("expire_at", "TIMESTAMP"); err != nil {
		return err
	}

	// Create unique index for file_short_code in subscribe_files (only for non-empty values)
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscribe_files_file_short_code ON subscribe_files(file_short_code) WHERE file_short_code != '';`); err != nil {
		return fmt.Errorf("create subscribe_files file_short_code index: %w", err)
	}

	// Generate file short codes for existing subscribe_files that don't have one
	if err := r.generateMissingFileShortCodes(); err != nil {
		return fmt.Errorf("generate missing file short codes: %w", err)
	}

	// Create system_config table for global settings
	const systemConfigSchema = `
CREATE TABLE IF NOT EXISTS system_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    proxy_groups_source_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := r.db.Exec(systemConfigSchema); err != nil {
		return fmt.Errorf("migrate system_config: %w", err)
	}

	// Ensure system_config has exactly one row (singleton pattern)
	const ensureSystemConfigRow = `
INSERT INTO system_config (id, proxy_groups_source_url)
SELECT 1, ''
WHERE NOT EXISTS (SELECT 1 FROM system_config WHERE id = 1);
`
	if _, err := r.db.Exec(ensureSystemConfigRow); err != nil {
		return fmt.Errorf("seed system_config: %w", err)
	}

	// Add client_compatibility_mode column to system_config table
	if err := r.ensureSystemConfigColumn("client_compatibility_mode", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	const customRulesSchema = `
CREATE TABLE IF NOT EXISTS custom_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('dns','rules','rule-providers')),
    mode TEXT NOT NULL CHECK (mode IN ('replace','prepend','append')),
    content TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, type)
);
CREATE INDEX IF NOT EXISTS idx_custom_rules_type ON custom_rules(type);
CREATE INDEX IF NOT EXISTS idx_custom_rules_enabled ON custom_rules(enabled);
`

	if _, err := r.db.Exec(customRulesSchema); err != nil {
		return fmt.Errorf("migrate custom_rules: %w", err)
	}

	// Migrate existing custom_rules table to support 'append' mode
	if err := r.migrateCustomRulesAppendMode(); err != nil {
		return fmt.Errorf("migrate custom_rules append mode: %w", err)
	}

	// Add auto_sync_custom_rules column to subscribe_files table
	if err := r.ensureSubscribeFileColumn("auto_sync_custom_rules", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Create custom_rule_applications table for tracking applied content
	const customRuleApplicationsSchema = `
CREATE TABLE IF NOT EXISTS custom_rule_applications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscribe_file_id INTEGER NOT NULL,
    custom_rule_id INTEGER NOT NULL,
    rule_type TEXT NOT NULL,
    rule_mode TEXT NOT NULL,
    applied_content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (subscribe_file_id) REFERENCES subscribe_files(id) ON DELETE CASCADE,
    FOREIGN KEY (custom_rule_id) REFERENCES custom_rules(id) ON DELETE CASCADE,
    UNIQUE(subscribe_file_id, custom_rule_id, rule_type)
);
CREATE INDEX IF NOT EXISTS idx_custom_rule_applications_file ON custom_rule_applications(subscribe_file_id);
CREATE INDEX IF NOT EXISTS idx_custom_rule_applications_rule ON custom_rule_applications(custom_rule_id);
`

	if _, err := r.db.Exec(customRuleApplicationsSchema); err != nil {
		return fmt.Errorf("migrate custom_rule_applications: %w", err)
	}

	// Templates table for ACL4SSR rule configuration
	const templatesSchema = `
CREATE TABLE IF NOT EXISTS templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'clash' CHECK (category IN ('clash','surge')),
    template_url TEXT NOT NULL DEFAULT '',
    rule_source TEXT NOT NULL DEFAULT '',
    use_proxy INTEGER NOT NULL DEFAULT 0,
    enable_include_all INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);
CREATE INDEX IF NOT EXISTS idx_templates_category ON templates(category);
`

	if _, err := r.db.Exec(templatesSchema); err != nil {
		return fmt.Errorf("migrate templates: %w", err)
	}

	// Proxy provider configs table
	const proxyProviderConfigsSchema = `
CREATE TABLE IF NOT EXISTS proxy_provider_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    external_subscription_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'http',
    interval INTEGER DEFAULT 3600,
    proxy TEXT DEFAULT 'DIRECT',
    size_limit INTEGER DEFAULT 0,
    header TEXT,
    health_check_enabled INTEGER DEFAULT 1,
    health_check_url TEXT DEFAULT 'https://www.gstatic.com/generate_204',
    health_check_interval INTEGER DEFAULT 300,
    health_check_timeout INTEGER DEFAULT 5000,
    health_check_lazy INTEGER DEFAULT 1,
    health_check_expected_status INTEGER DEFAULT 204,
    filter TEXT,
    exclude_filter TEXT,
    exclude_type TEXT,
    geo_ip_filter TEXT,
    override TEXT,
    process_mode TEXT DEFAULT 'client',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (external_subscription_id) REFERENCES external_subscriptions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_proxy_provider_configs_username ON proxy_provider_configs(username);
CREATE INDEX IF NOT EXISTS idx_proxy_provider_configs_external_subscription_id ON proxy_provider_configs(external_subscription_id);
`
	if _, err := r.db.Exec(proxyProviderConfigsSchema); err != nil {
		return fmt.Errorf("migrate proxy_provider_configs: %w", err)
	}

	// 添加 geo_ip_filter 列（为旧数据库迁移）
	if err := r.ensureProxyProviderConfigColumn("geo_ip_filter", "TEXT"); err != nil {
		return fmt.Errorf("ensure geo_ip_filter column: %w", err)
	}

	return nil
}

// ListSubscriptionLinks returns all configured subscription links ordered by creation.
func (r *TrafficRepository) ListSubscriptionLinks(ctx context.Context) ([]SubscriptionLink, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list subscription links: %w", err)
	}
	defer rows.Close()

	var links []SubscriptionLink
	for rows.Next() {
		link, err := scanSubscriptionLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan subscription link: %w", err)
		}
		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscription links: %w", err)
	}

	return links, nil
}

// GetSubscriptionByName retrieves a subscription link by its unique name.
func (r *TrafficRepository) GetSubscriptionByName(ctx context.Context, name string) (SubscriptionLink, error) {
	var link SubscriptionLink
	if r == nil || r.db == nil {
		return link, errors.New("traffic repository not initialized")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return link, errors.New("subscription name is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links WHERE name = ? LIMIT 1`, name)
	result, err := scanSubscriptionLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return link, ErrSubscriptionNotFound
		}
		return link, fmt.Errorf("get subscription by name: %w", err)
	}

	return result, nil
}

// GetSubscriptionByID retrieves a subscription link by its identifier.
func (r *TrafficRepository) GetSubscriptionByID(ctx context.Context, id int64) (SubscriptionLink, error) {
	var link SubscriptionLink
	if r == nil || r.db == nil {
		return link, errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return link, errors.New("subscription id is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links WHERE id = ? LIMIT 1`, id)
	result, err := scanSubscriptionLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return link, ErrSubscriptionNotFound
		}
		return link, fmt.Errorf("get subscription by id: %w", err)
	}

	return result, nil
}

// GetFirstSubscriptionLink returns the earliest created subscription link.
func (r *TrafficRepository) GetFirstSubscriptionLink(ctx context.Context) (SubscriptionLink, error) {
	var link SubscriptionLink
	if r == nil || r.db == nil {
		return link, errors.New("traffic repository not initialized")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links ORDER BY id ASC LIMIT 1`)
	result, err := scanSubscriptionLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return link, ErrSubscriptionNotFound
		}
		return link, fmt.Errorf("get first subscription: %w", err)
	}

	return result, nil
}

// CreateSubscriptionLink inserts a new subscription link definition.
func (r *TrafficRepository) CreateSubscriptionLink(ctx context.Context, link SubscriptionLink) (SubscriptionLink, error) {
	if r == nil || r.db == nil {
		return SubscriptionLink{}, errors.New("traffic repository not initialized")
	}

	link.Name = strings.TrimSpace(link.Name)
	link.Type = strings.TrimSpace(link.Type)
	link.Description = strings.TrimSpace(link.Description)
	link.RuleFilename = strings.TrimSpace(link.RuleFilename)

	if link.Name == "" {
		return SubscriptionLink{}, errors.New("subscription name is required")
	}
	if link.Type == "" {
		link.Type = link.Name
	}
	if link.RuleFilename == "" {
		return SubscriptionLink{}, errors.New("rule filename is required")
	}

	encodedButtons, err := encodeSubscriptionButtons(link.Buttons)
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("encode subscription buttons: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `INSERT INTO subscription_links (name, type, description, rule_filename, buttons) VALUES (?, ?, ?, ?, ?)`, link.Name, link.Type, link.Description, link.RuleFilename, encodedButtons)
	if err != nil {
		lowered := strings.ToLower(err.Error())
		if strings.Contains(lowered, "unique") {
			return SubscriptionLink{}, ErrSubscriptionExists
		}
		return SubscriptionLink{}, fmt.Errorf("create subscription link: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("fetch subscription id: %w", err)
	}

	// Note: subscription_links uses old short URL system, not the new composite short link system
	// The new composite system (file_short_code + user_short_code) is used by subscribe_files table

	return r.GetSubscriptionByID(ctx, id)
}

// UpdateSubscriptionLink updates an existing subscription link.
func (r *TrafficRepository) UpdateSubscriptionLink(ctx context.Context, link SubscriptionLink) (SubscriptionLink, error) {
	if r == nil || r.db == nil {
		return SubscriptionLink{}, errors.New("traffic repository not initialized")
	}

	if link.ID <= 0 {
		return SubscriptionLink{}, errors.New("subscription id is required")
	}

	link.Name = strings.TrimSpace(link.Name)
	link.Type = strings.TrimSpace(link.Type)
	link.Description = strings.TrimSpace(link.Description)
	link.RuleFilename = strings.TrimSpace(link.RuleFilename)

	if link.Name == "" {
		return SubscriptionLink{}, errors.New("subscription name is required")
	}
	if link.Type == "" {
		link.Type = link.Name
	}
	if link.RuleFilename == "" {
		return SubscriptionLink{}, errors.New("rule filename is required")
	}

	encodedButtons, err := encodeSubscriptionButtons(link.Buttons)
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("encode subscription buttons: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `UPDATE subscription_links SET name = ?, type = ?, description = ?, rule_filename = ?, buttons = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, link.Name, link.Type, link.Description, link.RuleFilename, encodedButtons, link.ID)
	if err != nil {
		lowered := strings.ToLower(err.Error())
		if strings.Contains(lowered, "unique") {
			return SubscriptionLink{}, ErrSubscriptionExists
		}
		return SubscriptionLink{}, fmt.Errorf("update subscription link: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("subscription update rows affected: %w", err)
	}
	if affected == 0 {
		return SubscriptionLink{}, ErrSubscriptionNotFound
	}

	return r.GetSubscriptionByID(ctx, link.ID)
}

// DeleteSubscriptionLink removes a subscription link definition.
func (r *TrafficRepository) DeleteSubscriptionLink(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	if id <= 0 {
		return errors.New("subscription id is required")
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM subscription_links WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subscription link: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("subscription delete rows affected: %w", err)
	}
	if affected == 0 {
		return ErrSubscriptionNotFound
	}

	return nil
}

// CountSubscriptionsByFilename returns how many subscriptions reference the given rule filename.
func (r *TrafficRepository) CountSubscriptionsByFilename(ctx context.Context, filename string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		return 0, errors.New("rule filename is required")
	}

	var count int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM subscription_links WHERE rule_filename = ?`, filename).Scan(&count); err != nil {
		return 0, fmt.Errorf("count subscription by filename: %w", err)
	}

	return count, nil
}

// GetProbeConfig returns the current probe configuration with associated servers.
func (r *TrafficRepository) GetProbeConfig(ctx context.Context) (ProbeConfig, error) {
	var cfg ProbeConfig
	if r == nil || r.db == nil {
		return cfg, errors.New("traffic repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, probe_type, address, created_at, updated_at FROM probe_configs WHERE id = 1 LIMIT 1`)
	result, err := scanProbeConfig(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cfg, ErrProbeConfigNotFound
		}
		return cfg, fmt.Errorf("get probe config: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, config_id, server_id, name, traffic_method, monthly_traffic_bytes, position, created_at, updated_at FROM probe_servers WHERE config_id = ? ORDER BY position ASC, id ASC`, result.ID)
	if err != nil {
		return cfg, fmt.Errorf("list probe servers: %w", err)
	}
	defer rows.Close()

	var servers []ProbeServer
	for rows.Next() {
		server, err := scanProbeServer(rows)
		if err != nil {
			return cfg, fmt.Errorf("scan probe server: %w", err)
		}
		servers = append(servers, server)
	}

	if err := rows.Err(); err != nil {
		return cfg, fmt.Errorf("iterate probe servers: %w", err)
	}

	result.Servers = servers

	return result, nil
}

// UpsertProbeConfig updates the singleton probe configuration and replaces its server list.
func (r *TrafficRepository) UpsertProbeConfig(ctx context.Context, cfg ProbeConfig) (ProbeConfig, error) {
	if r == nil || r.db == nil {
		return ProbeConfig{}, errors.New("traffic repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	type sanitizedServer struct {
		ServerID            string
		Name                string
		TrafficMethod       string
		MonthlyTrafficBytes int64
	}

	cfg.ProbeType = strings.ToLower(strings.TrimSpace(cfg.ProbeType))
	if _, ok := allowedProbeTypes[cfg.ProbeType]; !ok {
		return ProbeConfig{}, errors.New("unsupported probe type")
	}

	cfg.Address = strings.TrimSpace(cfg.Address)
	if cfg.Address == "" {
		return ProbeConfig{}, errors.New("probe address is required")
	}

	if len(cfg.Servers) == 0 {
		return ProbeConfig{}, errors.New("at least one server is required")
	}

	sanitized := make([]sanitizedServer, 0, len(cfg.Servers))
	for idx, srv := range cfg.Servers {
		serverID := strings.TrimSpace(srv.ServerID)
		if serverID == "" {
			return ProbeConfig{}, fmt.Errorf("server %d: server id is required", idx+1)
		}

		name := strings.TrimSpace(srv.Name)
		if name == "" {
			return ProbeConfig{}, fmt.Errorf("server %d: server name is required", idx+1)
		}

		method := strings.ToLower(strings.TrimSpace(srv.TrafficMethod))
		if _, ok := allowedTrafficMethods[method]; !ok {
			return ProbeConfig{}, fmt.Errorf("server %d: unsupported traffic method", idx+1)
		}

		if srv.MonthlyTrafficBytes < 0 {
			return ProbeConfig{}, fmt.Errorf("server %d: monthly traffic cannot be negative", idx+1)
		}

		sanitized = append(sanitized, sanitizedServer{
			ServerID:            serverID,
			Name:                name,
			TrafficMethod:       method,
			MonthlyTrafficBytes: srv.MonthlyTrafficBytes,
		})
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ProbeConfig{}, fmt.Errorf("begin probe config tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO probe_configs (id, probe_type, address) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET probe_type = excluded.probe_type, address = excluded.address, updated_at = CURRENT_TIMESTAMP`, cfg.ProbeType, cfg.Address); err != nil {
		return ProbeConfig{}, fmt.Errorf("upsert probe config: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM probe_servers WHERE config_id = 1`); err != nil {
		return ProbeConfig{}, fmt.Errorf("clear probe servers: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO probe_servers (config_id, server_id, name, traffic_method, monthly_traffic_bytes, position) VALUES (1, ?, ?, ?, ?, ?)`)
	if err != nil {
		return ProbeConfig{}, fmt.Errorf("prepare insert probe server: %w", err)
	}
	defer stmt.Close()

	for idx, srv := range sanitized {
		if _, err := stmt.ExecContext(ctx, srv.ServerID, srv.Name, srv.TrafficMethod, srv.MonthlyTrafficBytes, idx); err != nil {
			return ProbeConfig{}, fmt.Errorf("insert probe server %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ProbeConfig{}, fmt.Errorf("commit probe config: %w", err)
	}

	return r.GetProbeConfig(ctx)
}

func (r *TrafficRepository) ensureDefaultProbeConfig() error {
	// No longer creating default probe configuration
	// Users must configure probe settings via the web interface
	return nil
}

func (r *TrafficRepository) migrateProbeConfigsForNezhaV0() error {
	// Check if table exists first
	rows, err := r.db.Query(`SELECT sql FROM sqlite_master WHERE type='table' AND name='probe_configs'`)
	if err != nil {
		return fmt.Errorf("query schema: %w", err)
	}
	defer rows.Close()

	var schemaSql string
	if rows.Next() {
		if err := rows.Scan(&schemaSql); err != nil {
			return fmt.Errorf("scan schema: %w", err)
		}
	} else {
		// Table doesn't exist yet, no migration needed
		return nil
	}
	rows.Close()

	// If schema already contains nezhav0, no migration needed
	if strings.Contains(schemaSql, "nezhav0") {
		return nil
	}

	// If old schema doesn't contain probe_type check, also skip (brand new table will be created correctly)
	if !strings.Contains(schemaSql, "probe_type") {
		return nil
	}

	// Need to migrate: recreate table with new CHECK constraint
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create new table with updated schema
	_, err = tx.Exec(`
CREATE TABLE probe_configs_new (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    probe_type TEXT NOT NULL CHECK (probe_type IN ('nezha','nezhav0','dstatus','komari')),
    address TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`)
	if err != nil {
		return fmt.Errorf("create new table: %w", err)
	}

	// Copy data from old table
	_, err = tx.Exec(`INSERT INTO probe_configs_new SELECT * FROM probe_configs`)
	if err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	// Drop old table
	_, err = tx.Exec(`DROP TABLE probe_configs`)
	if err != nil {
		return fmt.Errorf("drop old table: %w", err)
	}

	// Rename new table
	_, err = tx.Exec(`ALTER TABLE probe_configs_new RENAME TO probe_configs`)
	if err != nil {
		return fmt.Errorf("rename table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (r *TrafficRepository) ensureUserColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(users)`)
	if err != nil {
		return fmt.Errorf("users table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE users ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) ensureUserTokenColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(user_tokens)`)
	if err != nil {
		return fmt.Errorf("user_tokens table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE user_tokens ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) ensureSubscriptionLinkColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(subscription_links)`)
	if err != nil {
		return fmt.Errorf("subscription_links table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE subscription_links ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) ensureNodeColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(nodes)`)
	if err != nil {
		return fmt.Errorf("nodes table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE nodes ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) ensureUserSettingsColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(user_settings)`)
	if err != nil {
		return fmt.Errorf("user_settings table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE user_settings ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) migrateCustomRulesAppendMode() error {
	// Check if table already supports 'append' mode by trying to insert a dummy row
	// If it fails, we need to recreate the table
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if append mode is already supported
	_, err = tx.Exec(`INSERT INTO custom_rules (name, type, mode, content) VALUES ('__test_append__', 'rules', 'append', 'test')`)
	if err == nil {
		// append mode is supported, clean up test row
		tx.Exec(`DELETE FROM custom_rules WHERE name = '__test_append__'`)
		tx.Commit()
		return nil
	}

	// Need to migrate - recreate table with new constraint
	// 1. Rename old table
	if _, err := tx.Exec(`ALTER TABLE custom_rules RENAME TO custom_rules_old`); err != nil {
		return fmt.Errorf("rename old table: %w", err)
	}

	// 2. Create new table with updated constraint
	const newTableSchema = `
CREATE TABLE custom_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('dns','rules','rule-providers')),
    mode TEXT NOT NULL CHECK (mode IN ('replace','prepend','append')),
    content TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, type)
);
CREATE INDEX IF NOT EXISTS idx_custom_rules_type ON custom_rules(type);
CREATE INDEX IF NOT EXISTS idx_custom_rules_enabled ON custom_rules(enabled);
`
	if _, err := tx.Exec(newTableSchema); err != nil {
		return fmt.Errorf("create new table: %w", err)
	}

	// 3. Copy data from old table
	if _, err := tx.Exec(`
		INSERT INTO custom_rules (id, name, type, mode, content, enabled, created_at, updated_at)
		SELECT id, name, type, mode, content, enabled, created_at, updated_at
		FROM custom_rules_old
	`); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	// 4. Drop old table
	if _, err := tx.Exec(`DROP TABLE custom_rules_old`); err != nil {
		return fmt.Errorf("drop old table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (r *TrafficRepository) ensureSubscribeFileColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(subscribe_files)`)
	if err != nil {
		return fmt.Errorf("subscribe_files table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE subscribe_files ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) ensureExternalSubscriptionColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(external_subscriptions)`)
	if err != nil {
		return fmt.Errorf("external_subscriptions table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE external_subscriptions ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) ensureProxyProviderConfigColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(proxy_provider_configs)`)
	if err != nil {
		return fmt.Errorf("proxy_provider_configs table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE proxy_provider_configs ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) ensureSystemConfigColumn(name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(system_config)`)
	if err != nil {
		return fmt.Errorf("system_config table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}

	alter := fmt.Sprintf("ALTER TABLE system_config ADD COLUMN %s %s", name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func (r *TrafficRepository) syncNicknames() error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if _, err := r.db.Exec(`UPDATE users SET nickname = username WHERE nickname IS NULL OR nickname = ''`); err != nil {
		return fmt.Errorf("sync nicknames: %w", err)
	}

	return nil
}

// RecordDaily upserts the aggregated traffic usage for the provided date.
func (r *TrafficRepository) RecordDaily(ctx context.Context, date time.Time, totalLimit, totalUsed, totalRemaining int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	normalized := date.UTC().Format("2006-01-02")

	const stmt = `
INSERT INTO traffic_records (date, total_limit, total_used, total_remaining)
VALUES (?, ?, ?, ?)
ON CONFLICT(date) DO UPDATE SET
    total_limit = excluded.total_limit,
    total_used = excluded.total_used,
    total_remaining = excluded.total_remaining,
    created_at = CURRENT_TIMESTAMP;
`

	if _, err := r.db.ExecContext(ctx, stmt, normalized, totalLimit, totalUsed, totalRemaining); err != nil {
		return fmt.Errorf("upsert traffic record: %w", err)
	}

	return nil
}

// ListRecent returns up to the requested number of most recent traffic records, ordered from newest to oldest.
func (r *TrafficRepository) ListRecent(ctx context.Context, limit int) ([]TrafficRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	if limit <= 0 {
		limit = 30
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT date, total_limit, total_used, total_remaining
FROM traffic_records
ORDER BY date DESC
LIMIT ?;
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent traffic records: %w", err)
	}
	defer rows.Close()

	var records []TrafficRecord
	for rows.Next() {
		var (
			dateStr        string
			totalLimit     int64
			totalUsed      int64
			totalRemaining int64
		)

		if err := rows.Scan(&dateStr, &totalLimit, &totalUsed, &totalRemaining); err != nil {
			return nil, fmt.Errorf("scan traffic record: %w", err)
		}

		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("parse traffic record date: %w", err)
		}

		records = append(records, TrafficRecord{
			Date:           parsed,
			TotalLimit:     totalLimit,
			TotalUsed:      totalUsed,
			TotalRemaining: totalRemaining,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traffic records: %w", err)
	}

	return records, nil
}

// GetOrCreateUserToken returns the existing token for the given username or creates a new one.
func (r *TrafficRepository) GetOrCreateUserToken(ctx context.Context, username string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}

	const selectStmt = `SELECT token FROM user_tokens WHERE username = ? LIMIT 1;`
	var token string
	if err := r.db.QueryRowContext(ctx, selectStmt, username).Scan(&token); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("query user token: %w", err)
		}

		// Generate new token and user short code with retry logic
		newToken := uuid.NewString()
		const maxRetries = 10
		for i := 0; i < maxRetries; i++ {
			newUserShortCode, err := generateUserShortCode()
			if err != nil {
				return "", fmt.Errorf("generate user short code: %w", err)
			}

			const insertStmt = `INSERT INTO user_tokens (username, token, user_short_code, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP);`
			if _, err := r.db.ExecContext(ctx, insertStmt, username, newToken, newUserShortCode); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") && strings.Contains(strings.ToLower(err.Error()), "user_short_code") {
					// User short code collision, retry
					continue
				}
				return "", fmt.Errorf("insert user token: %w", err)
			}
			break
		}
		token = newToken
	}

	return token, nil
}

// ResetUserToken generates and stores a new token for the provided username.
func (r *TrafficRepository) ResetUserToken(ctx context.Context, username string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}

	newToken := uuid.NewString()

	// Generate new user short code with retry logic
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		newUserShortCode, err := generateUserShortCode()
		if err != nil {
			return "", fmt.Errorf("generate user short code: %w", err)
		}

		const stmt = `
INSERT INTO user_tokens (username, token, user_short_code, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(username) DO UPDATE SET
    token = excluded.token,
    user_short_code = excluded.user_short_code,
    updated_at = CURRENT_TIMESTAMP;
`

		if _, err := r.db.ExecContext(ctx, stmt, username, newToken, newUserShortCode); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") && strings.Contains(strings.ToLower(err.Error()), "user_short_code") {
				// User short code collision, retry
				continue
			}
			return "", fmt.Errorf("reset user token: %w", err)
		}

		return newToken, nil
	}

	return "", errors.New("failed to generate unique user short code after retries")
}

// ValidateUserToken returns the username associated with the provided token if it exists.
func (r *TrafficRepository) ValidateUserToken(ctx context.Context, token string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token is required")
	}

	const stmt = `SELECT username FROM user_tokens WHERE token = ? LIMIT 1;`
	var username string
	if err := r.db.QueryRowContext(ctx, stmt, token).Scan(&username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrTokenNotFound
		}
		return "", fmt.Errorf("query user token by value: %w", err)
	}

	return username, nil
}

// generateFileShortCode generates a random 3-character string for file short codes.
func generateFileShortCode() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 3

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	for i := range bytes {
		bytes[i] = charset[int(bytes[i])%len(charset)]
	}

	return string(bytes), nil
}

// generateUserShortCode generates a random 3-character string for user short codes.
func generateUserShortCode() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 3

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	for i := range bytes {
		bytes[i] = charset[int(bytes[i])%len(charset)]
	}

	return string(bytes), nil
}

// generateMissingFileShortCodes generates file short codes for subscribe_files that don't have one.
func (r *TrafficRepository) generateMissingFileShortCodes() error {
	// Get all subscribe_files without file short codes
	rows, err := r.db.Query(`SELECT id FROM subscribe_files WHERE file_short_code = '' OR file_short_code IS NULL`)
	if err != nil {
		return fmt.Errorf("query subscribe files without file short codes: %w", err)
	}
	defer rows.Close()

	var fileIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan file ID: %w", err)
		}
		fileIDs = append(fileIDs, id)
	}

	// Generate file short code for each file
	for _, id := range fileIDs {
		const maxRetries = 10
		for i := 0; i < maxRetries; i++ {
			newShortCode, err := generateFileShortCode()
			if err != nil {
				return fmt.Errorf("generate file short code: %w", err)
			}

			_, err = r.db.Exec(`UPDATE subscribe_files SET file_short_code = ? WHERE id = ?`, newShortCode, id)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") {
					continue // Retry with a new short code
				}
				return fmt.Errorf("update file short code for file %d: %w", id, err)
			}
			break // Success, move to next file
		}
	}

	return nil
}

// generateMissingUserShortCodes generates user short codes for user_tokens that don't have one.
func (r *TrafficRepository) generateMissingUserShortCodes() error {
	// Get all user_tokens without user short codes
	rows, err := r.db.Query(`SELECT username FROM user_tokens WHERE user_short_code = '' OR user_short_code IS NULL`)
	if err != nil {
		return fmt.Errorf("query users without user short codes: %w", err)
	}
	defer rows.Close()

	var usernames []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return fmt.Errorf("scan username: %w", err)
		}
		usernames = append(usernames, username)
	}

	// Generate user short code for each user
	for _, username := range usernames {
		const maxRetries = 10
		for i := 0; i < maxRetries; i++ {
			newShortCode, err := generateUserShortCode()
			if err != nil {
				return fmt.Errorf("generate user short code: %w", err)
			}

			_, err = r.db.Exec(`UPDATE user_tokens SET user_short_code = ? WHERE username = ?`, newShortCode, username)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") {
					continue // Retry with a new short code
				}
				return fmt.Errorf("update user short code for user %s: %w", username, err)
			}
			break // Success, move to next user
		}
	}

	return nil
}

// ResetAllSubscriptionShortURLs resets file short codes for all subscribe_files.
// This is called when the user clicks "reset short link" button in settings.
func (r *TrafficRepository) ResetAllSubscriptionShortURLs(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	// Get all subscribe_files IDs
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM subscribe_files`)
	if err != nil {
		return fmt.Errorf("query subscribe_files IDs: %w", err)
	}
	defer rows.Close()

	var fileIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan file ID: %w", err)
		}
		fileIDs = append(fileIDs, id)
	}

	// Reset file short code for each subscribe_file
	for _, id := range fileIDs {
		if err := r.resetFileShortCode(ctx, id); err != nil {
			return fmt.Errorf("reset file short code for file %d: %w", id, err)
		}
	}

	return nil
}

// resetFileShortCode resets the file short code for a single subscribe_file.
func (r *TrafficRepository) resetFileShortCode(ctx context.Context, fileID int64) error {
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		newShortCode, err := generateFileShortCode()
		if err != nil {
			return fmt.Errorf("generate file short code: %w", err)
		}

		const updateStmt = `UPDATE subscribe_files SET file_short_code = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;`
		_, err = r.db.ExecContext(ctx, updateStmt, newShortCode, fileID)
		if err != nil {
			// Check if it's a unique constraint violation
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue // Try again with a different short code
			}
			return fmt.Errorf("update file short code: %w", err)
		}

		return nil
	}

	return errors.New("failed to generate unique short URL after retries")
}

// GetSubscriptionByShortURL returns the subscription filename associated with a short URL.
func (r *TrafficRepository) GetSubscriptionByShortURL(ctx context.Context, shortcode string) (filename string, err error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	shortcode = strings.TrimSpace(shortcode)
	if shortcode == "" {
		return "", errors.New("shortcode is required")
	}

	const stmt = `SELECT rule_filename FROM subscription_links WHERE short_url = ? LIMIT 1;`
	if err := r.db.QueryRowContext(ctx, stmt, shortcode).Scan(&filename); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSubscriptionNotFound
		}
		return "", fmt.Errorf("query subscription by short URL: %w", err)
	}

	return filename, nil
}

// GetFilenameByFileShortCode returns the subscription filename associated with a file short code.
func (r *TrafficRepository) GetFilenameByFileShortCode(ctx context.Context, fileShortCode string) (filename string, err error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	fileShortCode = strings.TrimSpace(fileShortCode)
	if fileShortCode == "" {
		return "", errors.New("file short code is required")
	}

	const stmt = `SELECT filename FROM subscribe_files WHERE file_short_code = ? LIMIT 1;`
	if err := r.db.QueryRowContext(ctx, stmt, fileShortCode).Scan(&filename); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSubscribeFileNotFound
		}
		return "", fmt.Errorf("query subscribe file by file short code: %w", err)
	}

	return filename, nil
}

// GetUsernameByUserShortCode returns the username associated with a user short code.
func (r *TrafficRepository) GetUsernameByUserShortCode(ctx context.Context, userShortCode string) (username string, err error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	userShortCode = strings.TrimSpace(userShortCode)
	if userShortCode == "" {
		return "", errors.New("user short code is required")
	}

	const stmt = `SELECT username FROM user_tokens WHERE user_short_code = ? LIMIT 1;`
	if err := r.db.QueryRowContext(ctx, stmt, userShortCode).Scan(&username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("user not found")
		}
		return "", fmt.Errorf("query user by user short code: %w", err)
	}

	return username, nil
}

// GetUserShortCode returns the user short code for a given username.
func (r *TrafficRepository) GetUserShortCode(ctx context.Context, username string) (userShortCode string, err error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}

	const stmt = `SELECT user_short_code FROM user_tokens WHERE username = ? LIMIT 1;`
	if err := r.db.QueryRowContext(ctx, stmt, username).Scan(&userShortCode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("user short code not found")
		}
		return "", fmt.Errorf("query user short code: %w", err)
	}

	return userShortCode, nil
}

// SaveRuleVersion persists a new rule version for the provided filename and returns the new version number.
func (r *TrafficRepository) SaveRuleVersion(ctx context.Context, filename, content, createdBy string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}

	filename = strings.TrimSpace(filename)
	createdBy = strings.TrimSpace(createdBy)
	if filename == "" {
		return 0, errors.New("filename is required")
	}
	if createdBy == "" {
		return 0, errors.New("createdBy is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()

	var currentVersion sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT MAX(version) FROM rule_versions WHERE filename = ?`, filename).Scan(&currentVersion); err != nil {
		return 0, fmt.Errorf("query max version: %w", err)
	}

	newVersion := int64(1)
	if currentVersion.Valid {
		newVersion = currentVersion.Int64 + 1
	}

	if _, err = tx.ExecContext(ctx, `INSERT INTO rule_versions (filename, version, content, created_by) VALUES (?, ?, ?, ?)`, filename, newVersion, content, createdBy); err != nil {
		return 0, fmt.Errorf("insert rule version: %w", err)
	}

	return newVersion, nil
}

// ListRuleVersions returns the most recent rule versions for a file.
func (r *TrafficRepository) ListRuleVersions(ctx context.Context, filename string, limit int) ([]RuleVersion, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, errors.New("filename is required")
	}

	if limit <= 0 {
		limit = 10
	}

	rows, err := r.db.QueryContext(ctx, `SELECT version, content, created_by, created_at FROM rule_versions WHERE filename = ? ORDER BY version DESC LIMIT ?`, filename, limit)
	if err != nil {
		return nil, fmt.Errorf("query rule versions: %w", err)
	}
	defer rows.Close()

	var versions []RuleVersion
	for rows.Next() {
		var rv RuleVersion
		rv.Filename = filename
		if err := rows.Scan(&rv.Version, &rv.Content, &rv.CreatedBy, &rv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan rule version: %w", err)
		}
		versions = append(versions, rv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rule versions: %w", err)
	}

	return versions, nil
}

// LatestRuleVersion returns the most recent stored version for the provided rule file.
func (r *TrafficRepository) LatestRuleVersion(ctx context.Context, filename string) (RuleVersion, error) {
	versions, err := r.ListRuleVersions(ctx, filename, 1)
	if err != nil {
		return RuleVersion{}, err
	}
	if len(versions) == 0 {
		return RuleVersion{}, ErrRuleVersionNotFound
	}
	return versions[0], nil
}

// RuleVersion represents an archived version of a YAML rule file.
type RuleVersion struct {
	Filename  string
	Version   int64
	Content   string
	CreatedBy string
	CreatedAt time.Time
}

// User represents an authenticated account stored in the repository.
type User struct {
	Username     string
	PasswordHash string
	Email        string
	Nickname     string
	AvatarURL    string
	Role         string
	IsActive     bool
	Remark       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserProfileUpdate captures editable profile fields for a user.
type UserProfileUpdate struct {
	Email     string
	Nickname  string
	AvatarURL string
}

// EnsureUser inserts or updates the provided user.
func (r *TrafficRepository) EnsureUser(ctx context.Context, username, passwordHash string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if passwordHash == "" {
		return errors.New("password hash is required")
	}

	_, err := r.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, nickname, role) VALUES (?, ?, ?, ?) ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`, username, passwordHash, username, RoleUser)
	if err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}

	return nil
}

// CreateUser inserts a brand new user with the provided credentials. Returns ErrUserExists if username already present.
func (r *TrafficRepository) CreateUser(ctx context.Context, username, email, nickname, passwordHash, role, remark string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	nickname = strings.TrimSpace(nickname)
	role = strings.TrimSpace(role)
	remark = strings.TrimSpace(remark)

	if username == "" {
		return errors.New("username is required")
	}
	if passwordHash == "" {
		return errors.New("password hash is required")
	}
	if nickname == "" {
		nickname = username
	}
	if role == "" {
		role = RoleUser
	}
	role = strings.ToLower(role)
	if role != RoleAdmin {
		role = RoleUser
	}

	_, err := r.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, email, nickname, role, is_active, remark) VALUES (?, ?, ?, ?, ?, 1, ?)`, username, passwordHash, email, nickname, role, remark)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrUserExists
		}
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by username.
func (r *TrafficRepository) GetUser(ctx context.Context, username string) (User, error) {
	var user User
	if r == nil || r.db == nil {
		return user, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return user, errors.New("username is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT username, password_hash, COALESCE(email, ''), COALESCE(nickname, ''), COALESCE(avatar_url, ''), COALESCE(role, ''), is_active, created_at, updated_at FROM users WHERE username = ? LIMIT 1`, username)
	var active int
	if err := row.Scan(&user.Username, &user.PasswordHash, &user.Email, &user.Nickname, &user.AvatarURL, &user.Role, &active, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, ErrUserNotFound
		}
		return user, fmt.Errorf("get user: %w", err)
	}
	if user.Nickname == "" {
		user.Nickname = user.Username
	}
	if user.Role == "" {
		user.Role = RoleUser
	}
	user.IsActive = active != 0

	return user, nil
}

// ListUsers returns up to limit users ordered by creation time.
func (r *TrafficRepository) ListUsers(ctx context.Context, limit int) ([]User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	if limit <= 0 {
		limit = 10
	}

	rows, err := r.db.QueryContext(ctx, `SELECT username, password_hash, COALESCE(email, ''), COALESCE(nickname, ''), COALESCE(avatar_url, ''), COALESCE(role, ''), is_active, COALESCE(remark, ''), created_at, updated_at FROM users ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var active int
		if err := rows.Scan(&user.Username, &user.PasswordHash, &user.Email, &user.Nickname, &user.AvatarURL, &user.Role, &active, &user.Remark, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		if user.Nickname == "" {
			user.Nickname = user.Username
		}
		if user.Role == "" {
			user.Role = RoleUser
		}
		user.IsActive = active != 0
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

// UpdateUserRemark updates the remark field for the specified user.
func (r *TrafficRepository) UpdateUserRemark(ctx context.Context, username, remark string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	const stmt = `UPDATE users SET remark = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`
	_, err := r.db.ExecContext(ctx, stmt, remark, username)
	if err != nil {
		return fmt.Errorf("update user remark: %w", err)
	}
	return nil
}

// UpdateUserPassword updates the stored password hash for the specified user.
func (r *TrafficRepository) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if passwordHash == "" {
		return errors.New("password hash is required")
	}

	res, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, passwordHash, username)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("password rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateUserRole sets the role for the specified user.
func (r *TrafficRepository) UpdateUserRole(ctx context.Context, username, role string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	role = strings.TrimSpace(role)
	if username == "" {
		return errors.New("username is required")
	}
	if role == "" {
		role = RoleUser
	}
	role = strings.ToLower(role)
	if role != RoleAdmin {
		role = RoleUser
	}

	res, err := r.db.ExecContext(ctx, `UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, role, username)
	if err != nil {
		return fmt.Errorf("update user role: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("role rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateUserStatus toggles the active state for a user.
func (r *TrafficRepository) UpdateUserStatus(ctx context.Context, username string, active bool) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	value := 0
	if active {
		value = 1
	}

	res, err := r.db.ExecContext(ctx, `UPDATE users SET is_active = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, value, username)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("status rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// DeleteUser removes a user and all related data (subscriptions, sessions, nodes, etc.)
func (r *TrafficRepository) DeleteUser(ctx context.Context, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	// Start a transaction to ensure all deletions succeed or fail together
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete user's subscriptions bindings
	_, err = tx.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user subscriptions: %w", err)
	}

	// Delete user's sessions
	_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}

	// Delete user's nodes
	_, err = tx.ExecContext(ctx, `DELETE FROM nodes WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user nodes: %w", err)
	}

	// Delete user's external subscriptions
	_, err = tx.ExecContext(ctx, `DELETE FROM external_subscriptions WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user external subscriptions: %w", err)
	}

	// Delete user's settings
	_, err = tx.ExecContext(ctx, `DELETE FROM user_settings WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user settings: %w", err)
	}

	// Delete user's token
	_, err = tx.ExecContext(ctx, `DELETE FROM user_tokens WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user token: %w", err)
	}

	// Finally, delete the user
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete user rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// UpdateUserNickname updates the nickname associated with a user account.
func (r *TrafficRepository) UpdateUserNickname(ctx context.Context, username, nickname string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	nickname = strings.TrimSpace(nickname)

	if username == "" {
		return errors.New("username is required")
	}
	if nickname == "" {
		nickname = username
	}

	res, err := r.db.ExecContext(ctx, `UPDATE users SET nickname = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, nickname, username)
	if err != nil {
		return fmt.Errorf("update user nickname: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("nickname rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateUserProfile updates editable profile fields for the specified user.
func (r *TrafficRepository) UpdateUserProfile(ctx context.Context, username string, profile UserProfileUpdate) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	email := strings.TrimSpace(profile.Email)
	nickname := strings.TrimSpace(profile.Nickname)
	avatar := strings.TrimSpace(profile.AvatarURL)

	if nickname == "" {
		nickname = username
	}

	res, err := r.db.ExecContext(ctx, `UPDATE users SET email = ?, nickname = ?, avatar_url = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, email, nickname, avatar, username)
	if err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("profile rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// RenameUser changes a username and updates dependent tables.
func (r *TrafficRepository) RenameUser(ctx context.Context, oldUsername, newUsername string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	oldUsername = strings.TrimSpace(oldUsername)
	newUsername = strings.TrimSpace(newUsername)
	if oldUsername == "" || newUsername == "" {
		return errors.New("usernames are required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rename user begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()

	res, err := tx.ExecContext(ctx, `UPDATE users SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, newUsername, oldUsername)
	if err != nil {
		return fmt.Errorf("rename user: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rename user rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	if _, err = tx.ExecContext(ctx, `UPDATE user_tokens SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, newUsername, oldUsername); err != nil {
		return fmt.Errorf("rename user tokens: %w", err)
	}

	return nil
}

// Session represents an authenticated session stored in the database.
type Session struct {
	Token     string
	Username  string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// CreateSession persists a new session to the database.
func (r *TrafficRepository) CreateSession(ctx context.Context, token, username string, expiresAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	token = strings.TrimSpace(token)
	username = strings.TrimSpace(username)
	if token == "" {
		return errors.New("token is required")
	}
	if username == "" {
		return errors.New("username is required")
	}

	const stmt = `INSERT INTO sessions (token, username, expires_at) VALUES (?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, stmt, token, username, expiresAt); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

// DeleteSession removes a session from the database.
func (r *TrafficRepository) DeleteSession(ctx context.Context, token string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token is required")
	}

	const stmt = `DELETE FROM sessions WHERE token = ?`
	if _, err := r.db.ExecContext(ctx, stmt, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// DeleteUserSessions removes all sessions for a specific user.
func (r *TrafficRepository) DeleteUserSessions(ctx context.Context, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	const stmt = `DELETE FROM sessions WHERE username = ?`
	if _, err := r.db.ExecContext(ctx, stmt, username); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}

	return nil
}

// LoadSessions retrieves all non-expired sessions from the database.
func (r *TrafficRepository) LoadSessions(ctx context.Context) ([]Session, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	const stmt = `SELECT token, username, expires_at, created_at FROM sessions WHERE expires_at > datetime('now') ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("load sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.Token, &session.Username, &session.ExpiresAt, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}

	return sessions, nil
}

// CleanupExpiredSessions removes expired sessions from the database.
func (r *TrafficRepository) CleanupExpiredSessions(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	const stmt = `DELETE FROM sessions WHERE expires_at <= datetime('now')`
	if _, err := r.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("cleanup expired sessions: %w", err)
	}

	return nil
}

// AssignSubscriptionToUser assigns a subscription to a user.
func (r *TrafficRepository) AssignSubscriptionToUser(ctx context.Context, username string, subscriptionID int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if subscriptionID <= 0 {
		return errors.New("invalid subscription ID")
	}

	_, err := r.db.ExecContext(ctx, `INSERT INTO user_subscriptions (username, subscription_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, username, subscriptionID)
	if err != nil {
		return fmt.Errorf("assign subscription to user: %w", err)
	}

	return nil
}

// RemoveSubscriptionFromUser removes a subscription assignment from a user.
func (r *TrafficRepository) RemoveSubscriptionFromUser(ctx context.Context, username string, subscriptionID int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if subscriptionID <= 0 {
		return errors.New("invalid subscription ID")
	}

	_, err := r.db.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE username = ? AND subscription_id = ?`, username, subscriptionID)
	if err != nil {
		return fmt.Errorf("remove subscription from user: %w", err)
	}

	return nil
}

// GetUserSubscriptionIDs returns all subscription IDs assigned to a user.
// Only returns IDs that exist in subscribe_files table (filters out orphaned records).
func (r *TrafficRepository) GetUserSubscriptionIDs(ctx context.Context, username string) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}

	// Join with subscribe_files to only return valid subscription IDs
	const stmt = `
		SELECT us.subscription_id
		FROM user_subscriptions us
		INNER JOIN subscribe_files sf ON us.subscription_id = sf.id
		WHERE us.username = ?
		ORDER BY us.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, stmt, username)
	if err != nil {
		return nil, fmt.Errorf("get user subscription IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan subscription ID: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscription IDs: %w", err)
	}

	return ids, nil
}

// SetUserSubscriptions replaces all subscriptions for a user with the provided list.
func (r *TrafficRepository) SetUserSubscriptions(ctx context.Context, username string, subscriptionIDs []int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	// 使用事务确保原子性
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 删除用户的所有现有订阅
	_, err = tx.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete existing subscriptions: %w", err)
	}

	// 插入新的订阅
	if len(subscriptionIDs) > 0 {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO user_subscriptions (username, subscription_id) VALUES (?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare insert statement: %w", err)
		}
		defer stmt.Close()

		for _, id := range subscriptionIDs {
			if id <= 0 {
				continue
			}
			_, err = stmt.ExecContext(ctx, username, id)
			if err != nil {
				return fmt.Errorf("insert subscription %d: %w", id, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// GetUserSubscriptions returns all subscriptions assigned to a user.
func (r *TrafficRepository) GetUserSubscriptions(ctx context.Context, username string) ([]SubscribeFile, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}

	const stmt = `
		SELECT s.id, s.name, COALESCE(s.description, ''), COALESCE(s.url, ''), s.type, s.filename, COALESCE(s.file_short_code, ''), COALESCE(s.auto_sync_custom_rules, 0), s.expire_at, s.created_at, s.updated_at
		FROM subscribe_files s
		INNER JOIN user_subscriptions us ON s.id = us.subscription_id
		WHERE us.username = ?
		ORDER BY s.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, stmt, username)
	if err != nil {
		return nil, fmt.Errorf("get user subscriptions: %w", err)
	}
	defer rows.Close()

	var subscriptions []SubscribeFile
	for rows.Next() {
		var sub SubscribeFile
		var autoSync int
		var expireAt sql.NullTime
		if err := rows.Scan(&sub.ID, &sub.Name, &sub.Description, &sub.URL, &sub.Type, &sub.Filename, &sub.FileShortCode, &autoSync, &expireAt, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		sub.AutoSyncCustomRules = autoSync != 0
		if expireAt.Valid {
			sub.ExpireAt = &expireAt.Time
		}
		subscriptions = append(subscriptions, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}

	return subscriptions, nil
}

// GetUserSettings retrieves user settings for a given username.
func (r *TrafficRepository) GetUserSettings(ctx context.Context, username string) (UserSettings, error) {
	var settings UserSettings
	if r == nil || r.db == nil {
		return settings, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return settings, errors.New("username is required")
	}

	const stmt = `SELECT username, force_sync_external, COALESCE(match_rule, 'node_name'), COALESCE(sync_scope, 'saved_only'), COALESCE(keep_node_name, 1), COALESCE(cache_expire_minutes, 0), COALESCE(sync_traffic, 0), COALESCE(enable_probe_binding, 0), COALESCE(custom_rules_enabled, 0), COALESCE(enable_short_link, 0), COALESCE(use_new_template_system, 1), COALESCE(enable_proxy_provider, 0), COALESCE(node_order, '[]'), COALESCE(debug_enabled, 0), COALESCE(debug_log_path, ''), debug_started_at, created_at, updated_at FROM user_settings WHERE username = ? LIMIT 1`
	var forceSyncInt, keepNodeNameInt, syncTrafficInt, enableProbeBindingInt, customRulesEnabledInt, enableShortLinkInt, useNewTemplateSystemInt, enableProxyProviderInt, debugEnabledInt int
	var nodeOrderJSON string
	var debugStartedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, stmt, username).Scan(&settings.Username, &forceSyncInt, &settings.MatchRule, &settings.SyncScope, &keepNodeNameInt, &settings.CacheExpireMinutes, &syncTrafficInt, &enableProbeBindingInt, &customRulesEnabledInt, &enableShortLinkInt, &useNewTemplateSystemInt, &enableProxyProviderInt, &nodeOrderJSON, &debugEnabledInt, &settings.DebugLogPath, &debugStartedAt, &settings.CreatedAt, &settings.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return settings, ErrUserSettingsNotFound
		}
		return settings, fmt.Errorf("get user settings: %w", err)
	}

	settings.ForceSyncExternal = forceSyncInt == 1
	settings.KeepNodeName = keepNodeNameInt == 1
	settings.SyncTraffic = syncTrafficInt == 1
	settings.EnableProbeBinding = enableProbeBindingInt == 1
	settings.CustomRulesEnabled = customRulesEnabledInt == 1
	settings.EnableShortLink = enableShortLinkInt == 1
	settings.UseNewTemplateSystem = useNewTemplateSystemInt == 1
	settings.EnableProxyProvider = enableProxyProviderInt == 1
	settings.DebugEnabled = debugEnabledInt == 1

	// Parse node_order JSON
	if nodeOrderJSON != "" && nodeOrderJSON != "[]" {
		if err := json.Unmarshal([]byte(nodeOrderJSON), &settings.NodeOrder); err != nil {
			// If JSON parsing fails, use empty array
			settings.NodeOrder = []int64{}
		}
	} else {
		settings.NodeOrder = []int64{}
	}

	// Handle nullable debug_started_at
	if debugStartedAt.Valid {
		settings.DebugStartedAt = &debugStartedAt.Time
	}

	return settings, nil
}

// UpsertUserSettings creates or updates user settings.
func (r *TrafficRepository) UpsertUserSettings(ctx context.Context, settings UserSettings) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username := strings.TrimSpace(settings.Username)
	if username == "" {
		return errors.New("username is required")
	}

	forceSyncInt := 0
	if settings.ForceSyncExternal {
		forceSyncInt = 1
	}

	keepNodeNameInt := 1 // default to true
	if !settings.KeepNodeName {
		keepNodeNameInt = 0
	}

	syncTrafficInt := 0
	if settings.SyncTraffic {
		syncTrafficInt = 1
	}

	enableProbeBindingInt := 0
	if settings.EnableProbeBinding {
		enableProbeBindingInt = 1
	}

	customRulesEnabledInt := 0
	if settings.CustomRulesEnabled {
		customRulesEnabledInt = 1
	}

	enableShortLinkInt := 0
	if settings.EnableShortLink {
		enableShortLinkInt = 1
	}

	useNewTemplateSystemInt := 1 // default to true
	if !settings.UseNewTemplateSystem {
		useNewTemplateSystemInt = 0
	}

	enableProxyProviderInt := 0
	if settings.EnableProxyProvider {
		enableProxyProviderInt = 1
	}

	debugEnabledInt := 0
	if settings.DebugEnabled {
		debugEnabledInt = 1
	}

	matchRule := strings.TrimSpace(settings.MatchRule)
	if matchRule == "" {
		matchRule = "node_name"
	}

	syncScope := strings.TrimSpace(settings.SyncScope)
	if syncScope == "" {
		syncScope = "saved_only"
	}

	cacheExpireMinutes := settings.CacheExpireMinutes
	if cacheExpireMinutes < 0 {
		cacheExpireMinutes = 0
	}

	// Serialize node_order to JSON
	nodeOrderJSON := "[]"
	if len(settings.NodeOrder) > 0 {
		nodeOrderBytes, err := json.Marshal(settings.NodeOrder)
		if err == nil {
			nodeOrderJSON = string(nodeOrderBytes)
		}
	}

	const stmt = `
		INSERT INTO user_settings (username, force_sync_external, match_rule, sync_scope, keep_node_name, cache_expire_minutes, sync_traffic, enable_probe_binding, custom_rules_enabled, enable_short_link, use_new_template_system, enable_proxy_provider, node_order, debug_enabled, debug_log_path, debug_started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(username) DO UPDATE SET
			force_sync_external = excluded.force_sync_external,
			match_rule = excluded.match_rule,
			sync_scope = excluded.sync_scope,
			keep_node_name = excluded.keep_node_name,
			cache_expire_minutes = excluded.cache_expire_minutes,
			sync_traffic = excluded.sync_traffic,
			enable_probe_binding = excluded.enable_probe_binding,
			custom_rules_enabled = excluded.custom_rules_enabled,
			enable_short_link = excluded.enable_short_link,
			use_new_template_system = excluded.use_new_template_system,
			enable_proxy_provider = excluded.enable_proxy_provider,
			node_order = excluded.node_order,
			debug_enabled = excluded.debug_enabled,
			debug_log_path = excluded.debug_log_path,
			debug_started_at = excluded.debug_started_at,
			updated_at = CURRENT_TIMESTAMP
	`

	if _, err := r.db.ExecContext(ctx, stmt, username, forceSyncInt, matchRule, syncScope, keepNodeNameInt, cacheExpireMinutes, syncTrafficInt, enableProbeBindingInt, customRulesEnabledInt, enableShortLinkInt, useNewTemplateSystemInt, enableProxyProviderInt, nodeOrderJSON, debugEnabledInt, settings.DebugLogPath, settings.DebugStartedAt); err != nil {
		return fmt.Errorf("upsert user settings: %w", err)
	}

	return nil
}

// ListExternalSubscriptions returns all external subscriptions for a user.
func (r *TrafficRepository) ListExternalSubscriptions(ctx context.Context, username string) ([]ExternalSubscription, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}

	const stmt = `SELECT id, username, name, url, COALESCE(user_agent, 'clash-meta/2.4.0'), node_count, last_sync_at, COALESCE(upload, 0), COALESCE(download, 0), COALESCE(total, 0), expire, COALESCE(traffic_mode, 'both'), created_at, updated_at FROM external_subscriptions WHERE username = ? ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, stmt, username)
	if err != nil {
		return nil, fmt.Errorf("list external subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []ExternalSubscription
	for rows.Next() {
		var sub ExternalSubscription
		var lastSyncAt, expire sql.NullTime
		if err := rows.Scan(&sub.ID, &sub.Username, &sub.Name, &sub.URL, &sub.UserAgent, &sub.NodeCount, &lastSyncAt, &sub.Upload, &sub.Download, &sub.Total, &expire, &sub.TrafficMode, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan external subscription: %w", err)
		}
		if lastSyncAt.Valid {
			sub.LastSyncAt = &lastSyncAt.Time
		}
		if expire.Valid {
			sub.Expire = &expire.Time
		}
		subs = append(subs, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate external subscriptions: %w", err)
	}

	return subs, nil
}

// GetExternalSubscription retrieves an external subscription by ID.
func (r *TrafficRepository) GetExternalSubscription(ctx context.Context, id int64, username string) (ExternalSubscription, error) {
	var sub ExternalSubscription
	if r == nil || r.db == nil {
		return sub, errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return sub, errors.New("subscription id is required")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return sub, errors.New("username is required")
	}

	const stmt = `SELECT id, username, name, url, COALESCE(user_agent, 'clash-meta/2.4.0'), node_count, last_sync_at, COALESCE(upload, 0), COALESCE(download, 0), COALESCE(total, 0), expire, COALESCE(traffic_mode, 'both'), created_at, updated_at FROM external_subscriptions WHERE id = ? AND username = ? LIMIT 1`
	var lastSyncAt, expire sql.NullTime
	err := r.db.QueryRowContext(ctx, stmt, id, username).Scan(&sub.ID, &sub.Username, &sub.Name, &sub.URL, &sub.UserAgent, &sub.NodeCount, &lastSyncAt, &sub.Upload, &sub.Download, &sub.Total, &expire, &sub.TrafficMode, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sub, ErrExternalSubscriptionNotFound
		}
		return sub, fmt.Errorf("get external subscription: %w", err)
	}

	if lastSyncAt.Valid {
		sub.LastSyncAt = &lastSyncAt.Time
	}
	if expire.Valid {
		sub.Expire = &expire.Time
	}

	return sub, nil
}

// CreateExternalSubscription creates a new external subscription.
func (r *TrafficRepository) CreateExternalSubscription(ctx context.Context, sub ExternalSubscription) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}

	username := strings.TrimSpace(sub.Username)
	if username == "" {
		return 0, errors.New("username is required")
	}

	name := strings.TrimSpace(sub.Name)
	if name == "" {
		return 0, errors.New("subscription name is required")
	}

	url := strings.TrimSpace(sub.URL)
	if url == "" {
		return 0, errors.New("subscription url is required")
	}

	userAgent := strings.TrimSpace(sub.UserAgent)
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}

	trafficMode := strings.TrimSpace(sub.TrafficMode)
	if trafficMode == "" {
		trafficMode = "both"
	}

	const stmt = `INSERT INTO external_subscriptions (username, name, url, user_agent, node_count, last_sync_at, upload, download, total, expire, traffic_mode) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := r.db.ExecContext(ctx, stmt, username, name, url, userAgent, sub.NodeCount, sub.LastSyncAt, sub.Upload, sub.Download, sub.Total, sub.Expire, trafficMode)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, ErrExternalSubscriptionExists
		}
		return 0, fmt.Errorf("create external subscription: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}

	return id, nil
}

// UpdateExternalSubscription updates an existing external subscription.
func (r *TrafficRepository) UpdateExternalSubscription(ctx context.Context, sub ExternalSubscription) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if sub.ID <= 0 {
		return errors.New("subscription id is required")
	}

	username := strings.TrimSpace(sub.Username)
	if username == "" {
		return errors.New("username is required")
	}

	name := strings.TrimSpace(sub.Name)
	if name == "" {
		return errors.New("subscription name is required")
	}

	url := strings.TrimSpace(sub.URL)
	if url == "" {
		return errors.New("subscription url is required")
	}

	userAgent := strings.TrimSpace(sub.UserAgent)
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}

	trafficMode := strings.TrimSpace(sub.TrafficMode)
	if trafficMode == "" {
		trafficMode = "both"
	}

	const stmt = `UPDATE external_subscriptions SET name = ?, url = ?, user_agent = ?, node_count = ?, last_sync_at = ?, upload = ?, download = ?, total = ?, expire = ?, traffic_mode = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`
	result, err := r.db.ExecContext(ctx, stmt, name, url, userAgent, sub.NodeCount, sub.LastSyncAt, sub.Upload, sub.Download, sub.Total, sub.Expire, trafficMode, sub.ID, username)
	if err != nil {
		return fmt.Errorf("update external subscription: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrExternalSubscriptionNotFound
	}

	return nil
}

// DeleteExternalSubscription deletes an external subscription.
func (r *TrafficRepository) DeleteExternalSubscription(ctx context.Context, id int64, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return errors.New("subscription id is required")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	// 先删除关联的代理集合配置
	const deleteProvidersStmt = `DELETE FROM proxy_provider_configs WHERE external_subscription_id = ?`
	if _, err := r.db.ExecContext(ctx, deleteProvidersStmt, id); err != nil {
		return fmt.Errorf("delete related proxy provider configs: %w", err)
	}

	const stmt = `DELETE FROM external_subscriptions WHERE id = ? AND username = ?`
	result, err := r.db.ExecContext(ctx, stmt, id, username)
	if err != nil {
		return fmt.Errorf("delete external subscription: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrExternalSubscriptionNotFound
	}

	return nil
}

// Custom Rules CRUD operations

var (
	ErrCustomRuleNotFound = errors.New("custom rule not found")
)

// ListCustomRules returns all custom rules, optionally filtered by type.
func (r *TrafficRepository) ListCustomRules(ctx context.Context, ruleType string) ([]CustomRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	var query string
	var args []interface{}

	if ruleType != "" {
		query = `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules WHERE type = ? ORDER BY created_at DESC`
		args = append(args, ruleType)
	} else {
		query = `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules ORDER BY created_at DESC`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list custom rules: %w", err)
	}
	defer rows.Close()

	var rules []CustomRule
	for rows.Next() {
		var rule CustomRule
		var enabled int
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Type, &rule.Mode, &rule.Content, &enabled, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan custom rule: %w", err)
		}
		rule.Enabled = enabled != 0
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom rules: %w", err)
	}

	return rules, nil
}

// GetCustomRule returns a custom rule by ID.
func (r *TrafficRepository) GetCustomRule(ctx context.Context, id int64) (*CustomRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return nil, errors.New("custom rule id is required")
	}

	const query = `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules WHERE id = ?`

	var rule CustomRule
	var enabled int
	err := r.db.QueryRowContext(ctx, query, id).Scan(&rule.ID, &rule.Name, &rule.Type, &rule.Mode, &rule.Content, &enabled, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCustomRuleNotFound
		}
		return nil, fmt.Errorf("get custom rule: %w", err)
	}

	rule.Enabled = enabled != 0
	return &rule, nil
}

// CreateCustomRule creates a new custom rule.
func (r *TrafficRepository) CreateCustomRule(ctx context.Context, rule *CustomRule) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if rule == nil {
		return errors.New("custom rule is required")
	}

	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		return errors.New("custom rule name is required")
	}

	rule.Type = strings.TrimSpace(rule.Type)
	if rule.Type != "dns" && rule.Type != "rules" && rule.Type != "rule-providers" {
		return errors.New("custom rule type must be 'dns', 'rules', or 'rule-providers'")
	}

	rule.Mode = strings.TrimSpace(rule.Mode)
	if rule.Type == "dns" {
		rule.Mode = "replace"
	} else if rule.Type == "rules" {
		// rules type supports replace, prepend, and append
		if rule.Mode != "replace" && rule.Mode != "prepend" && rule.Mode != "append" {
			return errors.New("custom rule mode must be 'replace', 'prepend', or 'append' for rules type")
		}
	} else if rule.Mode != "replace" && rule.Mode != "prepend" {
		return errors.New("custom rule mode must be 'replace' or 'prepend'")
	}

	rule.Content = strings.TrimSpace(rule.Content)
	if rule.Content == "" {
		return errors.New("custom rule content is required")
	}

	const stmt = `INSERT INTO custom_rules (name, type, mode, content, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`

	enabled := 0
	if rule.Enabled {
		enabled = 1
	}

	result, err := r.db.ExecContext(ctx, stmt, rule.Name, rule.Type, rule.Mode, rule.Content, enabled)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return errors.New("custom rule with this name and type already exists")
		}
		return fmt.Errorf("create custom rule: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	rule.ID = id
	return nil
}

// UpdateCustomRule updates an existing custom rule.
func (r *TrafficRepository) UpdateCustomRule(ctx context.Context, rule *CustomRule) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if rule == nil {
		return errors.New("custom rule is required")
	}

	if rule.ID <= 0 {
		return errors.New("custom rule id is required")
	}

	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		return errors.New("custom rule name is required")
	}

	rule.Type = strings.TrimSpace(rule.Type)
	if rule.Type != "dns" && rule.Type != "rules" && rule.Type != "rule-providers" {
		return errors.New("custom rule type must be 'dns', 'rules', or 'rule-providers'")
	}

	rule.Mode = strings.TrimSpace(rule.Mode)
	if rule.Type == "dns" {
		rule.Mode = "replace"
	} else if rule.Type == "rules" {
		// rules type supports replace, prepend, and append
		if rule.Mode != "replace" && rule.Mode != "prepend" && rule.Mode != "append" {
			return errors.New("custom rule mode must be 'replace', 'prepend', or 'append' for rules type")
		}
	} else if rule.Mode != "replace" && rule.Mode != "prepend" {
		return errors.New("custom rule mode must be 'replace' or 'prepend'")
	}

	rule.Content = strings.TrimSpace(rule.Content)
	if rule.Content == "" {
		return errors.New("custom rule content is required")
	}

	const stmt = `UPDATE custom_rules SET name = ?, type = ?, mode = ?, content = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	enabled := 0
	if rule.Enabled {
		enabled = 1
	}

	result, err := r.db.ExecContext(ctx, stmt, rule.Name, rule.Type, rule.Mode, rule.Content, enabled, rule.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return errors.New("custom rule with this name and type already exists")
		}
		return fmt.Errorf("update custom rule: %w", err)
	}

	rows2, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rows2 == 0 {
		return ErrCustomRuleNotFound
	}

	return nil
}

// DeleteCustomRule deletes a custom rule by ID.
func (r *TrafficRepository) DeleteCustomRule(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return errors.New("custom rule id is required")
	}

	const stmt = `DELETE FROM custom_rules WHERE id = ?`
	result, err := r.db.ExecContext(ctx, stmt, id)
	if err != nil {
		return fmt.Errorf("delete custom rule: %w", err)
	}

	rows3, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rows3 == 0 {
		return ErrCustomRuleNotFound
	}

	return nil
}

// ListEnabledCustomRules returns all enabled custom rules, optionally filtered by type.
func (r *TrafficRepository) ListEnabledCustomRules(ctx context.Context, ruleType string) ([]CustomRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	var query string
	var args []interface{}

	if ruleType != "" {
		query = `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules WHERE type = ? AND enabled = 1 ORDER BY created_at DESC`
		args = append(args, ruleType)
	} else {
		query = `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules WHERE enabled = 1 ORDER BY created_at DESC`
	}

	rows4, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list enabled custom rules: %w", err)
	}
	defer rows4.Close()

	var rules []CustomRule
	for rows4.Next() {
		var rule CustomRule
		var enabled int
		if err := rows4.Scan(&rule.ID, &rule.Name, &rule.Type, &rule.Mode, &rule.Content, &enabled, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan custom rule: %w", err)
		}
		rule.Enabled = enabled != 0
		rules = append(rules, rule)
	}

	if err := rows4.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom rules: %w", err)
	}

	return rules, nil
}

// GetCustomRuleApplications retrieves all custom rule applications for a subscribe file.
func (r *TrafficRepository) GetCustomRuleApplications(ctx context.Context, fileID int64) ([]CustomRuleApplication, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	if fileID <= 0 {
		return nil, errors.New("subscribe file id is required")
	}

	const query = `SELECT id, subscribe_file_id, custom_rule_id, rule_type, rule_mode, applied_content, content_hash, applied_at
		FROM custom_rule_applications
		WHERE subscribe_file_id = ?
		ORDER BY applied_at DESC`

	rows, err := r.db.QueryContext(ctx, query, fileID)
	if err != nil {
		return nil, fmt.Errorf("get custom rule applications: %w", err)
	}
	defer rows.Close()

	var applications []CustomRuleApplication
	for rows.Next() {
		var app CustomRuleApplication
		if err := rows.Scan(&app.ID, &app.SubscribeFileID, &app.CustomRuleID, &app.RuleType, &app.RuleMode, &app.AppliedContent, &app.ContentHash, &app.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan custom rule application: %w", err)
		}
		applications = append(applications, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom rule applications: %w", err)
	}

	return applications, nil
}

// UpsertCustomRuleApplication inserts or updates a custom rule application record.
func (r *TrafficRepository) UpsertCustomRuleApplication(ctx context.Context, app *CustomRuleApplication) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if app.SubscribeFileID <= 0 {
		return errors.New("subscribe file id is required")
	}
	if app.CustomRuleID <= 0 {
		return errors.New("custom rule id is required")
	}
	if app.RuleType == "" {
		return errors.New("rule type is required")
	}

	const stmt = `INSERT INTO custom_rule_applications (subscribe_file_id, custom_rule_id, rule_type, rule_mode, applied_content, content_hash, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(subscribe_file_id, custom_rule_id, rule_type)
		DO UPDATE SET
			rule_mode = excluded.rule_mode,
			applied_content = excluded.applied_content,
			content_hash = excluded.content_hash,
			applied_at = CURRENT_TIMESTAMP`

	_, err := r.db.ExecContext(ctx, stmt, app.SubscribeFileID, app.CustomRuleID, app.RuleType, app.RuleMode, app.AppliedContent, app.ContentHash)
	if err != nil {
		return fmt.Errorf("upsert custom rule application: %w", err)
	}

	return nil
}

// DeleteCustomRuleApplication deletes a custom rule application record.
func (r *TrafficRepository) DeleteCustomRuleApplication(ctx context.Context, fileID, ruleID int64, ruleType string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if fileID <= 0 {
		return errors.New("subscribe file id is required")
	}
	if ruleID <= 0 {
		return errors.New("custom rule id is required")
	}
	if ruleType == "" {
		return errors.New("rule type is required")
	}

	const stmt = `DELETE FROM custom_rule_applications WHERE subscribe_file_id = ? AND custom_rule_id = ? AND rule_type = ?`
	_, err := r.db.ExecContext(ctx, stmt, fileID, ruleID, ruleType)
	if err != nil {
		return fmt.Errorf("delete custom rule application: %w", err)
	}

	return nil
}

// IsSyncTrafficEnabled checks if sync_traffic is enabled in any user_settings (system-level setting).
func (r *TrafficRepository) IsSyncTrafficEnabled(ctx context.Context) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("traffic repository not initialized")
	}

	const query = `SELECT COUNT(*) FROM user_settings WHERE sync_traffic = 1 LIMIT 1`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check sync traffic setting: %w", err)
	}

	return count > 0, nil
}

// ListAllExternalSubscriptions returns all external subscriptions from all users.
func (r *TrafficRepository) ListAllExternalSubscriptions(ctx context.Context) ([]ExternalSubscription, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	const stmt = `SELECT id, username, name, url, COALESCE(user_agent, 'clash-meta/2.4.0'), node_count, last_sync_at, COALESCE(upload, 0), COALESCE(download, 0), COALESCE(total, 0), expire, COALESCE(traffic_mode, 'both'), created_at, updated_at FROM external_subscriptions ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("list all external subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []ExternalSubscription
	for rows.Next() {
		var sub ExternalSubscription
		var lastSyncAt sql.NullTime
		var expire sql.NullTime
		if err := rows.Scan(&sub.ID, &sub.Username, &sub.Name, &sub.URL, &sub.UserAgent, &sub.NodeCount, &lastSyncAt, &sub.Upload, &sub.Download, &sub.Total, &expire, &sub.TrafficMode, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan external subscription: %w", err)
		}
		if lastSyncAt.Valid {
			sub.LastSyncAt = &lastSyncAt.Time
		}
		if expire.Valid {
			sub.Expire = &expire.Time
		}
		subs = append(subs, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate external subscriptions: %w", err)
	}

	return subs, nil
}

// GetSubscribeFilesWithAutoSync returns all subscribe files that have auto-sync enabled.
func (r *TrafficRepository) GetSubscribeFilesWithAutoSync(ctx context.Context) ([]SubscribeFile, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	const query = `SELECT id, name, COALESCE(description, ''), url, type, filename, COALESCE(file_short_code, ''), auto_sync_custom_rules, expire_at, created_at, updated_at
		FROM subscribe_files
		WHERE auto_sync_custom_rules = 1
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get subscribe files with auto sync: %w", err)
	}
	defer rows.Close()

	var files []SubscribeFile
	for rows.Next() {
		var file SubscribeFile
		var autoSync int
		var expireAt sql.NullTime
		if err := rows.Scan(&file.ID, &file.Name, &file.Description, &file.URL, &file.Type, &file.Filename, &file.FileShortCode, &autoSync, &expireAt, &file.CreatedAt, &file.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscribe file: %w", err)
		}
		file.AutoSyncCustomRules = autoSync != 0
		if expireAt.Valid {
			file.ExpireAt = &expireAt.Time
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscribe files: %w", err)
	}

	return files, nil
}

// ==================== ProxyProviderConfig CRUD ====================

// CreateProxyProviderConfig creates a new proxy provider config
func (r *TrafficRepository) CreateProxyProviderConfig(ctx context.Context, config *ProxyProviderConfig) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}

	healthCheckEnabled := 0
	if config.HealthCheckEnabled {
		healthCheckEnabled = 1
	}
	healthCheckLazy := 0
	if config.HealthCheckLazy {
		healthCheckLazy = 1
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO proxy_provider_configs (
			username, external_subscription_id, name, type, interval, proxy, size_limit, header,
			health_check_enabled, health_check_url, health_check_interval, health_check_timeout,
			health_check_lazy, health_check_expected_status,
			filter, exclude_filter, exclude_type, geo_ip_filter, override, process_mode
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		config.Username, config.ExternalSubscriptionID, config.Name, config.Type,
		config.Interval, config.Proxy, config.SizeLimit, config.Header,
		healthCheckEnabled, config.HealthCheckURL, config.HealthCheckInterval, config.HealthCheckTimeout,
		healthCheckLazy, config.HealthCheckExpectedStatus,
		config.Filter, config.ExcludeFilter, config.ExcludeType, config.GeoIPFilter, config.Override, config.ProcessMode,
	)
	if err != nil {
		return 0, fmt.Errorf("create proxy provider config: %w", err)
	}

	return result.LastInsertId()
}

// GetProxyProviderConfig retrieves a proxy provider config by ID
func (r *TrafficRepository) GetProxyProviderConfig(ctx context.Context, id int64) (*ProxyProviderConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, username, external_subscription_id, name, type, interval, proxy, size_limit,
			COALESCE(header, ''), health_check_enabled, health_check_url, health_check_interval,
			health_check_timeout, health_check_lazy, health_check_expected_status,
			COALESCE(filter, ''), COALESCE(exclude_filter, ''), COALESCE(exclude_type, ''),
			COALESCE(geo_ip_filter, ''), COALESCE(override, ''), process_mode, created_at, updated_at
		FROM proxy_provider_configs WHERE id = ?
	`, id)

	var config ProxyProviderConfig
	var healthCheckEnabled, healthCheckLazy int
	err := row.Scan(
		&config.ID, &config.Username, &config.ExternalSubscriptionID, &config.Name, &config.Type,
		&config.Interval, &config.Proxy, &config.SizeLimit, &config.Header,
		&healthCheckEnabled, &config.HealthCheckURL, &config.HealthCheckInterval,
		&config.HealthCheckTimeout, &healthCheckLazy, &config.HealthCheckExpectedStatus,
		&config.Filter, &config.ExcludeFilter, &config.ExcludeType,
		&config.GeoIPFilter, &config.Override, &config.ProcessMode, &config.CreatedAt, &config.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get proxy provider config: %w", err)
	}

	config.HealthCheckEnabled = healthCheckEnabled != 0
	config.HealthCheckLazy = healthCheckLazy != 0

	return &config, nil
}

// GetProxyProviderConfigByName retrieves a proxy provider config by name
func (r *TrafficRepository) GetProxyProviderConfigByName(ctx context.Context, name string) (*ProxyProviderConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, username, external_subscription_id, name, type, interval, proxy, size_limit,
			COALESCE(header, ''), health_check_enabled, health_check_url, health_check_interval,
			health_check_timeout, health_check_lazy, health_check_expected_status,
			COALESCE(filter, ''), COALESCE(exclude_filter, ''), COALESCE(exclude_type, ''),
			COALESCE(geo_ip_filter, ''), COALESCE(override, ''), process_mode, created_at, updated_at
		FROM proxy_provider_configs WHERE name = ?
	`, name)

	var config ProxyProviderConfig
	var healthCheckEnabled, healthCheckLazy int
	err := row.Scan(
		&config.ID, &config.Username, &config.ExternalSubscriptionID, &config.Name, &config.Type,
		&config.Interval, &config.Proxy, &config.SizeLimit, &config.Header,
		&healthCheckEnabled, &config.HealthCheckURL, &config.HealthCheckInterval,
		&config.HealthCheckTimeout, &healthCheckLazy, &config.HealthCheckExpectedStatus,
		&config.Filter, &config.ExcludeFilter, &config.ExcludeType,
		&config.GeoIPFilter, &config.Override, &config.ProcessMode, &config.CreatedAt, &config.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get proxy provider config by name: %w", err)
	}

	config.HealthCheckEnabled = healthCheckEnabled != 0
	config.HealthCheckLazy = healthCheckLazy != 0

	return &config, nil
}

// ListProxyProviderConfigs returns all proxy provider configs for a user
func (r *TrafficRepository) ListProxyProviderConfigs(ctx context.Context, username string) ([]ProxyProviderConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, external_subscription_id, name, type, interval, proxy, size_limit,
			COALESCE(header, ''), health_check_enabled, health_check_url, health_check_interval,
			health_check_timeout, health_check_lazy, health_check_expected_status,
			COALESCE(filter, ''), COALESCE(exclude_filter, ''), COALESCE(exclude_type, ''),
			COALESCE(geo_ip_filter, ''), COALESCE(override, ''), process_mode, created_at, updated_at
		FROM proxy_provider_configs WHERE username = ? ORDER BY id ASC
	`, username)
	if err != nil {
		return nil, fmt.Errorf("list proxy provider configs: %w", err)
	}
	defer rows.Close()

	var configs []ProxyProviderConfig
	for rows.Next() {
		var config ProxyProviderConfig
		var healthCheckEnabled, healthCheckLazy int
		err := rows.Scan(
			&config.ID, &config.Username, &config.ExternalSubscriptionID, &config.Name, &config.Type,
			&config.Interval, &config.Proxy, &config.SizeLimit, &config.Header,
			&healthCheckEnabled, &config.HealthCheckURL, &config.HealthCheckInterval,
			&config.HealthCheckTimeout, &healthCheckLazy, &config.HealthCheckExpectedStatus,
			&config.Filter, &config.ExcludeFilter, &config.ExcludeType,
			&config.GeoIPFilter, &config.Override, &config.ProcessMode, &config.CreatedAt, &config.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan proxy provider config: %w", err)
		}
		config.HealthCheckEnabled = healthCheckEnabled != 0
		config.HealthCheckLazy = healthCheckLazy != 0
		configs = append(configs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proxy provider configs: %w", err)
	}

	return configs, nil
}

// ListProxyProviderConfigsBySubscription returns all proxy provider configs for an external subscription
func (r *TrafficRepository) ListProxyProviderConfigsBySubscription(ctx context.Context, externalSubscriptionID int64) ([]ProxyProviderConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, external_subscription_id, name, type, interval, proxy, size_limit,
			COALESCE(header, ''), health_check_enabled, health_check_url, health_check_interval,
			health_check_timeout, health_check_lazy, health_check_expected_status,
			COALESCE(filter, ''), COALESCE(exclude_filter, ''), COALESCE(exclude_type, ''),
			COALESCE(geo_ip_filter, ''), COALESCE(override, ''), process_mode, created_at, updated_at
		FROM proxy_provider_configs WHERE external_subscription_id = ? ORDER BY id ASC
	`, externalSubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list proxy provider configs by subscription: %w", err)
	}
	defer rows.Close()

	var configs []ProxyProviderConfig
	for rows.Next() {
		var config ProxyProviderConfig
		var healthCheckEnabled, healthCheckLazy int
		err := rows.Scan(
			&config.ID, &config.Username, &config.ExternalSubscriptionID, &config.Name, &config.Type,
			&config.Interval, &config.Proxy, &config.SizeLimit, &config.Header,
			&healthCheckEnabled, &config.HealthCheckURL, &config.HealthCheckInterval,
			&config.HealthCheckTimeout, &healthCheckLazy, &config.HealthCheckExpectedStatus,
			&config.Filter, &config.ExcludeFilter, &config.ExcludeType,
			&config.GeoIPFilter, &config.Override, &config.ProcessMode, &config.CreatedAt, &config.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan proxy provider config: %w", err)
		}
		config.HealthCheckEnabled = healthCheckEnabled != 0
		config.HealthCheckLazy = healthCheckLazy != 0
		configs = append(configs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proxy provider configs: %w", err)
	}

	return configs, nil
}

// ListMMWProxyProviderConfigs 返回所有妙妙屋模式的代理集合配置
// 该方法用于定时同步器获取需要自动刷新的代理集合列表
func (r *TrafficRepository) ListMMWProxyProviderConfigs(ctx context.Context) ([]ProxyProviderConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, external_subscription_id, name, type, interval, proxy, size_limit,
			COALESCE(header, ''), health_check_enabled, health_check_url, health_check_interval,
			health_check_timeout, health_check_lazy, health_check_expected_status,
			COALESCE(filter, ''), COALESCE(exclude_filter, ''), COALESCE(exclude_type, ''),
			COALESCE(geo_ip_filter, ''), COALESCE(override, ''), process_mode, created_at, updated_at
		FROM proxy_provider_configs
		WHERE process_mode = 'mmw'
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list mmw proxy provider configs: %w", err)
	}
	defer rows.Close()

	var configs []ProxyProviderConfig
	for rows.Next() {
		var config ProxyProviderConfig
		var healthCheckEnabled, healthCheckLazy int
		err := rows.Scan(
			&config.ID, &config.Username, &config.ExternalSubscriptionID, &config.Name, &config.Type,
			&config.Interval, &config.Proxy, &config.SizeLimit, &config.Header,
			&healthCheckEnabled, &config.HealthCheckURL, &config.HealthCheckInterval,
			&config.HealthCheckTimeout, &healthCheckLazy, &config.HealthCheckExpectedStatus,
			&config.Filter, &config.ExcludeFilter, &config.ExcludeType,
			&config.GeoIPFilter, &config.Override, &config.ProcessMode, &config.CreatedAt, &config.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan mmw proxy provider config: %w", err)
		}
		config.HealthCheckEnabled = healthCheckEnabled != 0
		config.HealthCheckLazy = healthCheckLazy != 0
		configs = append(configs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mmw proxy provider configs: %w", err)
	}

	return configs, nil
}

// UpdateProxyProviderConfig updates an existing proxy provider config
func (r *TrafficRepository) UpdateProxyProviderConfig(ctx context.Context, config *ProxyProviderConfig) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	healthCheckEnabled := 0
	if config.HealthCheckEnabled {
		healthCheckEnabled = 1
	}
	healthCheckLazy := 0
	if config.HealthCheckLazy {
		healthCheckLazy = 1
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE proxy_provider_configs SET
			name = ?, type = ?, interval = ?, proxy = ?, size_limit = ?, header = ?,
			health_check_enabled = ?, health_check_url = ?, health_check_interval = ?,
			health_check_timeout = ?, health_check_lazy = ?, health_check_expected_status = ?,
			filter = ?, exclude_filter = ?, exclude_type = ?, geo_ip_filter = ?, override = ?, process_mode = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND username = ?
	`,
		config.Name, config.Type, config.Interval, config.Proxy, config.SizeLimit, config.Header,
		healthCheckEnabled, config.HealthCheckURL, config.HealthCheckInterval,
		config.HealthCheckTimeout, healthCheckLazy, config.HealthCheckExpectedStatus,
		config.Filter, config.ExcludeFilter, config.ExcludeType, config.GeoIPFilter, config.Override, config.ProcessMode,
		config.ID, config.Username,
	)
	if err != nil {
		return fmt.Errorf("update proxy provider config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return errors.New("proxy provider config not found or not owned by user")
	}

	return nil
}

// DeleteProxyProviderConfig deletes a proxy provider config
func (r *TrafficRepository) DeleteProxyProviderConfig(ctx context.Context, id int64, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	result, err := r.db.ExecContext(ctx, `DELETE FROM proxy_provider_configs WHERE id = ? AND username = ?`, id, username)
	if err != nil {
		return fmt.Errorf("delete proxy provider config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return errors.New("proxy provider config not found or not owned by user")
	}

	return nil
}


// GetSystemConfig retrieves the global system configuration.
// Returns an empty SystemConfig if the row doesn't exist (should not happen after migration).
func (r *TrafficRepository) GetSystemConfig(ctx context.Context) (SystemConfig, error) {
	const query = `
SELECT proxy_groups_source_url, client_compatibility_mode
FROM system_config
WHERE id = 1
`

	var cfg SystemConfig
	var compatibilityMode int
	err := r.db.QueryRowContext(ctx, query).Scan(&cfg.ProxyGroupsSourceURL, &compatibilityMode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return empty config if row doesn't exist (defensive)
			return SystemConfig{}, nil
		}
		return SystemConfig{}, fmt.Errorf("query system config: %w", err)
	}

	cfg.ClientCompatibilityMode = compatibilityMode != 0
	return cfg, nil
}

// UpdateSystemConfig updates the global system configuration.
// Creates the singleton row if it doesn't exist (defensive).
func (r *TrafficRepository) UpdateSystemConfig(ctx context.Context, cfg SystemConfig) error {
	const updateStmt = `
UPDATE system_config
SET proxy_groups_source_url = ?,
    client_compatibility_mode = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = 1
`

	compatibilityMode := 0
	if cfg.ClientCompatibilityMode {
		compatibilityMode = 1
	}

	result, err := r.db.ExecContext(ctx, updateStmt, cfg.ProxyGroupsSourceURL, compatibilityMode)
	if err != nil {
		return fmt.Errorf("update system config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	// If no rows were updated, insert the singleton row (defensive fallback)
	if rowsAffected == 0 {
		const insertStmt = `
INSERT INTO system_config (id, proxy_groups_source_url, client_compatibility_mode)
VALUES (1, ?, ?)
`
		if _, err := r.db.ExecContext(ctx, insertStmt, cfg.ProxyGroupsSourceURL, compatibilityMode); err != nil {
			return fmt.Errorf("insert system config: %w", err)
		}
	}

	return nil
}
