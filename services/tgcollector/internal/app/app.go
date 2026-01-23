package app

import (
	"context"
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
	log = log.With(slog.String("component", "app"))

	client, err := mtclient.New(cfg.MTProto, log)
	if err != nil {
		return nil, err
	}

	store, err := storage.NewSQLite(storage.SQLiteConfig{
		//todo вынести в конфиг
		Path:           "data/scraper.db",
		DedupWindow:    cfg.Scrape.DedupWindow,
		BusyTimeout:    5 * time.Second,
		JournalModeWAL: true,
	})
	if err != nil {
		return nil, err
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

func (a *App) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

func (a *App) RunDaemon(ctx context.Context) error {
	a.log.Info("run started (daemon mode)")

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

//func (a *App) crawlOnce(ctx context.Context) error {
//	return a.client.WithClient(ctx, func(ctx context.Context, td *telegram.Client) error {
//		return a.scraper.Crawl(ctx, td)
//	})
//}
