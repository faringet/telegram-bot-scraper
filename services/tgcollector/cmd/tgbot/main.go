package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/faringet/telegram-bot-scraper/pkg/logger"
	"github.com/faringet/telegram-bot-scraper/services/tgcollector/config"
	"github.com/faringet/telegram-bot-scraper/services/tgcollector/internal/app"
)

func main() {
	cfg := config.New()
	log := logger.NewLogger(cfg.Logger)

	application, err := app.New(cfg, log)
	if err != nil {
		log.Error("app init failed", slog.Any("err", err))
		os.Exit(1)
	}
	defer func() {
		if err := application.Close(); err != nil {
			log.Error("app close failed", slog.Any("err", err))
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := application.RunDaemon(ctx); err != nil && !isShutdownErr(err) {
		log.Error("app run failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func isShutdownErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
