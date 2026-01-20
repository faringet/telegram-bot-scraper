// services/tgbot/cmd/tgbot/main.go
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/faringet/telegram-bot-scraper/pkg/logger"
	tgbotcfg "github.com/faringet/telegram-bot-scraper/services/tgbot/config"
	"github.com/faringet/telegram-bot-scraper/services/tgbot/internal/app"
)

func main() {
	start := time.Now()

	// OS signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Config + Logger
	cfg := tgbotcfg.New()
	log := logger.NewLogger(cfg.Logger)
	slog.SetDefault(log)

	log.Info("starting",
		slog.String("app", cfg.AppName),
		slog.String("env", cfg.Env),
		slog.String("mode", cfg.Mode.Kind),
	)

	application, err := app.New(cfg, log)
	if err != nil {
		log.Error("app init failed", slog.Any("err", err))
		os.Exit(1)
	}

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("stopped with error", slog.Any("err", err))
		os.Exit(1)
	}

	log.Info("stopped", slog.Duration("uptime", time.Since(start)))
}
