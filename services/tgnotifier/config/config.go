package config

import (
	"errors"
	"fmt"
	"strings"
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

type Schedule struct {
	Timezone       string `mapstructure:"timezone"`
	DailyAt        string `mapstructure:"daily_at"`
	CatchUpOnStart *bool  `mapstructure:"catch_up_on_start"`
}

type Notifier struct {
	SupervisorChatID int64         `mapstructure:"supervisor_chat_id"`
	Schedule         Schedule      `mapstructure:"schedule"`
	BatchSize        int           `mapstructure:"batch_size"`
	MinDelay         time.Duration `mapstructure:"min_delay"`
	MaxTextRunes     int           `mapstructure:"max_text_runes"`
	DryRun           bool          `mapstructure:"dry_run"`
}

func (s *Schedule) setDefaults() {
	if strings.TrimSpace(s.Timezone) == "" {
		s.Timezone = "Europe/Moscow"
	}
	if strings.TrimSpace(s.DailyAt) == "" {
		s.DailyAt = "09:00"
	}
	if s.CatchUpOnStart == nil {
		v := true
		s.CatchUpOnStart = &v
	}
}

func (s Schedule) CatchUpEnabled() bool {
	if s.CatchUpOnStart == nil {
		return true
	}
	return *s.CatchUpOnStart
}

func (s *Schedule) Validate() error {
	if s == nil {
		return errors.New("schedule config is nil")
	}
	if strings.TrimSpace(s.Timezone) == "" {
		return errors.New("notifier.schedule.timezone is required")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("notifier.schedule.timezone is invalid: %w", err)
	}
	if strings.TrimSpace(s.DailyAt) == "" {
		return errors.New("notifier.schedule.daily_at is required")
	}
	if _, err := time.Parse("15:04", s.DailyAt); err != nil {
		return fmt.Errorf("notifier.schedule.daily_at must be HH:MM, got %q", s.DailyAt)
	}
	return nil
}

func (n *Notifier) setDefaults() {
	n.Schedule.setDefaults()

	if n.BatchSize <= 0 {
		n.BatchSize = 50
	}
	if n.MinDelay == 0 {
		n.MinDelay = 500 * time.Millisecond
	}
	if n.MaxTextRunes <= 0 {
		n.MaxTextRunes = 300
	}
}

func (n *Notifier) Validate() error {
	if n == nil {
		return errors.New("notifier config is nil")
	}
	if n.SupervisorChatID == 0 {
		return errors.New("notifier.supervisor_chat_id is required")
	}
	if err := n.Schedule.Validate(); err != nil {
		return err
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
		c.Storage.Driver = "postgres"
	}
	if c.Storage.Postgres.MaxOpenConns <= 0 {
		c.Storage.Postgres.MaxOpenConns = 10
	}
	if c.Storage.Postgres.MaxIdleConns < 0 {
		c.Storage.Postgres.MaxIdleConns = 0
	}
	if c.Storage.Postgres.ConnMaxLifetime < 0 {
		c.Storage.Postgres.ConnMaxLifetime = 0
	}
	if c.Storage.Postgres.ConnMaxIdleTime < 0 {
		c.Storage.Postgres.ConnMaxIdleTime = 0
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
	if c.Storage.Driver != "postgres" {
		return fmt.Errorf("tgnotifier supports only storage.driver=postgres, got %q", c.Storage.Driver)
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
