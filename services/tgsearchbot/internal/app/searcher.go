package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/faringet/telegram-bot-scraper/internal/newsfmt"
	"github.com/faringet/telegram-bot-scraper/internal/platform/searchtext"
	"github.com/faringet/telegram-bot-scraper/services/tgsearchbot/internal/storage"
)

type SearcherConfig struct {
	DefaultLookback time.Duration
	MaxResults      int
	MaxQueryRunes   int
	MaxTextRunes    int
}

type Searcher struct {
	log   *slog.Logger
	store storage.Store
	cfg   SearcherConfig
	fmt   *newsfmt.Formatter
}

func NewSearcher(log *slog.Logger, st storage.Store, cfg SearcherConfig) *Searcher {
	if log == nil {
		log = slog.Default()
	}
	if cfg.DefaultLookback <= 0 {
		cfg.DefaultLookback = 7 * 24 * time.Hour
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 10
	}
	if cfg.MaxQueryRunes <= 0 {
		cfg.MaxQueryRunes = 120
	}
	if cfg.MaxTextRunes <= 0 {
		cfg.MaxTextRunes = 300
	}

	return &Searcher{
		log: log.With(
			slog.String("layer", "worker"),
			slog.String("module", "searchbot.searcher"),
		),
		store: st,
		cfg:   cfg,
		fmt:   newsfmt.NewFormatter(cfg.MaxTextRunes),
	}
}

func (s *Searcher) Search(ctx context.Context, rawQuery string) ([]string, error) {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return nil, errors.New("empty search query")
	}

	rawQuery = truncateRunes(rawQuery, s.cfg.MaxQueryRunes)
	normalizedQuery := searchtext.Normalize(rawQuery)
	if normalizedQuery == "" {
		return nil, errors.New("empty normalized search query")
	}

	since := time.Now().UTC().Add(-s.cfg.DefaultLookback)

	hits, err := s.store.SearchRecent(ctx, normalizedQuery, since, s.cfg.MaxResults)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, s.fmt.HitMessage(newsfmt.HitView{
			ID:          h.ID,
			Channel:     h.Channel,
			MessageID:   h.MessageID,
			MessageDate: h.MessageDate,
			Text:        h.Text,
			Link:        h.Link,
			Keyword:     h.Keyword,
			Category:    h.Category,
			Reason:      h.Reason,
			Confidence:  h.Confidence,
		}))
	}

	return out, nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
