// services/tgbot/config/config.go
package config

import (
	"errors"
	"fmt"
	"time"

	cfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type TGBot struct {
	AppName string     `mapstructure:"app_name"`
	Env     string     `mapstructure:"env"` // dev/prod
	Logger  cfg.Logger `mapstructure:"logger"`
	Mode    cfg.Mode   `mapstructure:"mode"`

	MTProto cfg.MTProto `mapstructure:"mtproto"`

	Scrape cfg.Scrape `mapstructure:"scrape"`

	// Опционально: если позже захочешь UI-бота через Bot API
	TelegramBot cfg.TelegramBot `mapstructure:"telegram_bot"`
}

func (c *TGBot) Validate() error {
	if c.AppName == "" {
		return errors.New("app_name is required")
	}
	if c.Env == "" {
		c.Env = "dev"
	}

	// Поддерживаем "mtproto по умолчанию"
	if c.Mode.Kind == "" {
		c.Mode.Kind = "mtproto"
	}
	if err := c.Mode.Validate(); err != nil {
		return fmt.Errorf("mode: %w", err)
	}

	// В зависимости от режима включаем нужную валидацию
	switch c.Mode.Kind {
	case "mtproto":
		if err := c.MTProto.Validate(); err != nil {
			return fmt.Errorf("mtproto: %w", err)
		}
	case "botapi":
		if err := c.TelegramBot.Validate(true); err != nil {
			return fmt.Errorf("telegram_bot: %w", err)
		}

	default:
		return fmt.Errorf("unsupported mode.kind=%q", c.Mode.Kind)
	}

	if err := c.Scrape.Validate(); err != nil {
		return fmt.Errorf("scrape: %w", err)
	}

	// Пробрасываем app_name в logger (как у тебя в goshop)
	c.Logger.AppName = c.AppName
	if c.Logger.Level == "" {
		c.Logger.Level = "info"
	}

	return nil
}

func New() *TGBot {
	c := cfg.MustLoad[TGBot](cfg.Options{
		Paths: []string{
			"./services/tgbot/config",
			"./config",
			"./configs",
			"/etc/telegram-bot-scraper",
		},
		Names:         []string{"defaults", "tgbot", "config", "config.local"},
		Type:          "yaml",
		EnvPrefix:     "TG_BOT",
		OptionalFiles: true,
	})

	// ---- дефолты (как в goshop: New() ставит дефолты после загрузки) ----

	if c.AppName == "" {
		c.AppName = "tgbot"
	}
	if c.Env == "" {
		c.Env = "dev"
	}

	if c.Mode.Kind == "" {
		c.Mode.Kind = "mtproto"
	}

	// mtproto defaults
	if c.MTProto.Session == "" {
		c.MTProto.Session = "data/session.json"
	}
	if c.MTProto.RateLimit.MinDelay <= 0 {
		c.MTProto.RateLimit.MinDelay = 400 * time.Millisecond
	}
	if c.MTProto.RateLimit.Concurrency <= 0 {
		c.MTProto.RateLimit.Concurrency = 1
	}

	// scrape defaults
	if c.Scrape.Lookback <= 0 {
		c.Scrape.Lookback = 168 * time.Hour
	}
	if c.Scrape.DedupWindow <= 0 {
		c.Scrape.DedupWindow = 30 * 24 * time.Hour
	}

	// logger defaults
	if c.Logger.Level == "" {
		c.Logger.Level = "info"
	}
	c.Logger.AppName = c.AppName

	if err := c.Validate(); err != nil {
		panic(fmt.Errorf("invalid tgbot config: %w", err))
	}

	return c
}
