// services/tgbot/internal/app/app.go
package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gotd/td/tg"

	tgbotcfg "github.com/faringet/telegram-bot-scraper/services/tgbot/config"
	"github.com/faringet/telegram-bot-scraper/services/tgbot/internal/mtproto"
)

// App — корневой объект сервиса tgbot.
// Он собирает инфраструктуру и управляет жизненным циклом.
//
// main.go знает ТОЛЬКО про App.
type App struct {
	cfg    *tgbotcfg.TGBot
	log    *slog.Logger
	client *mtproto.Client
}

// New создает приложение целиком (config + mtproto).
func New(cfg *tgbotcfg.TGBot, log *slog.Logger) (*App, error) {
	if cfg == nil {
		return nil, errors.New("app: config is nil")
	}
	if log == nil {
		log = slog.Default()
	}

	client, err := mtproto.New(cfg.MTProto, log)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:    cfg,
		log:    log.With(slog.String("component", "app")),
		client: client,
	}, nil
}

// Run запускает приложение и держит его живым до отмены контекста.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("app run started")

	return a.client.Run(ctx, func(ctx context.Context, api *tg.Client) error {
		// Пока просто живем.
		// Следующий шаг: reader -> matcher -> storage.
		<-ctx.Done()
		return ctx.Err()
	})
}
