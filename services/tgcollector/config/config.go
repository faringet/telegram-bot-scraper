package config

import (
	"fmt"
	"time"

	pcfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type TGCollector struct {
	AppName string      `mapstructure:"app_name"`
	Env     string      `mapstructure:"env"`
	Logger  pcfg.Logger `mapstructure:"logger"`

	MTProto pcfg.MTProto `mapstructure:"mtproto"`
	Scrape  pcfg.Scrape  `mapstructure:"scrape"`
}

func (c *TGCollector) Validate() error {
	if c.AppName == "" {
		c.AppName = "tg-collector"
	}
	if c.Env == "" {
		c.Env = "dev"
	}

	if c.Logger.Level == "" {
		c.Logger.Level = "info"
	}
	c.Logger.AppName = c.AppName
	if err := c.Logger.Validate(); err != nil {
		return fmt.Errorf("logger: %w", err)
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
	if err := c.Validate(); err != nil {
		panic(fmt.Errorf("invalid tg-collector config: %w", err))
	}

	return c
}
