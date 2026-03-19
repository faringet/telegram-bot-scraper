package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gotd/td/telegram"

	tgcollector "github.com/faringet/telegram-bot-scraper/services/tgcollector/config"
	mtclient "github.com/faringet/telegram-bot-scraper/services/tgcollector/internal/mtproto"
	"github.com/faringet/telegram-bot-scraper/services/tgcollector/internal/scraper"
	"github.com/faringet/telegram-bot-scraper/services/tgcollector/internal/storage"
)

type App struct {
	cfg *tgcollector.TGCollector
	log *slog.Logger

	client  *mtclient.Client
	store   storage.Store
	scraper *scraper.Scraper
}

func New(cfg *tgcollector.TGCollector, log *slog.Logger) (*App, error) {
	if cfg == nil {
		return nil, errors.New("collector app: config is nil")
	}
	if log == nil {
		return nil, errors.New("collector app: logger is nil")
	}

	log = log.With(slog.String("component", "app"))

	client, err := mtclient.New(cfg.MTProto, log)
	if err != nil {
		return nil, fmt.Errorf("create mtproto client: %w", err)
	}

	store, err := openStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	s := scraper.New(scraper.Config{
		Channels:             cfg.Scrape.Channels,
		Keywords:             cfg.Scrape.Keywords,
		Lookback:             cfg.Scrape.Lookback,
		PerChannelMaxScan:    cfg.Scrape.PerChannelMaxScan,
		MinDelay:             cfg.Scrape.MinDelay,
		BetweenChannelsDelay: cfg.Scrape.BetweenChannelsDelay,
	}, log, store)

	return &App{
		cfg:     cfg,
		log:     log,
		client:  client,
		store:   store,
		scraper: s,
	}, nil
}

func openStore(cfg *tgcollector.TGCollector) (storage.Store, error) {
	switch cfg.Storage.Driver {
	case "sqlite":
		return storage.NewSQLite(storage.SQLiteConfig{
			Path:           cfg.Storage.SQLite.Path,
			DedupWindow:    cfg.Scrape.DedupWindow,
			BusyTimeout:    cfg.Storage.SQLite.BusyTimeout,
			JournalModeWAL: cfg.Storage.SQLite.JournalModeWAL,
		})
	case "postgres":
		return nil, errors.New("collector postgres storage is not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported storage driver: %s", cfg.Storage.Driver)
	}
}

func (a *App) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

func (a *App) Run(ctx context.Context) error {
	a.log.Info("run started (daemon mode)",
		slog.String("storage_driver", a.cfg.Storage.Driver),
		slog.String("sqlite_path", a.cfg.Storage.SQLite.Path),
		slog.Duration("interval", a.cfg.Scrape.Interval),
	)

	interval := a.cfg.Scrape.Interval
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	return a.client.WithClient(ctx, func(ctx context.Context, td *telegram.Client) error {
		if err := a.scraper.Crawl(ctx, td); err != nil {
			a.log.Error("initial crawl failed", slog.Any("err", err))
		}

		t := time.NewTicker(interval)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				a.log.Info("shutdown", slog.Any("err", ctx.Err()))
				return ctx.Err()

			case <-t.C:
				if err := a.scraper.Crawl(ctx, td); err != nil {
					a.log.Error("scheduled crawl failed", slog.Any("err", err))
				}
			}
		}
	})
}
