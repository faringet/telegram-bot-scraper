package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	cfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type TGNotifier struct {
	AppName string     `mapstructure:"app_name"`
	Env     string     `mapstructure:"env"`
	Logger  cfg.Logger `mapstructure:"logger"`

	TelegramBot cfg.TelegramBot `mapstructure:"telegram_bot"`
	Notifier    Notifier        `mapstructure:"notifier"`
}

type Notifier struct {
	DBPath           string        `mapstructure:"db_path"`
	SupervisorChatID int64         `mapstructure:"supervisor_chat_id"`
	Interval         time.Duration `mapstructure:"interval"`
	BatchSize        int           `mapstructure:"batch_size"`
	MinDelay         time.Duration `mapstructure:"min_delay"`
	MaxTextRunes     int           `mapstructure:"max_text_runes"`
	DryRun           bool          `mapstructure:"dry_run"`
}

func (n *Notifier) Validate() error {
	if n == nil {
		return errors.New("notifier config is nil")
	}
	if strings.TrimSpace(n.DBPath) == "" {
		return errors.New("notifier.db_path is required")
	}
	if n.SupervisorChatID == 0 {
		return errors.New("notifier.supervisor_chat_id is required")
	}
	if n.Interval <= 0 {
		return errors.New("notifier.interval must be > 0 (e.g. 2m)")
	}
	if n.BatchSize <= 0 {
		return errors.New("notifier.batch_size must be > 0")
	}
	if n.MinDelay < 0 {
		return errors.New("notifier.min_delay must be >= 0")
	}
	if n.MaxTextRunes <= 0 {
		return errors.New("notifier.max_text_runes must be > 0")
	}
	return nil
}

func (c *TGNotifier) Validate() error {
	if c.AppName == "" {
		c.AppName = "tg-notifier"
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

	if c.Notifier.Interval <= 0 {
		c.Notifier.Interval = 2 * time.Minute
	}
	if c.Notifier.BatchSize <= 0 {
		c.Notifier.BatchSize = 20
	}
	if c.Notifier.MinDelay == 0 {
		c.Notifier.MinDelay = 200 * time.Millisecond
	}
	if c.Notifier.MaxTextRunes <= 0 {
		c.Notifier.MaxTextRunes = 900
	}

	if err := c.Notifier.Validate(); err != nil {
		return fmt.Errorf("notifier: %w", err)
	}

	if c.TelegramBot.PollTimeout <= 0 {
		c.TelegramBot.PollTimeout = 30 * time.Second
	}
	if err := c.TelegramBot.Validate(true); err != nil {
		return fmt.Errorf("telegram_bot: %w", err)
	}

	return nil
}

func New() *TGNotifier {
	c := cfg.MustLoad[TGNotifier](cfg.Options{
		Paths: []string{
			"./services/tgnotifier/config",
			"./config",
			"./configs",
			"/etc/telegram-bot-scraper",
		},
		Names:         []string{"tgnotifier", "config", "config.local"},
		Type:          "yaml",
		EnvPrefix:     "TG_NOTIFIER",
		OptionalFiles: true,
	})

	if err := c.Validate(); err != nil {
		panic(fmt.Errorf("invalid tgnotifier config: %w", err))
	}

	return c
}
