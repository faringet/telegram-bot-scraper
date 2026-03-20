package classifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	classifierprompt "github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/prompt"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/refdata"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/storage"
)

type OllamaClient interface {
	Classify(ctx context.Context, model string, prompt string) (string, error)
}

type Config struct {
	Interval        time.Duration
	BatchSize       int
	Lease           time.Duration
	WorkerID        string
	Model           string
	MaxTextRunes    int
	MaxRetries      int
	RetryBackoff    time.Duration
	OnlyUndelivered bool
	WhitelistPath   string
	PromptPath      string
}

type Worker struct {
	log    *slog.Logger
	cfg    Config
	store  storage.Store
	ollama OllamaClient

	whitelist []string
}

func NewWorker(log *slog.Logger, cfg Config, st storage.Store, oc OllamaClient) (*Worker, error) {
	if log == nil {
		log = slog.Default()
	}
	if st == nil {
		return nil, errors.New("classifier worker: store is nil")
	}
	if oc == nil {
		return nil, errors.New("classifier worker: ollama client is nil")
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 2 * time.Minute
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = fmt.Sprintf("tgclassifier-%d", time.Now().UnixNano())
	}
	if cfg.Model == "" {
		cfg.Model = "qwen2.5:7b"
	}
	if cfg.MaxTextRunes <= 0 {
		cfg.MaxTextRunes = 1800
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 750 * time.Millisecond
	}

	whitelist, err := refdata.LoadCompanies(cfg.WhitelistPath)
	if err != nil {
		return nil, fmt.Errorf("classifier worker: load whitelist: %w", err)
	}

	baseLog := log.With(
		slog.String("layer", "worker"),
		slog.String("module", "classifier.worker"),
		slog.String("worker_id", cfg.WorkerID),
	)
	baseLog.Info("whitelist loaded", "count", len(whitelist), "path", cfg.WhitelistPath)

	return &Worker{
		log:       baseLog,
		cfg:       cfg,
		store:     st,
		ollama:    oc,
		whitelist: whitelist,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		"interval", w.cfg.Interval.String(),
		"batch_size", w.cfg.BatchSize,
		"lease", w.cfg.Lease.String(),
		"model", w.cfg.Model,
		"max_retries", w.cfg.MaxRetries,
		"only_undelivered", w.cfg.OnlyUndelivered,
	)

	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()

	if err := w.tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
		w.log.Warn("initial tick error", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker stopped", "reason", ctx.Err())
			return ctx.Err()
		case <-t.C:
			if err := w.tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.log.Warn("tick error", "err", err)
			}
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	hits, err := w.store.ClaimUnclassifiedHits(ctx, storage.ClaimOptions{
		Limit:           w.cfg.BatchSize,
		WorkerID:        w.cfg.WorkerID,
		Lease:           w.cfg.Lease,
		OnlyUndelivered: w.cfg.OnlyUndelivered,
	})
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		return nil
	}

	for _, h := range hits {
		if err := w.classifyOne(ctx, h); err != nil {
			if errors.Is(err, storage.ErrClaimLost) {
				w.log.Warn("claim lost while classifying hit",
					"id", h.ID,
					"channel", h.Channel,
					"message_id", h.MessageID,
				)
				continue
			}

			_ = w.store.ReleaseProcessing(ctx, h.ID, w.cfg.WorkerID)
			w.log.Warn("failed to classify hit", "id", h.ID, "err", err)
			continue
		}
	}

	return nil
}

func (w *Worker) classifyOne(ctx context.Context, h storage.Hit) error {
	text := normalizeText(h.Text)
	text = truncateRunes(text, w.cfg.MaxTextRunes)

	companiesFound := refdata.FindCompanies(text, w.whitelist, 5)

	promptText, err := classifierprompt.BuildStrictReasonPrompt(
		w.cfg.PromptPath,
		classifierprompt.StrictReasonInput{
			Keyword:        h.Keyword,
			Text:           text,
			CompaniesFound: companiesFound,
		},
	)
	if err != nil {
		return fmt.Errorf("build prompt: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= w.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			sleep := w.cfg.RetryBackoff * time.Duration(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
		}

		raw, err := w.ollama.Classify(ctx, w.cfg.Model, promptText)
		if err != nil {
			lastErr = fmt.Errorf("ollama classify: %w", err)
			continue
		}

		res, err := parseLLMJSON(raw)
		if err != nil {
			lastErr = fmt.Errorf("parse llm response: %w (raw=%q)", err, safeSnippet(raw, 240))
			continue
		}

		cat := normalizeCategory(res.Category)
		if cat == "" {
			lastErr = fmt.Errorf("invalid category in response: %q", res.Category)
			continue
		}

		reason := res.Reason
		reason = truncateRunes(reason, 140)

		cls := storage.Classification{
			Category:     cat,
			LLMModel:     w.cfg.Model,
			ClassifiedAt: time.Now().UTC(),
			Reason:       &reason,
		}

		if res.Confidence != nil {
			v := *res.Confidence
			if v < 0 {
				v = 0
			}
			if v > 1 {
				v = 1
			}
			cls.Confidence = &v
		} else {
			v := 0.5
			cls.Confidence = &v
		}

		if err := w.store.UpdateClassification(ctx, h.ID, w.cfg.WorkerID, cls); err != nil {
			lastErr = fmt.Errorf("update classification: %w", err)
			if errors.Is(err, storage.ErrClaimLost) {
				return err
			}
			continue
		}

		w.log.Info("classified",
			"id", h.ID,
			"channel", h.Channel,
			"message_id", h.MessageID,
			"keyword", h.Keyword,
			"category", cat,
			"confidence", cls.Confidence,
		)

		return nil
	}

	fallback := "LLM не вернул корректный JSON/умозаключение; требуется перепроверка"
	if lastErr != nil {
		fallback = truncateRunes(fallback+": "+lastErr.Error(), 140)
	}

	v := 0.0
	cls := storage.Classification{
		Category:     "other",
		LLMModel:     w.cfg.Model,
		ClassifiedAt: time.Now().UTC(),
		Confidence:   &v,
		Reason:       &fallback,
	}

	if err := w.store.UpdateClassification(ctx, h.ID, w.cfg.WorkerID, cls); err != nil {
		if errors.Is(err, storage.ErrClaimLost) {
			return err
		}
		w.log.Warn("failed to persist fallback classification",
			"id", h.ID,
			"err", err,
		)
	}

	if lastErr == nil {
		lastErr = errors.New("unknown classification error")
	}
	return lastErr
}
