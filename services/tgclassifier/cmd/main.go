package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/faringet/telegram-bot-scraper/pkg/logger"
	cfg "github.com/faringet/telegram-bot-scraper/services/tgclassifier/config"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/app"
)

func main() {
	config := cfg.New()
	log := logger.NewLogger(logger.Options{
		AppName: config.Base.AppName,
		Env:     config.Base.Env,
		Level:   config.Logger.Level,
		JSON:    config.Logger.JSON,
	})

	application, err := app.New(config, log)
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

	if err := application.Run(ctx); err != nil && !isShutdownErr(err) {
		log.Error("app run failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func isShutdownErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
