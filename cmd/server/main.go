package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/captcha"
	"miaomiaowu/internal/handler"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/patches"
	"miaomiaowu/internal/proxygroups"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/taskrun"
	"miaomiaowu/internal/version"
	"miaomiaowu/internal/web"
	ruletemplates "miaomiaowu/rule_templates"
	"miaomiaowu/subscribes"
)

func main() {
	// 初始化logger
	logger.Init()
	logger.Info("妙妙屋服务器启动中", "version", version.Version)

	// 启动日志清理任务（每天凌晨3点清理7天前的日志）
	go startLogCleanup()

	addr := getAddr()

	repo, err := storage.NewTrafficRepository(filepath.Join("data", "traffic.db"))
	if err != nil {
		logger.Error("流量数据库初始化失败", "error", err)
		os.Exit(1)
	}
	defer repo.Close()

	authManager, err := auth.NewManager(repo)
	if err != nil {
		logger.Error("认证管理器加载失败", "error", err)
		os.Exit(1)
	}

	tokenStore := auth.NewTokenStore(24 * time.Hour)
	twoFactorStore := auth.NewTwoFactorPendingStore(5 * time.Minute)

	// Load persisted sessions from database
	ctx := context.Background()
	sessions, err := repo.LoadSessions(ctx)
	if err != nil {
		logger.Warn("从数据库加载会话失败", "error", err)
	} else {
		for _, session := range sessions {
			tokenStore.LoadSession(session.Token, session.Username, session.ExpiresAt)
		}
		logger.Info("会话加载完成", "count", len(sessions))
	}

	// Cleanup expired sessions from database
	if err := repo.CleanupExpiredSessions(ctx); err != nil {
		logger.Warn("清理过期会话失败", "error", err)
	}

	subscribeDir := filepath.Join("subscribes")
	if err := subscribes.Ensure(subscribeDir); err != nil {
		logger.Error("订阅文件准备失败", "error", err)
		os.Exit(1)
	}

	ruleTemplatesDir := filepath.Join("rule_templates")
	if err := ruletemplates.Ensure(ruleTemplatesDir); err != nil {
		logger.Error("规则模板文件准备失败", "error", err)
		os.Exit(1)
	}

	// rule_templates 补丁:Ensure 不覆盖已存在文件(保护用户自定义),
	// 但对历史已知错误的 dns 块(语义比对,顺序无关)做一次精准替换。详见 internal/patches 包注释。
	if patched, err := patches.ApplyDNSPatches(ruleTemplatesDir); err != nil {
		logger.Warn("DNS 模板补丁应用过程出错(不影响启动)", "error", err)
	} else if patched > 0 {
		logger.Info("DNS 模板补丁已应用", "count", patched)
	}

	// 初始化代理组配置 Store（纯内存存储）
	// 优先从系统配置的远程地址拉取，失败时使用空配置
	var proxyGroupsStore *proxygroups.Store

	// 获取系统配置中的远程地址
	systemConfig, err := repo.GetSystemConfig(ctx)
	if err != nil {
		logger.Warn("加载系统配置失败", "error", err)
	}

	// 从远程拉取配置
	data, resolvedURL, fetchErr := proxygroups.FetchConfig(systemConfig.ProxyGroupsSourceURL)
	if fetchErr != nil {
		logger.Warn("拉取代理组配置失败", "error", fetchErr)
		// 远程拉取失败时使用空配置初始化
		proxyGroupsStore, err = proxygroups.NewStore([]byte("[]"), "empty-fallback")
		if err != nil {
			logger.Error("创建代理组存储失败", "error", err)
			os.Exit(1)
		}
		logger.Info("代理组存储已使用空配置初始化", "reason", "远程拉取失败")
	} else {
		// 远程拉取成功
		proxyGroupsStore, err = proxygroups.NewStore(data, resolvedURL)
		if err != nil {
			logger.Error("代理组配置无效", "source", resolvedURL, "error", err)
			os.Exit(1)
		}
		logger.Info("代理组配置加载成功", "source", resolvedURL)
	}

	syncSubscribeFilesToDatabase(repo, subscribeDir)

	// 初始化通知模块
	sysCfg, _ := repo.GetSystemConfig(context.Background())
	handler.SetBlockUnknownSubscriptionUA(sysCfg.BlockUnknownSubUA)
	handler.InitNotifier(notify.Config{
		Enabled:                sysCfg.NotifyEnabled,
		BotToken:               sysCfg.TelegramBotToken,
		ChatID:                 sysCfg.TelegramChatID,
		NotifySubscribeFetch:   sysCfg.NotifySubscribeFetch,
		NotifyLogin:            sysCfg.NotifyLogin,
		NotifyIPBan:            sysCfg.NotifyIPBan,
		NotifySilentMode:       sysCfg.NotifySilentMode,
		NotifyDailyTraffic:     sysCfg.NotifyDailyTraffic,
		NotifyExpiry:           sysCfg.NotifyExpiry,
		NotifyNodeProbeOffline: sysCfg.NotifyNodeProbeOffline,
		NotifyNodeProbeOnline:  sysCfg.NotifyNodeProbeOnline,
		DailyTrafficTime:       sysCfg.NotifyDailyTrafficTime,
	})

	// 启动时初始化代理集合缓存
	go handler.InitProxyProviderCacheOnStartup(repo)

	// 启动代理集合定时同步器
	proxySyncCtx, stopProxySync := context.WithCancel(context.Background())
	go handler.StartProxyProviderCacheSync(proxySyncCtx, repo)

	trafficHandler := handler.NewTrafficSummaryHandler(repo)
	userRepo := auth.NewRepositoryAdapter(repo)
	loginRateLimiter := handler.NewLoginRateLimiterWithConfig(sysCfg.LoginRateMaxAttempts, sysCfg.LoginRateWindow, sysCfg.LoginRateLockDuration)
	loginRateLimiter.SetSkipLocalIP(sysCfg.SkipLocalIP)

	mux := http.NewServeMux()
	mux.Handle("/api/setup/status", handler.NewSetupStatusHandler(repo))
	mux.Handle("/api/setup/init", handler.NewInitialSetupHandler(repo))
	mux.Handle("/api/setup/restore-backup", handler.NewSetupRestoreBackupHandler(repo))
	turnstileVerifier := captcha.New(repo)
	mux.HandleFunc("/api/captcha/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": turnstileVerifier.Enabled(r.Context()), "site_key": turnstileVerifier.SiteKey(r.Context())})
	})
	mux.Handle("/api/login", handler.NewLoginHandler(authManager, tokenStore, repo, loginRateLimiter, twoFactorStore, turnstileVerifier))
	mux.Handle("/api/login/2fa", handler.NewTwoFactorLoginHandler(tokenStore, repo, twoFactorStore))
	mux.Handle("/api/login/recovery", handler.NewRecoveryLoginHandler(tokenStore, repo, twoFactorStore))

	// Admin-only endpoints
	mux.Handle("/api/admin/credentials", auth.RequireAdmin(tokenStore, userRepo, handler.NewCredentialsHandler(authManager, tokenStore)))
	mux.Handle("/api/admin/users", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserListHandler(repo)))
	mux.Handle("/api/admin/users/create", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserCreateHandler(repo)))
	mux.Handle("/api/admin/users/delete", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserDeleteHandler(repo)))
	mux.Handle("/api/admin/users/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserStatusHandler(repo)))
	mux.Handle("/api/admin/users/reset-password", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserResetPasswordHandler(repo)))
	mux.Handle("/api/admin/users/remark", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserRemarkHandler(repo)))
	mux.Handle("/api/admin/users/custom-short-code", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserCustomShortCodeHandler(repo)))
	mux.Handle("/api/admin/users/", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserSubscriptionsHandler(repo)))
	securityLogHandler := handler.NewSecurityLogHandler(repo)
	mux.Handle("/api/admin/security/", auth.RequireAdmin(tokenStore, userRepo, securityLogHandler))
	mux.Handle("/api/admin/security/turnstile", auth.RequireAdmin(tokenStore, userRepo, handler.NewTurnstileSettingsHandler(repo)))
	mux.Handle("/api/admin/tasks/", auth.RequireAdmin(tokenStore, userRepo, handler.NewTaskLogHandler(repo)))
	mux.Handle("/api/admin/operations", auth.RequireAdmin(tokenStore, userRepo, handler.NewOperationLogHandler(repo)))
	mux.Handle("/api/admin/subscriptions", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionAdminHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/subscriptions/", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionAdminHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/subscribe-files", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscribeFilesHandler(repo)))
	mux.Handle("/api/admin/subscribe-files/", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscribeFilesHandler(repo)))
	mux.Handle("/api/admin/probe-config", auth.RequireAdmin(tokenStore, userRepo, handler.NewProbeConfigHandler(repo)))
	mux.Handle("/api/admin/probe-sync", auth.RequireAdmin(tokenStore, userRepo, handler.NewProbeSyncHandler(repo)))
	mux.Handle("/api/admin/rules/", auth.RequireAdmin(tokenStore, userRepo, http.StripPrefix("/api/admin/rules/", handler.NewRuleEditorHandler(subscribeDir, repo))))
	mux.Handle("/api/admin/rule-templates", auth.RequireToken(tokenStore, handler.NewRuleTemplatesHandler(repo)))
	mux.Handle("/api/admin/rule-templates/", auth.RequireToken(tokenStore, handler.NewRuleTemplatesHandler(repo)))
	mux.Handle("/api/user/default-template", auth.RequireToken(tokenStore, handler.NewUserDefaultTemplateHandler(repo)))
	mux.Handle("/api/admin/template-v3/", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateV3Handler(repo)))
	mux.Handle("/api/admin/nodes", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodesHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/nodes/", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodesHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/sync-external-subscriptions", auth.RequireAdmin(tokenStore, userRepo, handler.NewSyncExternalSubscriptionsHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/sync-external-subscription", auth.RequireAdmin(tokenStore, userRepo, handler.NewSyncSingleExternalSubscriptionHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/sync-external-subscriptions/confirm", auth.RequireAdmin(tokenStore, userRepo, handler.NewConfirmExternalSyncHandler(repo)))
	mux.Handle("/api/admin/rules/latest", auth.RequireAdmin(tokenStore, userRepo, handler.NewRuleMetadataHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/custom-rules", auth.RequireAdmin(tokenStore, userRepo, handler.NewCustomRulesHandler(repo)))
	mux.Handle("/api/admin/custom-rules/", auth.RequireAdmin(tokenStore, userRepo, handler.NewCustomRuleHandler(repo)))
	mux.Handle("/api/admin/apply-custom-rules", auth.RequireAdmin(tokenStore, userRepo, handler.NewApplyCustomRulesHandler(repo)))
	mux.Handle("/api/admin/override-scripts", auth.RequireAdmin(tokenStore, userRepo, handler.NewOverrideScriptsHandler(repo)))
	mux.Handle("/api/admin/override-scripts/", auth.RequireAdmin(tokenStore, userRepo, handler.NewOverrideScriptsHandler(repo)))
	mux.Handle("/api/admin/templates", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplatesHandler(repo)))
	mux.Handle("/api/admin/templates/", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateHandler(repo)))
	mux.Handle("/api/admin/templates/convert", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateConvertHandler()))
	mux.Handle("/api/admin/templates/fetch-source", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateFetchSourceHandler()))
	mux.Handle("/api/admin/backup/download", auth.RequireAdmin(tokenStore, userRepo, handler.NewBackupDownloadHandler(repo)))
	mux.Handle("/api/admin/backup/restore", auth.RequireAdmin(tokenStore, userRepo, handler.NewBackupRestoreHandler(repo)))
	mux.Handle("/api/admin/update/check", auth.RequireAdmin(tokenStore, userRepo, handler.NewUpdateCheckHandler()))
	mux.Handle("/api/admin/update/apply", auth.RequireAdmin(tokenStore, userRepo, handler.NewUpdateApplyHandler()))
	mux.Handle("/api/admin/update/apply-sse", auth.RequireAdmin(tokenStore, userRepo, handler.NewUpdateApplySSEHandler()))
	mux.Handle("/api/admin/proxy-groups/sync", auth.RequireAdmin(tokenStore, userRepo, handler.NewProxyGroupsSyncHandler(repo, proxyGroupsStore)))
	mux.Handle("/api/admin/notify-config", auth.RequireAdmin(tokenStore, userRepo, handler.NewNotifyConfigHandler(repo)))
	mux.Handle("/api/admin/notify-config/", auth.RequireAdmin(tokenStore, userRepo, handler.NewNotifyConfigHandler(repo)))

	// TCPing endpoint (admin only)
	mux.Handle("/api/admin/tcping", auth.RequireAdmin(tokenStore, userRepo, handler.NewTCPingHandler()))
	mux.Handle("/api/admin/tcping/batch", auth.RequireAdmin(tokenStore, userRepo, handler.NewTCPingBatchHandler()))

	// User endpoints (all authenticated users)
	mux.Handle("/api/proxy-groups", auth.RequireToken(tokenStore, handler.NewProxyGroupsHandler(proxyGroupsStore)))
	mux.Handle("/api/user/password", auth.RequireToken(tokenStore, handler.NewPasswordHandler(authManager)))
	mux.Handle("/api/user/profile", auth.RequireToken(tokenStore, handler.NewProfileHandler(repo)))
	mux.Handle("/api/user/settings", auth.RequireToken(tokenStore, handler.NewUserSettingsHandler(repo, tokenStore)))
	mux.Handle("/api/user/config", auth.RequireToken(tokenStore, handler.NewUserConfigHandler(repo)))
	mux.Handle("/api/user/2fa/status", auth.RequireToken(tokenStore, handler.NewTwoFactorStatusHandler(repo)))
	mux.Handle("/api/user/2fa/setup", auth.RequireToken(tokenStore, handler.NewTwoFactorSetupHandler(authManager, repo)))
	mux.Handle("/api/user/2fa/verify-setup", auth.RequireToken(tokenStore, handler.NewTwoFactorVerifySetupHandler(repo)))
	mux.Handle("/api/user/2fa/disable", auth.RequireToken(tokenStore, handler.NewTwoFactorDisableHandler(authManager, repo)))
	mux.Handle("/api/user/token", auth.RequireToken(tokenStore, handler.NewUserTokenHandler(repo)))
	mux.Handle("/api/user/external-subscriptions", auth.RequireToken(tokenStore, handler.NewExternalSubscriptionsHandler(repo)))
	mux.Handle("/api/user/external-subscriptions/nodes", auth.RequireToken(tokenStore, handler.NewExternalSubscriptionNodesHandler(repo)))
	mux.Handle("/api/user/external-subscriptions/check-filter", auth.RequireToken(tokenStore, handler.NewExternalSubscriptionCheckFilterHandler(repo)))
	mux.Handle("/api/user/proxy-provider-configs", auth.RequireToken(tokenStore, handler.NewProxyProviderConfigsHandler(repo)))
	mux.Handle("/api/user/proxy-provider-cache/refresh", auth.RequireToken(tokenStore, handler.NewProxyProviderCacheRefreshHandler(repo)))
	mux.Handle("/api/user/proxy-provider-cache/status", auth.RequireToken(tokenStore, handler.NewProxyProviderCacheStatusHandler(repo)))
	mux.Handle("/api/user/proxy-provider-nodes", auth.RequireToken(tokenStore, handler.NewProxyProviderNodesHandler(repo)))
	mux.Handle("/api/proxy-provider/", handler.NewProxyProviderServeHandler(repo))

	// Debug日志相关endpoint
	mux.Handle("/api/user/debug/", auth.RequireToken(tokenStore, handler.NewDebugHandler(repo)))

	mux.Handle("/api/traffic/summary", auth.RequireToken(tokenStore, trafficHandler))
	mux.Handle("/api/traffic/subscribe", auth.RequireToken(tokenStore, http.HandlerFunc(trafficHandler.HandleSubscribeTraffic)))
	mux.Handle("/api/subscriptions", auth.RequireToken(tokenStore, handler.NewSubscriptionListHandler(repo)))
	mux.Handle("/api/dns/resolve", auth.RequireToken(tokenStore, handler.NewDNSHandler()))
	mux.Handle("/api/subscribe-files", auth.RequireToken(tokenStore, handler.NewSubscribeFilesListHandler(repo)))

	// Create subscription handler (shared between endpoint and short links)
	subscriptionHandler := handler.NewSubscriptionHandlerConcrete(repo, subscribeDir)
	mux.Handle("/api/clash/subscribe", handler.NewSubscriptionEndpoint(tokenStore, repo, subscribeDir))

	// Short link reset endpoint (authenticated)
	mux.Handle("/api/user/short-link", auth.RequireToken(tokenStore, handler.NewShortLinkResetHandler(repo)))
	mux.Handle("/api/user/custom-short-code", auth.RequireToken(tokenStore, handler.NewUserCustomShortCodeSelfHandler(repo)))

	// Speed test endpoints
	speedTesterWS := handler.NewSpeedTesterWSHandler(repo)
	speedTestHandler := handler.NewSpeedTestHandler(repo)
	speedTestHandler.SetTesterWS(speedTesterWS)
	mux.Handle("/api/admin/speedtest/", auth.RequireAdmin(tokenStore, userRepo, speedTestHandler))
	mux.Handle("/api/speedtest/tester/ws", speedTesterWS)

	// 外部节点探测:定时用 mihomo 走完整协议真连一次,测连通性与真实延迟。
	// 与 TCPing 互补 —— 那个只拨 server:port,握手失败/密码错/证书不对一概看不出来。
	// store 必须在这里建好:路由和下面的调度器要共用同一个内存实例。
	nodeProbeStore := handler.NewNodeProbeStore(0)
	mux.Handle("/api/admin/node-probe", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodeProbeHandler(repo, nodeProbeStore)))
	mux.Handle("/api/admin/node-probe/", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodeProbeHandler(repo, nodeProbeStore)))

	// Temporary subscription endpoints
	mux.Handle("/api/admin/temp-subscription", auth.RequireAdmin(tokenStore, userRepo, handler.NewTempSubscriptionHandler()))
	tempSubAccessHandler := handler.NewTempSubscriptionAccessHandler()

	// Combined handler for short links and web app
	// 短链接默认为 3 + 3, 订阅code+用户code, 自定义最小为1+1, 不限制长度
	// /t/{id} paths route to temporary subscription handler
	// All other paths go to the web handler
	shortLinkHandler := handler.NewShortLinkHandler(repo, subscriptionHandler)
	bruteForceProtector := handler.NewBruteForceProtectorWithConfig(sysCfg.BruteForceEnabled, sysCfg.BruteForceMaxFailures, sysCfg.BruteForceWindow, sysCfg.BruteForceBlockDuration)
	bruteForceProtector.SetSkipLocalIP(sysCfg.SkipLocalIP)
	bruteForceProtector.SetRepo(repo)
	bruteForceProtector.RestoreFromDB(context.Background())
	subRateLimiter := handler.NewSubscriptionRateLimiter(sysCfg.SubRateLimitMax, time.Duration(sysCfg.SubRateLimitWindow)*time.Minute)
	subRateLimiter.SetSkipLocalIP(sysCfg.SkipLocalIP)
	go subRateLimiter.StartCleanup(context.Background())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Trim(r.URL.Path, "/")
		clientIP := handler.GetClientIP(r)

		isTempSub := strings.HasPrefix(path, "t/") && len(path) == 10

		// 短链探测候选：单段字母数字路径，且不是已知前端 SPA 路由。
		// 仅这类路径才参与暴力探测计数/封禁；SPA 路由(/nodes 等)、静态资源、
		// 临时订阅一律放行，避免正常浏览被误判为订阅探测而把整个 IP 误封。
		isShortLinkProbe := !isTempSub && len(path) >= 2 && isAlphanumeric(path) && !reservedFrontendRoutes[path]

		// 暴力探测封禁检查（仅拦截短链探测类路径，避免误锁正常用户的 UI 访问）
		if isShortLinkProbe && bruteForceProtector.IsBlocked(clientIP, r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		isSubscriptionFetch := isTempSub || isShortLinkProbe
		if isSubscriptionFetch && !subRateLimiter.Allow(clientIP) {
			http.Error(w, "请求过于频繁，请稍后再试", http.StatusTooManyRequests)
			return
		}

		// Check if this is a temporary subscription access (starts with "t/" followed by 8 hex chars)
		if isTempSub {
			rec := &handler.StatusRecorder{ResponseWriter: w, StatusCode: 200}
			tempSubAccessHandler.ServeHTTP(rec, r)
			if rec.StatusCode == http.StatusNotFound || rec.StatusCode == http.StatusForbidden {
				bruteForceProtector.RecordFailure(clientIP, r.URL.Path)
			}
			return
		}
		// 自定义短链接后, 订阅+用户最小为2个字符
		// TryServe does DB lookup; returns false if no match, allowing fallthrough to web
		if isShortLinkProbe {
			if shortLinkHandler.TryServe(w, r) {
				return
			}
			bruteForceProtector.RecordFailure(clientIP, r.URL.Path)
		}
		// Otherwise, pass to web handler
		web.Handler().ServeHTTP(w, r)
	})

	allowedOrigins := getAllowedOrigins()

	// 静默模式中间件
	silentModeManager := handler.NewSilentModeManager(repo, tokenStore)
	handlerWithAudit := handler.OperationAuditMiddleware(mux, repo, tokenStore)
	handlerWithSilentMode := silentModeManager.Middleware(handlerWithAudit)
	handlerWithCORS := withCORS(handlerWithSilentMode, allowedOrigins)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handlerWithCORS,
		ReadHeaderTimeout: 5 * time.Second,
	}

	collectorCtx, stopCollector := context.WithCancel(context.Background())
	taskrun.Init(taskrun.New(repo, map[string]time.Duration{"wal_checkpoint": 5 * time.Minute}))
	go startWALCheckpointTask(collectorCtx, repo)
	go startDatabaseLogCleanup(collectorCtx, repo)
	go bruteForceProtector.StartCleanup(collectorCtx)
	go startTrafficCollector(collectorCtx, trafficHandler)

	notifyCtx, stopNotify := context.WithCancel(context.Background())
	go handler.StartNotifyScheduler(notifyCtx, repo, trafficHandler)

	autoUpdateCtx, stopAutoUpdate := context.WithCancel(context.Background())
	go handler.StartExternalSubscriptionAutoUpdateScheduler(autoUpdateCtx, repo, subscribeDir)

	nodeProbeCtx, stopNodeProbe := context.WithCancel(context.Background())
	handler.NewNodeProbeScheduler(repo, nodeProbeStore, speedTesterWS,
		filepath.Join("data", "node-probe.json"), subscribeDir).Start(nodeProbeCtx)

	go func() {
		logger.Info("HTTP服务器启动", "version", version.Version, "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP服务器运行失败", "error", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(srv, stopCollector, stopProxySync, stopNotify, stopAutoUpdate, stopNodeProbe)
}

func startWALCheckpointTask(ctx context.Context, repo *storage.TrafficRepository) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			taskrun.Record(ctx, "wal_checkpoint", func() (string, error) {
				truncated, remaining, err := repo.CheckpointBestEffort()
				if err != nil {
					logger.Warn("WAL 定时检查点失败", "error", err)
					return "", err
				}
				if !truncated {
					logger.Warn("WAL 暂未截断，已执行被动检查点", "remaining_frames", remaining)
					return fmt.Sprintf("passive checkpoint; %d frames remain", remaining), nil
				}
				return "WAL truncated", nil
			})
		}
	}
}

func startDatabaseLogCleanup(ctx context.Context, repo *storage.TrafficRepository) {
	cleanup := func() {
		now := time.Now()
		securityCount, securityErr := repo.DeleteOldSecurityEvents(ctx, now.AddDate(0, 0, -90))
		operationCount, operationErr := repo.DeleteOldOperationLogs(ctx, now.AddDate(0, 0, -90))
		taskCount, taskErr := repo.DeleteOldTaskRuns(ctx, now.AddDate(0, 0, -30))
		if securityErr != nil || operationErr != nil || taskErr != nil {
			logger.Warn("数据库日志清理部分失败", "security_error", securityErr, "operation_error", operationErr, "task_error", taskErr)
			return
		}
		if securityCount+operationCount+taskCount > 0 {
			logger.Info("数据库日志清理完成", "security", securityCount, "operations", operationCount, "tasks", taskCount)
		}
	}
	cleanup()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

func getAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}

// reservedFrontendRoutes 是前端 SPA 的顶层路由名（单段、纯字母数字的那些，需与
// miaomiaowu/src/routes 保持一致）。这些是合法的前端路由，访问时不能被当作订阅短链探测，
// 否则正常浏览(如 /nodes、/login)会被记为暴力探测失败、最终把整个 IP 误封导致无法访问。
// 含连字符的路由(custom-rules / subscribe-files / system-settings / templates-v3 /
// change-password)本身不是纯字母数字，不会命中探测逻辑，这里无需列出。
var reservedFrontendRoutes = map[string]bool{
	"nodes":        true,
	"login":        true,
	"rules":        true,
	"generator":    true,
	"probe":        true,
	"settings":     true,
	"templates":    true,
	"users":        true,
	"subscription": true,
	"404":          true,
}

// isAlphanumeric checks if a string contains only alphanumeric characters
func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func waitForShutdown(srv *http.Server, cancels ...context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("收到关闭信号，开始优雅关闭")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 停止所有后台任务
	for _, cancelFunc := range cancels {
		if cancelFunc != nil {
			cancelFunc()
		}
	}

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("优雅关闭失败", "error", err)
	} else {
		logger.Info("服务器已安全关闭")
	}
}

func startTrafficCollector(ctx context.Context, trafficHandler *handler.TrafficSummaryHandler) {
	if trafficHandler == nil {
		return
	}

	// 带重试的流量收集函数
	runWithRetry := func() {
		logger.Info("[流量收集器] 开始每日流量收集", "start_time", time.Now().Format("2006-01-02 15:04:05"))

		maxRetries := 3
		retryDelay := 30 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := trafficHandler.RecordDailyUsage(runCtx)
			cancel()

			if err == nil {
				logger.Info("[流量收集器] 每日流量收集成功")
				return
			}

			logger.Warn("[流量收集器] 每日流量收集失败", "attempt", attempt, "max_retries", maxRetries, "error", err)

			// 如果是探针配置未找到错误，不需要重试
			if errors.Is(err, storage.ErrProbeConfigNotFound) {
				logger.Info("[流量收集器] 探针未配置，跳过重试")
				return
			}

			if attempt < maxRetries {
				logger.Info("[流量收集器] 准备重试", "delay", retryDelay)
				select {
				case <-ctx.Done():
					logger.Info("[流量收集器] 重试已取消（服务器关闭）")
					return
				case <-time.After(retryDelay):
					// 继续重试
				}
			}
		}

		logger.Error("[流量收集器] 达到最大重试次数后仍失败", "max_retries", maxRetries)
	}

	runWithRetry()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Info("[流量收集器] 定时调度器已启动", "interval", "24小时")

	for {
		select {
		case <-ctx.Done():
			logger.Info("[流量收集器] 定时调度器已停止")
			return
		case <-ticker.C:
			runWithRetry()
		}
	}
}

// syncSubscribeFilesToDatabase scans the subscribes directory and ensures
// every YAML file has a corresponding record in the subscribe_files table.
// This helps with backward compatibility when upgrading from older versions.
func syncSubscribeFilesToDatabase(repo *storage.TrafficRepository, subscribeDir string) {
	if repo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Read all files from subscribes directory
	entries, err := os.ReadDir(subscribeDir)
	if err != nil {
		logger.Warn("读取订阅目录失败", "dir", subscribeDir, "error", err)
		return
	}

	synced := 0
	for _, entry := range entries {
		// Skip directories and non-YAML files
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if filepath.Ext(filename) != ".yaml" && filepath.Ext(filename) != ".yml" {
			continue
		}

		// Skip the .keep.yaml placeholder file
		if filename == ".keep.yaml" {
			continue
		}

		// Check if this file already has a database record
		if _, err := repo.GetSubscribeFileByFilename(ctx, filename); err == nil {
			// File already exists in database, skip
			continue
		} else if !errors.Is(err, storage.ErrSubscribeFileNotFound) {
			logger.Warn("检查订阅文件失败", "filename", filename, "error", err)
			continue
		}

		// File doesn't exist in database, create a new record
		// Use filename without extension as the name
		name := filename[:len(filename)-len(filepath.Ext(filename))]

		file := storage.SubscribeFile{
			Name:        name,
			Description: "自动同步的订阅文件",
			URL:         "",                          // No URL for legacy files
			Type:        storage.SubscribeTypeUpload, // Mark as upload type
			Filename:    filename,
		}

		if _, err := repo.CreateSubscribeFile(ctx, file); err != nil {
			logger.Warn("同步订阅文件到数据库失败", "filename", filename, "error", err)
			continue
		}

		synced++
	}

	if synced > 0 {
		logger.Info("订阅文件同步完成", "count", synced)
	}
}

// startLogCleanup 启动日志清理任务
func startLogCleanup() {
	logManager := logger.NewLogManager("data/logs")

	// 启动时立即清理一次
	if err := logManager.CleanupOldLogs(); err != nil {
		logger.Error("[日志清理] 启动时清理失败", "error", err)
	}

	// 每天凌晨3点清理
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Info("[日志清理] 定时清理任务已启动", "interval", "24小时", "max_age", "7天")

	for range ticker.C {
		if err := logManager.CleanupOldLogs(); err != nil {
			logger.Error("[日志清理] 定时清理失败", "error", err)
		}
	}
}
