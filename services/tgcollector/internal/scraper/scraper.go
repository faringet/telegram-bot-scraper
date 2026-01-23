package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"github.com/faringet/telegram-bot-scraper/services/tgcollector/internal/storage"
)

type Config struct {
	Channels []string
	Keywords []string
	Lookback time.Duration

	PerChannelMaxScan    int
	MinDelay             time.Duration
	BetweenChannelsDelay time.Duration
}

type Scraper struct {
	cfg   Config
	log   *slog.Logger
	store storage.Store
}

func New(cfg Config, log *slog.Logger, store storage.Store) *Scraper {
	if log == nil {
		log = slog.Default()
	}
	if cfg.PerChannelMaxScan <= 0 {
		cfg.PerChannelMaxScan = 500
	}
	if cfg.MinDelay <= 0 {
		cfg.MinDelay = 400 * time.Millisecond
	}
	if cfg.BetweenChannelsDelay <= 0 {
		cfg.BetweenChannelsDelay = 2 * time.Second
	}

	return &Scraper{
		cfg:   cfg,
		log:   log.With(slog.String("component", "scraper")),
		store: store,
	}
}

func (s *Scraper) Crawl(ctx context.Context, td *telegram.Client) error {

	keywords := normalizeKeywords(s.cfg.Keywords)
	api := tg.NewClient(td)

	for i, ref := range s.cfg.Channels {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}

		username := normalizeUsername(ref)
		if username == "" {
			return fmt.Errorf("scraper: channel must be @username or t.me link, got %q", ref)
		}

		s.log.Info("scan channel start", slog.Int("i", i), slog.String("channel", "@"+username))

		if err := s.scanChannel(ctx, api, username, keywords); err != nil {
			return err
		}

		if i < len(s.cfg.Channels)-1 {
			if err := sleepCtx(ctx, s.cfg.BetweenChannelsDelay); err != nil {
				return err
			}
		}
	}

	s.log.Info("crawl finished")
	return nil
}
