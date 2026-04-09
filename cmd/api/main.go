package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hddq/podgist/internal/config"
	apphttp "github.com/hddq/podgist/internal/http"
	"github.com/hddq/podgist/internal/migrations"
	"github.com/hddq/podgist/internal/service"
	"github.com/hddq/podgist/internal/store"
	"github.com/hddq/podgist/internal/webui"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	commandArgs := args
	if len(args) > 0 && (args[0] == "serve" || args[0] == "migrate") {
		command = args[0]
		commandArgs = args[1:]
	}

	switch command {
	case "serve":
		return runServe(commandArgs)
	case "migrate":
		return runMigrate(commandArgs)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func runServe(args []string) error {
	configPath, err := configPathFromArgs(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger := setupLogger(cfg.Logging)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.Database.GetDSN())
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
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
	dashHandlers := apphttp.NewDashboardHandlers(authSvc, st, logger)

	var webFS fs.FS
	if sub, err := fs.Sub(webui.Assets, "dist"); err == nil {
		webFS = sub
	}

	router := apphttp.NewRouter(authSvc, handlers, dashHandlers, cfg.Auth.Realm, logger, webFS)

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
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func runMigrate(args []string) error {
	if len(args) == 0 {
		return errors.New("missing migrate subcommand")
	}
	if args[0] != "up" {
		return fmt.Errorf("unsupported migrate subcommand %q", args[0])
	}

	configPath, err := configPathFromArgs(args[1:])
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger := setupLogger(cfg.Logging)
	ctx := context.Background()
	dir := migrations.Dir("")

	logger.Info("running migrations", "dir", dir)
	if err := migrations.Up(ctx, cfg.Database.GetDSN(), dir); err != nil {
		return err
	}
	logger.Info("migrations completed", "dir", dir)

	return nil
}

func configPathFromArgs(args []string) (string, error) {
	configPath := "/etc/podgist/config.yaml"
	switch len(args) {
	case 0:
		return configPath, nil
	case 1:
		return args[0], nil
	default:
		return "", fmt.Errorf("unexpected arguments: %v", args)
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
