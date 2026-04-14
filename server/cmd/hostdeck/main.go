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

	dbTarget := cfg.DBPath
	if cfg.DBDriver == storage.DriverPostgres {
		dbTarget = cfg.DBDSN
	}

	db, err := storage.Open(ctx, cfg.DBDriver, dbTarget)
	if err != nil {
		slog.Error("open database failed", "driver", cfg.DBDriver, "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := storage.MigrateWithDriver(ctx, db, cfg.DBDriver); err != nil {
		slog.Error("migrate database failed", "driver", cfg.DBDriver, "error", err)
		os.Exit(1)
	}

	serverRepo := storage.NewServerRepository(db, cfg.MasterKey)
	credentialRepo := storage.NewServerCredentialRepository(db)
	userRepo := storage.NewUserRepository(db)
	sessionRepo := storage.NewSessionRepository(db)
	authEventRepo := storage.NewAuthEventRepository(db)
	statusRepo := storage.NewStatusRepository(db)
	commandLogRepo := storage.NewCommandLogRepository(db)
	alertRepo := storage.NewAlertRepository(db)
	sshClient := sshx.NewClient()
	connectionService := service.NewServerConnectionService(serverRepo, credentialRepo, cfg.MasterKey)
	sshCollector := collector.NewSSHCollector(sshClient)
	poller := scheduler.NewPoller(serverRepo, connectionService, sshCollector, statusRepo, time.Duration(cfg.PollIntervalSeconds)*time.Second, cfg.PollConcurrency)
	authService := service.NewAuthService(userRepo, sessionRepo, authEventRepo, time.Duration(cfg.SessionTTLHours)*time.Hour)
	if err := authService.EnsureBootstrapAdmin(ctx, cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		slog.Error("bootstrap admin failed", "error", err)
		os.Exit(1)
	}
	alertService := service.NewAlertService(alertRepo, serverRepo, statusRepo)
	shellService := service.NewShellService(alertService, commandLogRepo, authEventRepo)
	serverViewService := service.NewServerViewService(serverRepo, statusRepo)
	overviewService := service.NewOverviewService(serverRepo, statusRepo, alertService)
	commandService := service.NewCommandService(connectionService, sshClient, commandLogRepo)
	probeHandler := api.NewProbeHandler(connectionService, sshClient, statusRepo)
	apiRouter := httpx.NewRouterWithHandlers(
		api.NewServerHandler(serverRepo, serverViewService),
		probeHandler,
		api.NewOverviewHandler(overviewService),
		api.NewServerDetailHandler(overviewService),
		api.NewCommandHandler(commandService),
		api.NewAlertHandler(alertService),
		httpx.WithAuthHandler(api.NewAuthHandler(authService, cfg.SessionCookieName, cfg.SessionCookieSecure, cfg.BootstrapAdminToken)),
		httpx.WithShellHandler(api.NewShellHandler(shellService)),
		httpx.WithAPIMiddleware(httpx.NewSessionAuthMiddleware(authService, cfg.SessionCookieName)),
		httpx.WithActionGuard(httpx.RequireInfrastructureAccess),
	)
	handler := httpx.WithRequestLogging(httpx.WithStaticFallback(apiRouter, cfg.WebDistDir))

	go poller.Run(ctx)

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
		"dbDriver", cfg.DBDriver,
		"webDistDir", cfg.WebDistDir,
		"sessionCookieName", cfg.SessionCookieName,
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}
