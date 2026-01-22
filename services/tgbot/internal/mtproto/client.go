// services/tgbot/internal/mtproto/client.go
package mtproto

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	cfg "github.com/faringet/telegram-bot-scraper/pkg/config"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
)

// Client — транспорт + lifecycle MTProto.
// Никакой auth-логики здесь нет: только создание клиента, Run и выдача tg.Client.
type Client struct {
	cfg cfg.MTProto
	log *slog.Logger
	td  *telegram.Client
}

// New создает MTProto клиента.
// Конфиг должен быть валиден (строгий режим).
func New(c cfg.MTProto, logg *slog.Logger) (*Client, error) {
	if logg == nil {
		logg = slog.Default()
	}
	logg = logg.With(slog.String("component", "tgbot.mtproto"))

	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("mtproto config: %w", err)
	}

	td, err := newTelegramClient(c, logg)
	if err != nil {
		return nil, err
	}

	return &Client{
		cfg: c,
		log: logg,
		td:  td,
	}, nil
}

// Run поднимает соединение, гарантирует авторизацию и передает tg.Client в fn.
// Завершается по ctx.Done().

func (c *Client) WithClient(ctx context.Context, fn func(ctx context.Context, td *telegram.Client) error) error {
	if c == nil || c.td == nil {
		return errors.New("mtproto: client is nil")
	}
	if fn == nil {
		return errors.New("mtproto: fn is nil")
	}

	return c.td.Run(ctx, func(ctx context.Context) error {
		if err := authorizeIfNeeded(ctx, c.td, c.cfg, c.log); err != nil {
			return err
		}
		return fn(ctx, c.td)
	})
}

// -------------------- wiring --------------------

func newTelegramClient(c cfg.MTProto, logg *slog.Logger) (*telegram.Client, error) {
	storage := &session.FileStorage{Path: c.Session}

	device := telegram.DeviceConfig{
		DeviceModel:    c.Device.Model,
		SystemVersion:  c.Device.System,
		AppVersion:     c.Device.AppVersion,
		LangCode:       c.Device.LangCode,
		SystemLangCode: c.Device.SystemLang,
	}

	//stdlog := log.New(
	//	&slogWriter{log: logg.With(slog.String("component", "gotd"))},
	//	"",
	//	0,
	//)

	//todo допилить логгер
	td := telegram.NewClient(c.APIID, c.APIHash, telegram.Options{
		SessionStorage: storage,
		Device:         device,
		//Logger:         stdlog,
	})

	return td, nil
}

type slogWriter struct {
	log *slog.Logger
}

func (w *slogWriter) Write(p []byte) (n int, err error) {
	if w == nil || w.log == nil {
		return len(p), nil
	}
	msg := string(p)
	if msg != "" {
		w.log.Info(msg)
	}
	return len(p), nil
}
