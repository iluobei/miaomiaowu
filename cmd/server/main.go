package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/handler"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/web"
	ruletemplates "miaomiaowu/rule_templates"
	"miaomiaowu/subscribes"
)

const version = "0.2.6"

func main() {
	addr := getAddr()

	repo, err := storage.NewTrafficRepository(filepath.Join("data", "traffic.db"))
	if err != nil {
		log.Fatalf("failed to initialize traffic repository: %v", err)
	}
	defer repo.Close()

	authManager, err := auth.NewManager(repo)
	if err != nil {
		log.Fatalf("failed to load auth manager: %v", err)
	}

	tokenStore := auth.NewTokenStore(24 * time.Hour)

	// Load persisted sessions from database
	ctx := context.Background()
	sessions, err := repo.LoadSessions(ctx)
	if err != nil {
		log.Printf("warning: failed to load sessions from database: %v", err)
	} else {
		for _, session := range sessions {
			tokenStore.LoadSession(session.Token, session.Username, session.ExpiresAt)
		}
		log.Printf("loaded %d persisted sessions from database", len(sessions))
	}

	// Cleanup expired sessions from database
	if err := repo.CleanupExpiredSessions(ctx); err != nil {
		log.Printf("warning: failed to cleanup expired sessions: %v", err)
	}

	subscribeDir := filepath.Join("subscribes")
	if err := subscribes.Ensure(subscribeDir); err != nil {
		log.Fatalf("failed to prepare subscription files: %v", err)
	}

	ruleTemplatesDir := filepath.Join("rule_templates")
	if err := ruletemplates.Ensure(ruleTemplatesDir); err != nil {
		log.Fatalf("failed to prepare rule template files: %v", err)
	}

	syncSubscribeFilesToDatabase(repo, subscribeDir)

	trafficHandler := handler.NewTrafficSummaryHandler(repo)
	userRepo := auth.NewRepositoryAdapter(repo)

	mux := http.NewServeMux()
	mux.Handle("/api/setup/status", handler.NewSetupStatusHandler(repo))
	mux.Handle("/api/setup/init", handler.NewInitialSetupHandler(repo))
	mux.Handle("/api/login", handler.NewLoginHandler(authManager, tokenStore, repo))

	// Admin-only endpoints
	mux.Handle("/api/admin/credentials", auth.RequireAdmin(tokenStore, userRepo, handler.NewCredentialsHandler(authManager, tokenStore)))
	mux.Handle("/api/admin/users", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserListHandler(repo)))
	mux.Handle("/api/admin/users/create", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserCreateHandler(repo)))
	mux.Handle("/api/admin/users/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserStatusHandler(repo)))
	mux.Handle("/api/admin/users/reset-password", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserResetPasswordHandler(repo)))
	mux.Handle("/api/admin/users/", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserSubscriptionsHandler(repo)))
	mux.Handle("/api/admin/subscriptions", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionAdminHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/subscriptions/", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionAdminHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/subscribe-files", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscribeFilesHandler(repo)))
	mux.Handle("/api/admin/subscribe-files/", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscribeFilesHandler(repo)))
	mux.Handle("/api/admin/probe-config", auth.RequireAdmin(tokenStore, userRepo, handler.NewProbeConfigHandler(repo)))
	mux.Handle("/api/admin/probe-sync", auth.RequireAdmin(tokenStore, userRepo, handler.NewProbeSyncHandler(repo)))
	mux.Handle("/api/admin/rules/", auth.RequireAdmin(tokenStore, userRepo, http.StripPrefix("/api/admin/rules/", handler.NewRuleEditorHandler(subscribeDir, repo))))
	mux.Handle("/api/admin/rule-templates", auth.RequireAdmin(tokenStore, userRepo, handler.NewRuleTemplatesHandler()))
	mux.Handle("/api/admin/rule-templates/", auth.RequireAdmin(tokenStore, userRepo, handler.NewRuleTemplatesHandler()))
	mux.Handle("/api/admin/nodes", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodesHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/nodes/", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodesHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/sync-external-subscriptions", auth.RequireAdmin(tokenStore, userRepo, handler.NewSyncExternalSubscriptionsHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/sync-external-subscription", auth.RequireAdmin(tokenStore, userRepo, handler.NewSyncSingleExternalSubscriptionHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/rules/latest", auth.RequireAdmin(tokenStore, userRepo, handler.NewRuleMetadataHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/custom-rules", auth.RequireAdmin(tokenStore, userRepo, handler.NewCustomRulesHandler(repo)))
	mux.Handle("/api/admin/custom-rules/", auth.RequireAdmin(tokenStore, userRepo, handler.NewCustomRuleHandler(repo)))
	mux.Handle("/api/admin/apply-custom-rules", auth.RequireAdmin(tokenStore, userRepo, handler.NewApplyCustomRulesHandler(repo)))

	// User endpoints (all authenticated users)
	mux.Handle("/api/user/password", auth.RequireToken(tokenStore, handler.NewPasswordHandler(authManager)))
	mux.Handle("/api/user/profile", auth.RequireToken(tokenStore, handler.NewProfileHandler(repo)))
	mux.Handle("/api/user/settings", auth.RequireToken(tokenStore, handler.NewUserSettingsHandler(repo, tokenStore)))
	mux.Handle("/api/user/config", auth.RequireToken(tokenStore, handler.NewUserConfigHandler(repo)))
	mux.Handle("/api/user/token", auth.RequireToken(tokenStore, handler.NewUserTokenHandler(repo)))
	mux.Handle("/api/user/external-subscriptions", auth.RequireToken(tokenStore, handler.NewExternalSubscriptionsHandler(repo)))
	mux.Handle("/api/traffic/summary", auth.RequireToken(tokenStore, trafficHandler))
	mux.Handle("/api/subscriptions", auth.RequireToken(tokenStore, handler.NewSubscriptionListHandler(repo)))
	mux.Handle("/api/dns/resolve", auth.RequireToken(tokenStore, handler.NewDNSHandler()))
	mux.Handle("/api/subscribe-files", auth.RequireToken(tokenStore, handler.NewSubscribeFilesListHandler(repo)))

	// Create subscription handler (shared between endpoint and short links)
	subscriptionHandler := handler.NewSubscriptionHandlerConcrete(repo, subscribeDir)
	mux.Handle("/api/clash/subscribe", handler.NewSubscriptionEndpoint(tokenStore, repo, subscribeDir))

	// Short link reset endpoint (authenticated)
	mux.Handle("/api/user/short-link", auth.RequireToken(tokenStore, handler.NewShortLinkResetHandler(repo)))

	// Temporary subscription endpoints
	mux.Handle("/api/admin/temp-subscription", auth.RequireAdmin(tokenStore, userRepo, handler.NewTempSubscriptionHandler()))
	tempSubAccessHandler := handler.NewTempSubscriptionAccessHandler()

	// Combined handler for short links and web app
	// This catches any 6-character paths like /AbC123 and routes them to short link handler
	// /t/{id} paths route to temporary subscription handler
	// All other paths go to the web handler
	shortLinkHandler := handler.NewShortLinkHandler(repo, subscriptionHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Trim(r.URL.Path, "/")
		// Check if this is a temporary subscription access (starts with "t/" followed by 8 hex chars)
		if strings.HasPrefix(path, "t/") && len(path) == 10 {
			tempSubAccessHandler.ServeHTTP(w, r)
			return
		}
		// Check if this looks like a short link (exactly 6 characters, alphanumeric)
		if len(path) == 6 && isAlphanumeric(path) {
			shortLinkHandler.ServeHTTP(w, r)
			return
		}
		// Otherwise, pass to web handler
		web.Handler().ServeHTTP(w, r)
	})

	allowedOrigins := getAllowedOrigins()
	handlerWithCORS := withCORS(mux, allowedOrigins)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handlerWithCORS,
		ReadHeaderTimeout: 5 * time.Second,
	}

	collectorCtx, stopCollector := context.WithCancel(context.Background())
	go startTrafficCollector(collectorCtx, trafficHandler)

	go func() {
		log.Printf("miaomiaowu Server v%s - HTTP server listening on %s", version, addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	waitForShutdown(srv, stopCollector)
}

func getAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
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

func waitForShutdown(srv *http.Server, stopCollector context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stopCollector()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func startTrafficCollector(ctx context.Context, trafficHandler *handler.TrafficSummaryHandler) {
	if trafficHandler == nil {
		return
	}

	// 带重试的流量收集函数
	runWithRetry := func() {
		log.Printf("[Traffic Collector] Starting daily traffic collection at %s", time.Now().Format("2006-01-02 15:04:05"))

		maxRetries := 3
		retryDelay := 30 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := trafficHandler.RecordDailyUsage(runCtx)
			cancel()

			if err == nil {
				log.Printf("[Traffic Collector] Daily traffic collection completed successfully")
				return
			}

			log.Printf("[Traffic Collector] Daily traffic collection failed (attempt %d/%d): %v", attempt, maxRetries, err)

			// 如果是探针配置未找到错误，不需要重试
			if errors.Is(err, storage.ErrProbeConfigNotFound) {
				log.Printf("[Traffic Collector] Probe not configured, skipping retries")
				return
			}

			if attempt < maxRetries {
				log.Printf("[Traffic Collector] Retrying in %v...", retryDelay)
				select {
				case <-ctx.Done():
					log.Printf("[Traffic Collector] Retry cancelled due to shutdown")
					return
				case <-time.After(retryDelay):
					// 继续重试
				}
			}
		}

		log.Printf("[Traffic Collector] Daily traffic collection failed after %d attempts", maxRetries)
	}

	runWithRetry()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	log.Printf("[Traffic Collector] Scheduler started, will run every 24 hours")

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Traffic Collector] Scheduler stopped")
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
		log.Printf("warning: failed to read subscribes directory: %v", err)
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
			log.Printf("warning: failed to check subscribe file %s: %v", filename, err)
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
			log.Printf("warning: failed to sync subscribe file %s to database: %v", filename, err)
			continue
		}

		synced++
	}

	if synced > 0 {
		log.Printf("synced %d subscribe file(s) from directory to database", synced)
	}
}
