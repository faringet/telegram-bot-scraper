package config

import (
	"fmt"
	"time"

	pcfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type TGCollector struct {
	Base    pcfg.Base    `mapstructure:",squash"`
	Logger  pcfg.Logger  `mapstructure:"logger"`
	Runtime pcfg.Runtime `mapstructure:"runtime"`
	Storage pcfg.Storage `mapstructure:"storage"`

	MTProto pcfg.MTProto `mapstructure:"mtproto"`
	Scrape  pcfg.Scrape  `mapstructure:"scrape"`
}

func (c *TGCollector) setDefaults() {
	if c.Base.AppName == "" {
		c.Base.AppName = "tg-collector"
	}
	if c.Base.Env == "" {
		c.Base.Env = "dev"
	}

	if c.Logger.Level == "" {
		c.Logger.Level = "info"
	}

	if c.Runtime.ShutdownTimeout == 0 {
		c.Runtime.ShutdownTimeout = 15 * time.Second
	}

	if c.Storage.Driver == "" {
		c.Storage.Driver = "sqlite"
	}
	if c.Storage.SQLite.Path == "" {
		c.Storage.SQLite.Path = "data/scraper.db"
	}
	if c.Storage.SQLite.BusyTimeout <= 0 {
		c.Storage.SQLite.BusyTimeout = 5 * time.Second
	}
	if !c.Storage.SQLite.JournalModeWAL {
		c.Storage.SQLite.JournalModeWAL = true
	}

	if c.MTProto.Session == "" {
		c.MTProto.Session = "data/session.json"
	}
	if c.MTProto.RateLimit.MinDelay <= 0 {
		c.MTProto.RateLimit.MinDelay = 400 * time.Millisecond
	}
	if c.MTProto.RateLimit.Concurrency <= 0 {
		c.MTProto.RateLimit.Concurrency = 1
	}

	if c.Scrape.Lookback <= 0 {
		c.Scrape.Lookback = 7 * 24 * time.Hour
	}
	if c.Scrape.DedupWindow <= 0 {
		c.Scrape.DedupWindow = 30 * 24 * time.Hour
	}
	if c.Scrape.PerChannelMaxScan <= 0 {
		c.Scrape.PerChannelMaxScan = 500
	}
	if c.Scrape.MinDelay <= 0 {
		c.Scrape.MinDelay = c.MTProto.RateLimit.MinDelay
		if c.Scrape.MinDelay <= 0 {
			c.Scrape.MinDelay = 400 * time.Millisecond
		}
	}
	if c.Scrape.BetweenChannelsDelay <= 0 {
		c.Scrape.BetweenChannelsDelay = 2 * time.Second
	}
	if c.Scrape.Interval <= 0 {
		c.Scrape.Interval = 60 * time.Minute
	}
}

func (c *TGCollector) Validate() error {
	c.setDefaults()

	if err := c.Base.Validate(); err != nil {
		return fmt.Errorf("base: %w", err)
	}
	if err := c.Logger.Validate(); err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	if err := c.Runtime.Validate(); err != nil {
		return fmt.Errorf("runtime: %w", err)
	}
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	if err := c.MTProto.Validate(); err != nil {
		return fmt.Errorf("mtproto: %w", err)
	}
	if err := c.Scrape.Validate(); err != nil {
		return fmt.Errorf("scrape: %w", err)
	}

	return nil
}

func New() *TGCollector {
	c := pcfg.MustLoad[TGCollector](pcfg.Options{
		Paths: []string{
			"./services/tgcollector/config",
			"./config",
			"./configs",
		},
		Names:         []string{"config", "tgcollector", "config.local"},
		Type:          "yaml",
		EnvPrefix:     "TGC",
		OptionalFiles: true,
	})

	if err := c.Validate(); err != nil {
		panic(fmt.Errorf("invalid tg-collector config: %w", err))
	}

	return c
}
