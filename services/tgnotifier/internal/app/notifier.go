package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/faringet/telegram-bot-scraper/internal/newsfmt"
	"github.com/faringet/telegram-bot-scraper/services/tgnotifier/internal/botapi"
	"github.com/faringet/telegram-bot-scraper/services/tgnotifier/internal/storage"
)

type Notifier struct {
	log *slog.Logger

	store storage.Store
	bot   *botapi.Client
	cfg   NotifierConfig
	fmt   *newsfmt.Formatter
}

type NotifierConfig struct {
	SupervisorChatID int64
	BatchSize        int
	MinDelay         time.Duration
	DryRun           bool
	MaxTextRunes     int
}

type NotifierDeps struct {
	Log   *slog.Logger
	Store storage.Store
	Bot   *botapi.Client
	Cfg   NotifierConfig
}

func NewNotifier(d NotifierDeps) *Notifier {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}

	cfg := d.Cfg
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxTextRunes <= 0 {
		cfg.MaxTextRunes = 300
	}

	log = log.With(
		slog.String("layer", "worker"),
		slog.String("module", "notifier.worker"),
	)

	return &Notifier{
		log:   log,
		store: d.Store,
		bot:   d.Bot,
		cfg:   cfg,
		fmt:   newsfmt.NewFormatter(cfg.MaxTextRunes),
	}
}

func (n *Notifier) ProcessWindow(ctx context.Context, reason string, cutoff time.Time) (Result, error) {
	total := Result{}
	attempted := make(map[int64]struct{})

	for {
		fetchLimit := n.cfg.BatchSize + len(attempted)
		if fetchLimit < n.cfg.BatchSize {
			fetchLimit = n.cfg.BatchSize
		}

		hits, err := n.store.ListUndeliveredBefore(ctx, fetchLimit, cutoff)
		if err != nil {
			return total, fmt.Errorf("list undelivered before: %w", err)
		}

		batch := takeFreshHits(hits, attempted, n.cfg.BatchSize)
		if len(batch) == 0 {
			return total, nil
		}

		n.log.Info("deliver batch start",
			slog.String("reason", reason),
			slog.Time("cutoff", cutoff.UTC()),
			slog.Int("count", len(batch)),
		)

		deliveredIDs := make([]int64, 0, len(batch))
		for _, h := range batch {
			attempted[h.ID] = struct{}{}
			total.Total++

			msg := n.fmt.HitMessage(hitToView(h))

			_, sendErr := n.bot.SendText(
				ctx,
				n.cfg.SupervisorChatID,
				msg,
				"HTML",
				true,
			)
			if sendErr != nil {
				n.log.Error("send failed",
					slog.String("reason", reason),
					slog.Int64("hit_id", h.ID),
					slog.String("channel", h.Channel),
					slog.Int64("message_id", h.MessageID),
					slog.Any("err", sendErr),
				)
				continue
			}

			total.Sent++
			deliveredIDs = append(deliveredIDs, h.ID)

			if err := botapi.SleepCtx(ctx, n.cfg.MinDelay); err != nil {
				n.log.Warn("interrupted by ctx",
					slog.String("reason", reason),
					slog.Any("err", err),
				)
				break
			}
		}

		if n.cfg.DryRun {
			continue
		}

		if len(deliveredIDs) == 0 {
			// Защита от вечной карусели на битых строках
			// Что уже пытались отправить в этом окне повторно не гоняем
			continue
		}

		if err := n.store.MarkDelivered(ctx, deliveredIDs); err != nil {
			return total, fmt.Errorf("mark delivered: %w", err)
		}
		total.Marked += len(deliveredIDs)
	}
}

func takeFreshHits(hits []storage.Hit, attempted map[int64]struct{}, limit int) []storage.Hit {
	if limit <= 0 {
		limit = len(hits)
	}

	out := make([]storage.Hit, 0, limit)
	for _, h := range hits {
		if _, seen := attempted[h.ID]; seen {
			continue
		}
		out = append(out, h)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func hitToView(h storage.Hit) newsfmt.HitView {
	var reason string
	if h.LLMReason.Valid {
		reason = h.LLMReason.String
	}

	var category string
	if h.Category.Valid {
		category = h.Category.String
	}

	var confidence *float64
	if h.LLMConfidence.Valid {
		v := h.LLMConfidence.Float64
		confidence = &v
	}

	var messageDate time.Time
	if !h.MessageDate.IsZero() {
		messageDate = h.MessageDate.UTC()
	}

	return newsfmt.HitView{
		ID:          h.ID,
		Channel:     h.Channel,
		MessageID:   h.MessageID,
		MessageDate: messageDate,
		Text:        h.Text,
		Link:        h.Link,
		Keyword:     h.Keyword,
		Category:    category,
		Reason:      reason,
		Confidence:  confidence,
	}
}
