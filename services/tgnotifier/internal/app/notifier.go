package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/faringet/telegram-bot-scraper/services/tgnotifier/internal/botapi"
	"github.com/faringet/telegram-bot-scraper/services/tgnotifier/internal/storage"
)

type Notifier struct {
	log *slog.Logger

	store storage.Store
	bot   *botapi.Client
	cfg   NotifierConfig
	fmt   *Formatter
}

type NotifierConfig struct {
	SupervisorChatID int64
	BatchSize        int
	MinDelay         time.Duration
	DryRun           bool

	MaxTextRunes int
}

type NotifierDeps struct {
	Log   *slog.Logger
	Store storage.Store
	Bot   *botapi.Client
	Cfg   NotifierConfig
}

func NewNotifier(d NotifierDeps) *Notifier {
	log := d.Log
	log = log.With(slog.String("component", "notifier"))

	return &Notifier{
		log:   log,
		store: d.Store,
		bot:   d.Bot,
		cfg:   d.Cfg,
		fmt:   NewFormatter(d.Cfg.MaxTextRunes),
	}
}

func (n *Notifier) Process(ctx context.Context, reason string) (Result, error) {
	hits, err := n.store.ListUndelivered(ctx, n.cfg.BatchSize)
	if err != nil {
		return Result{}, fmt.Errorf("list undelivered: %w", err)
	}

	res := Result{Total: len(hits)}
	if res.Total == 0 {
		return res, nil
	}

	n.log.Info("deliver batch start",
		slog.String("reason", reason),
		slog.Int("count", res.Total),
	)

	deliveredIDs := make([]int64, 0, len(hits))

	for _, h := range hits {
		msg := n.fmt.HitMessage(h)

		_, sendErr := n.bot.SendText(
			ctx,
			n.cfg.SupervisorChatID,
			msg,
			"",
			true,
		)
		if sendErr != nil {
			n.log.Error("send failed",
				slog.String("reason", reason),
				slog.Int64("hit_id", h.ID),
				slog.String("channel", h.Channel),
				slog.Int("message_id", h.MessageID),
				slog.Any("err", sendErr),
			)
			continue
		}

		res.Sent++
		deliveredIDs = append(deliveredIDs, h.ID)

		if err := botapi.SleepCtx(ctx, n.cfg.MinDelay); err != nil {
			n.log.Warn("interrupted by ctx",
				slog.String("reason", reason),
				slog.Any("err", err),
			)
			break
		}
	}

	if len(deliveredIDs) == 0 {
		return res, nil
	}

	if n.cfg.DryRun {
		return res, nil
	}

	if err := n.store.MarkDelivered(ctx, deliveredIDs); err != nil {
		return res, fmt.Errorf("mark delivered: %w", err)
	}

	res.Marked = len(deliveredIDs)
	return res, nil
}
