package app

import (
	"context"
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
	log = log.With(slog.String("component", "app"))

	st, err := storage.NewSQLite(storage.SQLiteConfig{
		Path:        cfg.Notifier.DBPath,
		BusyTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	b, err := botapi.New(cfg.TelegramBot, log)
	if err != nil {
		_ = st.Close()
		return nil, err
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
			MaxTextRunes:     900,
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

func (a *App) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

func (a *App) RunDaemon(ctx context.Context) error {
	interval := a.cfg.Notifier.Interval
	if interval <= 0 {
		interval = 2 * time.Minute
	}

	a.log.Info("run started (daemon mode)",
		slog.Bool("dry_run", a.cfg.Notifier.DryRun),
		slog.Duration("interval", interval),
		slog.Int("batch_size", a.cfg.Notifier.BatchSize),
		slog.Duration("min_delay", a.cfg.Notifier.MinDelay),
		slog.Int64("supervisor_chat_id", a.cfg.Notifier.SupervisorChatID),
		slog.String("db_path", a.cfg.Notifier.DBPath),
	)

	if err := a.bot.Ping(ctx); err != nil {
		a.log.Error("bot ping failed", slog.Any("err", err))
		return err
	}

	a.runTick(ctx, "startup")

	t := time.NewTicker(interval)
	defer t.Stop()

	tick := 0
	for {
		select {
		case <-ctx.Done():
			a.log.Info("shutdown", slog.Any("err", ctx.Err()))
			return ctx.Err()

		case <-t.C:
			tick++
			a.runTick(ctx, fmt.Sprintf("scheduled#%d", tick))
		}
	}
}

func (a *App) runTick(ctx context.Context, reason string) {
	start := time.Now()

	a.log.Info("notify tick", slog.String("reason", reason))

	res, err := a.notifier.Process(ctx, reason)
	if err != nil {
		a.log.Error("notify tick failed",
			slog.String("reason", reason),
			slog.Duration("duration", time.Since(start)),
			slog.Any("err", err),
		)
		return
	}

	if res.Total == 0 {
		a.log.Info("nothing to deliver",
			slog.String("reason", reason),
			slog.Duration("duration", time.Since(start)),
		)
		return
	}

	if a.cfg.Notifier.DryRun {
		a.log.Warn("dry_run enabled: delivered but NOT marked",
			slog.String("reason", reason),
			slog.Int("sent", res.Sent),
			slog.Int("marked", 0),
			slog.Int("total_in_batch", res.Total),
			slog.Duration("duration", time.Since(start)),
		)
		return
	}

	a.log.Info("deliver batch done",
		slog.String("reason", reason),
		slog.Int("sent", res.Sent),
		slog.Int("marked", res.Marked),
		slog.Int("total_in_batch", res.Total),
		slog.Duration("duration", time.Since(start)),
	)
}
