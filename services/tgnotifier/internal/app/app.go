package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	platformpg "github.com/faringet/telegram-bot-scraper/internal/platform/postgres"
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

	rootLog := log
	appLog := log.With(
		slog.String("layer", "app"),
		slog.String("module", "notifier.app"),
	)

	st, err := openStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	b, err := botapi.New(cfg.TelegramBot, rootLog)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("create bot client: %w", err)
	}

	n := NewNotifier(NotifierDeps{
		Log:   rootLog,
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
		log:      appLog,
		store:    st,
		bot:      b,
		notifier: n,
	}, nil
}

func openStore(cfg *tgcfg.TGNotifier) (storage.Store, error) {
	if cfg == nil {
		return nil, errors.New("notifier app: config is nil")
	}

	if cfg.Storage.Driver != "postgres" {
		return nil, fmt.Errorf("unsupported storage driver for tgnotifier: %s", cfg.Storage.Driver)
	}

	db, err := platformpg.Open(platformpg.Config{
		DSN:             cfg.Storage.Postgres.DSN,
		MaxOpenConns:    cfg.Storage.Postgres.MaxOpenConns,
		MaxIdleConns:    cfg.Storage.Postgres.MaxIdleConns,
		ConnMaxLifetime: cfg.Storage.Postgres.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Storage.Postgres.ConnMaxIdleTime,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres db: %w", err)
	}

	st, err := storage.NewPostgres(db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create postgres storage: %w", err)
	}
	return st, nil
}

func (a *App) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

func (a *App) Run(ctx context.Context) error {
	catchUpOnStart := a.cfg.Notifier.Schedule.CatchUpEnabled()

	loc, hour, minute, err := parseNotifierSchedule(
		a.cfg.Notifier.Schedule.Timezone,
		a.cfg.Notifier.Schedule.DailyAt,
	)
	if err != nil {
		return err
	}

	a.log.Info("run started",
		slog.String("storage_driver", a.cfg.Storage.Driver),
		slog.Bool("dry_run", a.cfg.Notifier.DryRun),
		slog.String("timezone", loc.String()),
		slog.String("daily_at", a.cfg.Notifier.Schedule.DailyAt),
		slog.Bool("catch_up_on_start", catchUpOnStart),
		slog.Int("batch_size", a.cfg.Notifier.BatchSize),
		slog.Duration("min_delay", a.cfg.Notifier.MinDelay),
		slog.Int64("supervisor_chat_id", a.cfg.Notifier.SupervisorChatID),
		slog.Int("max_open_conns", a.cfg.Storage.Postgres.MaxOpenConns),
		slog.Int("max_idle_conns", a.cfg.Storage.Postgres.MaxIdleConns),
	)

	if err := a.bot.Ping(ctx); err != nil {
		a.log.Error("bot ping failed", slog.Any("err", err))
		return err
	}

	now := time.Now().In(loc)
	if catchUpOnStart {
		cutoff := scheduledTimeForDate(now, hour, minute)
		if !now.Before(cutoff) {
			a.runWindow(ctx, "catchup_on_start", cutoff)
		}
	}

	for {
		next := nextScheduledTime(time.Now().In(loc), hour, minute)
		wait := time.Until(next)

		a.log.Info("next scheduled delivery",
			slog.Time("next_run", next),
			slog.Duration("wait", wait),
		)

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			a.log.Info("shutdown", slog.Any("err", ctx.Err()))
			return ctx.Err()
		case <-timer.C:
			a.runWindow(ctx, "scheduled", next)
		}
	}
}

func (a *App) runWindow(ctx context.Context, reason string, cutoff time.Time) {
	start := time.Now()

	a.log.Info("notify window start",
		slog.String("reason", reason),
		slog.Time("cutoff", cutoff.UTC()),
	)

	res, err := a.notifier.ProcessWindow(ctx, reason, cutoff)
	if err != nil {
		a.log.Error("notify window failed",
			slog.String("reason", reason),
			slog.Time("cutoff", cutoff.UTC()),
			slog.Duration("duration", time.Since(start)),
			slog.Any("err", err),
		)
		return
	}

	a.log.Info("notify window done",
		slog.String("reason", reason),
		slog.Time("cutoff", cutoff.UTC()),
		slog.Int("total", res.Total),
		slog.Int("sent", res.Sent),
		slog.Int("marked", res.Marked),
		slog.Duration("duration", time.Since(start)),
	)
}

func parseNotifierSchedule(timezone string, dailyAt string) (*time.Location, int, int, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("load notifier timezone: %w", err)
	}

	tm, err := time.Parse("15:04", dailyAt)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("parse notifier daily_at: %w", err)
	}

	return loc, tm.Hour(), tm.Minute(), nil
}

func scheduledTimeForDate(t time.Time, hour, minute int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), hour, minute, 0, 0, t.Location())
}

func nextScheduledTime(now time.Time, hour, minute int) time.Time {
	today := scheduledTimeForDate(now, hour, minute)
	if now.Before(today) {
		return today
	}
	tomorrow := now.AddDate(0, 0, 1)
	return scheduledTimeForDate(tomorrow, hour, minute)
}
