package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hddq/podgist/internal/config"
	apphttp "github.com/hddq/podgist/internal/http"
	"github.com/hddq/podgist/internal/service"
	"github.com/hddq/podgist/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	configPath := "/etc/podgist/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.Logging)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.Database.GetDSN())
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		os.Exit(1)
	}

	st := store.New(pool)

	authSvc := service.NewAuthService(st, cfg.Auth.BcryptCost)
	subsSvc := service.NewSubscriptionService(st)
	epsSvc := service.NewEpisodeService(st, cfg.API.MaxEpisodeActions)
	devsSvc := service.NewDeviceService(st)
	syncSvc := service.NewSyncService(st)
	settingsSvc := service.NewSettingsService(st)
	updatesSvc := service.NewUpdatesService(st)

	handlers := apphttp.NewHandlers(authSvc, subsSvc, epsSvc, devsSvc, syncSvc, settingsSvc, updatesSvc, cfg.API.MaxRequestSize, logger)
	router := apphttp.NewRouter(authSvc, handlers, cfg.Auth.Realm, logger)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	logger.Info("starting server", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(cfg config.LoggingConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
