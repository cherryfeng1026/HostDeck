package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hostdeck/server/internal/api"
	"hostdeck/server/internal/collector"
	"hostdeck/server/internal/config"
	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/httpx"
	"hostdeck/server/internal/scheduler"
	"hostdeck/server/internal/service"
	"hostdeck/server/internal/sshx"
	"hostdeck/server/internal/storage"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	db, err := storage.Open(ctx, cfg.DBDSN)
	if err != nil {
		slog.Error("open database failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := storage.Migrate(ctx, db); err != nil {
		slog.Error("migrate database failed", "error", err)
		os.Exit(1)
	}

	serverRepo := storage.NewServerRepository(db, cfg.MasterKey)
	credentialRepo := storage.NewServerCredentialRepository(db)
	userRepo := storage.NewUserRepository(db)
	sessionRepo := storage.NewSessionRepository(db)
	authEventRepo := storage.NewAuthEventRepository(db)
	auditEventRepo := storage.NewAuditEventRepository(db)
	statusRepo := storage.NewStatusRepository(db)
	commandLogRepo := storage.NewCommandLogRepository(db)
	alertRepo := storage.NewAlertRepository(db, cfg.MasterKey)
	sshClient := sshx.NewClient()
	connectionService := service.NewServerConnectionService(serverRepo, credentialRepo, cfg.MasterKey)
	sshCollector := collector.NewSSHCollector(sshClient)
	apiTokenRepo := storage.NewAPITokenRepository(db)
	authService := service.NewAuthService(userRepo, sessionRepo, apiTokenRepo, authEventRepo, time.Duration(cfg.SessionTTLHours)*time.Hour)
	if err := authService.EnsureBootstrapAdmin(ctx, cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		slog.Error("bootstrap admin failed", "error", err)
		os.Exit(1)
	}
	alertNotifier := service.NewDynamicWebhookAlertNotifier(alertRepo)
	alertService := service.NewAlertService(alertRepo, alertRepo, serverRepo, alertRepo, alertNotifier)
	currentNotificationSettings, err := alertService.GetNotificationSettings(ctx)
	if err != nil {
		slog.Error("load alert notification settings failed", "error", err)
		os.Exit(1)
	}
	if currentNotificationSettings.CreatedAt.IsZero() && cfg.AlertWebhookURL != "" {
		if _, err := alertService.SaveNotificationSettings(ctx, domain.AlertNotificationSettings{
			Enabled:               true,
			WebhookURL:            cfg.AlertWebhookURL,
			WebhookTimeoutSeconds: cfg.AlertWebhookTimeoutSeconds,
		}); err != nil {
			slog.Error("seed alert notification settings failed", "error", err)
			os.Exit(1)
		}
	}
	poller := scheduler.NewPoller(serverRepo, connectionService, sshCollector, statusRepo, alertService, time.Duration(cfg.PollIntervalSeconds)*time.Second, cfg.PollConcurrency)
	cleanupRunner := scheduler.NewCleanupRunner(
		statusRepo,
		commandLogRepo,
		alertRepo,
		authEventRepo,
		auditEventRepo,
		authService,
		time.Duration(cfg.CleanupIntervalSeconds)*time.Second,
		scheduler.CleanupSchedule{
			StatusHistoryRetention: time.Duration(cfg.StatusHistoryRetentionHours) * time.Hour,
			CommandLogRetention:    time.Duration(cfg.CommandLogRetentionHours) * time.Hour,
			AlertHistoryRetention:  time.Duration(cfg.AlertHistoryRetentionHours) * time.Hour,
			AuthEventRetention:     time.Duration(cfg.AuthEventRetentionHours) * time.Hour,
			AuditEventRetention:    time.Duration(cfg.AuditEventRetentionHours) * time.Hour,
			APITokenRetention:      time.Duration(cfg.APITokenRetentionHours) * time.Hour,
		},
	)
	shellService := service.NewShellService(alertService, commandLogRepo, authEventRepo, auditEventRepo, userRepo)
	serverViewService := service.NewServerViewService(serverRepo, statusRepo)
	overviewService := service.NewOverviewService(serverRepo, statusRepo, alertService)
	commandService := service.NewCommandService(connectionService, sshClient, commandLogRepo)
	probeHandler := api.NewProbeHandler(connectionService, sshClient, statusRepo, alertService)
	apiRouter := httpx.NewRouterWithHandlers(
		api.NewServerHandler(serverRepo, serverViewService, auditEventRepo),
		probeHandler,
		api.NewOverviewHandler(overviewService),
		api.NewServerDetailHandler(overviewService),
		api.NewCommandHandler(commandService, auditEventRepo),
		api.NewAlertHandler(alertService, auditEventRepo),
		httpx.WithAuthHandler(api.NewAuthHandler(authService, cfg.SessionCookieName, cfg.SessionCookieSecure, cfg.BootstrapAdminToken)),
		httpx.WithShellHandler(api.NewShellHandler(shellService)),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, cfg.SessionCookieName)),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)
	handler := httpx.WithRequestLogging(httpx.WithStaticFallback(apiRouter, cfg.WebDistDir))

	go poller.Run(ctx)
	go cleanupRunner.Run(ctx)

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: handler,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info(
		"starting server",
		"addr", cfg.HTTPAddr,
		"webDistDir", cfg.WebDistDir,
		"sessionCookieName", cfg.SessionCookieName,
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}
