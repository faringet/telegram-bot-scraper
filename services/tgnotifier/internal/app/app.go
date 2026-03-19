package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	tgcfg "github.com/faringet/telegram-bot-scraper/services/tgnotifier/config"
	"github.com/faringet/telegram-bot-scraper/services/tgnotifier/internal/botapi"
	"github.com/faringet/telegram-bot-scraper/services/tgnotifier/internal/storage"
)

type App struct {
	cfg *tgcfg.TGNotifier
	log *slog.Logger

	store    storage.Store
	bot      *botapi.Client
	notifier *Notifier
}

func New(cfg *tgcfg.TGNotifier, log *slog.Logger) (*App, error) {
	if cfg == nil {
		return nil, errors.New("notifier app: config is nil")
	}
	if log == nil {
		return nil, errors.New("notifier app: logger is nil")
	}

	log = log.With(slog.String("component", "app"))

	st, err := openStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	b, err := botapi.New(cfg.TelegramBot, log)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("create bot client: %w", err)
	}

	n := NewNotifier(NotifierDeps{
		Log:   log,
		Store: st,
		Bot:   b,
		Cfg: NotifierConfig{
			SupervisorChatID: cfg.Notifier.SupervisorChatID,
			BatchSize:        cfg.Notifier.BatchSize,
			MinDelay:         cfg.Notifier.MinDelay,
			DryRun:           cfg.Notifier.DryRun,
			MaxTextRunes:     cfg.Notifier.MaxTextRunes,
		},
	})

	return &App{
		cfg:      cfg,
		log:      log,
		store:    st,
		bot:      b,
		notifier: n,
	}, nil
}

func openStore(cfg *tgcfg.TGNotifier) (storage.Store, error) {
	switch cfg.Storage.Driver {
	case "sqlite":
		return storage.NewSQLite(storage.SQLiteConfig{
			Path:        cfg.Storage.SQLite.Path,
			BusyTimeout: cfg.Storage.SQLite.BusyTimeout,
		})
	case "postgres":
		return nil, errors.New("notifier postgres storage is not implemented yet")
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
	interval := a.cfg.Notifier.Interval
	if interval <= 0 {
		interval = 2 * time.Minute
	}

	a.log.Info("run started (daemon mode)",
		slog.String("storage_driver", a.cfg.Storage.Driver),
		slog.String("sqlite_path", a.cfg.Storage.SQLite.Path),
		slog.Bool("dry_run", a.cfg.Notifier.DryRun),
		slog.Duration("interval", interval),
		slog.Int("batch_size", a.cfg.Notifier.BatchSize),
		slog.Duration("min_delay", a.cfg.Notifier.MinDelay),
		slog.Int64("supervisor_chat_id", a.cfg.Notifier.SupervisorChatID),
	)

	if err := a.bot.Ping(ctx); err != nil {
		a.log.Error("bot ping failed", slog.Any("err", err))
		return err
	}

	a.runTick(ctx, "startup")

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("shutdown", slog.Any("err", ctx.Err()))
			return ctx.Err()

		case <-t.C:
			a.runTick(ctx, "scheduled")
		}
	}
}

func (a *App) runTick(ctx context.Context, reason string) {
	if a == nil || a.notifier == nil {
		a.log.Error("run tick skipped: notifier is nil")
		return
	}

	res, err := a.notifier.Process(ctx, reason)
	if err != nil {
		a.log.Error("tick failed",
			slog.String("reason", reason),
			slog.Any("err", err),
		)
		return
	}

	a.log.Info("tick finished",
		slog.String("reason", reason),
		slog.Int("total", res.Total),
		slog.Int("sent", res.Sent),
		slog.Int("marked", res.Marked),
	)
}
