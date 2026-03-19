package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/faringet/telegram-bot-scraper/internal/platform/migrate"
	"github.com/faringet/telegram-bot-scraper/internal/platform/migrations/postgresmigrations"
	"github.com/faringet/telegram-bot-scraper/internal/platform/postgres"
	"github.com/faringet/telegram-bot-scraper/pkg/logger"
)

func main() {
	cfg := NewConfig()
	log := logger.NewLogger(logger.Options{
		AppName: cfg.Base.AppName,
		Env:     cfg.Base.Env,
		Level:   cfg.Logger.Level,
		JSON:    cfg.Logger.JSON,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	db, err := postgres.Open(postgres.Config{
		DSN:             cfg.Storage.Postgres.DSN,
		MaxOpenConns:    cfg.Storage.Postgres.MaxOpenConns,
		MaxIdleConns:    cfg.Storage.Postgres.MaxIdleConns,
		ConnMaxLifetime: cfg.Storage.Postgres.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Storage.Postgres.ConnMaxIdleTime,
	})
	if err != nil {
		log.Error("open postgres failed", slog.Any("err", err))
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("close postgres failed", slog.Any("err", err))
		}
	}()

	waitCtx, cancel := context.WithTimeout(ctx, cfg.Migrator.StartupTimeout)
	defer cancel()

	if err := postgres.WaitReady(waitCtx, db, cfg.Migrator.PingInterval); err != nil {
		log.Error("postgres is not ready", slog.Any("err", err))
		os.Exit(1)
	}

	runner, err := migrate.NewRunner(migrate.RunnerConfig{
		DB:            db,
		FS:            postgresmigrations.FS,
		Dir:           postgresmigrations.Dir,
		LockNamespace: cfg.Migrator.LockNamespace,
		LockResource:  cfg.Migrator.LockResource,
		Log:           log,
	})
	if err != nil {
		log.Error("create migrator runner failed", slog.Any("err", err))
		os.Exit(1)
	}

	if err := runner.Run(ctx); err != nil {
		log.Error("migration run failed", slog.Any("err", err))
		os.Exit(1)
	}

	log.Info("migration run completed")
}
