package config

import (
	"errors"
	"fmt"
	"time"

	pcfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type TGNotifier struct {
	Base        pcfg.Base        `mapstructure:",squash"`
	Logger      pcfg.Logger      `mapstructure:"logger"`
	Runtime     pcfg.Runtime     `mapstructure:"runtime"`
	Storage     pcfg.Storage     `mapstructure:"storage"`
	TelegramBot pcfg.TelegramBot `mapstructure:"telegram_bot"`

	Notifier Notifier `mapstructure:"notifier"`
}

type Notifier struct {
	SupervisorChatID int64         `mapstructure:"supervisor_chat_id"`
	Interval         time.Duration `mapstructure:"interval"`
	BatchSize        int           `mapstructure:"batch_size"`
	MinDelay         time.Duration `mapstructure:"min_delay"`
	MaxTextRunes     int           `mapstructure:"max_text_runes"`
	DryRun           bool          `mapstructure:"dry_run"`
}

func (n *Notifier) setDefaults() {
	if n.Interval <= 0 {
		n.Interval = 2 * time.Minute
	}
	if n.BatchSize <= 0 {
		n.BatchSize = 20
	}
	if n.MinDelay == 0 {
		n.MinDelay = 200 * time.Millisecond
	}
	if n.MaxTextRunes <= 0 {
		n.MaxTextRunes = 200
	}
}

func (n *Notifier) Validate() error {
	if n == nil {
		return errors.New("notifier config is nil")
	}
	if n.SupervisorChatID == 0 {
		return errors.New("notifier.supervisor_chat_id is required")
	}
	if n.Interval <= 0 {
		return errors.New("notifier.interval must be > 0")
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

func (c *TGNotifier) setDefaults() {
	if c.Base.AppName == "" {
		c.Base.AppName = "tg-notifier"
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

	if c.TelegramBot.PollTimeout <= 0 {
		c.TelegramBot.PollTimeout = 30 * time.Second
	}

	c.Notifier.setDefaults()
}

func (c *TGNotifier) Validate() error {
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
	if err := c.TelegramBot.Validate(true); err != nil {
		return fmt.Errorf("telegram_bot: %w", err)
	}
	if err := c.Notifier.Validate(); err != nil {
		return fmt.Errorf("notifier: %w", err)
	}

	return nil
}

func New() *TGNotifier {
	c := pcfg.MustLoad[TGNotifier](pcfg.Options{
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
