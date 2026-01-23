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

type Client struct {
	cfg cfg.MTProto
	log *slog.Logger
	td  *telegram.Client
}

func New(c cfg.MTProto, logg *slog.Logger) (*Client, error) {
	if logg == nil {
		logg = slog.Default()
	}
	logg = logg.With(slog.String("component", "tgcollector.mtproto"))

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

func (c *Client) WithClient(ctx context.Context, fn func(ctx context.Context, td *telegram.Client) error) error {
	if c == nil || c.td == nil {
		return errors.New("mtproto: client is nil")
	}

	return c.td.Run(ctx, func(ctx context.Context) error {
		if err := authorizeIfNeeded(ctx, c.td, c.cfg, c.log); err != nil {
			return err
		}
		return fn(ctx, c.td)
	})
}

func newTelegramClient(c cfg.MTProto, logg *slog.Logger) (*telegram.Client, error) {
	storage := &session.FileStorage{Path: c.Session}

	device := telegram.DeviceConfig{
		DeviceModel:    c.Device.Model,
		SystemVersion:  c.Device.System,
		AppVersion:     c.Device.AppVersion,
		LangCode:       c.Device.LangCode,
		SystemLangCode: c.Device.SystemLang,
	}

	//todo допилить логгер
	td := telegram.NewClient(c.APIID, c.APIHash, telegram.Options{
		SessionStorage: storage,
		Device:         device,
	})

	return td, nil
}
