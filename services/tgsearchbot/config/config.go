package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	pcfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type TGSearchBot struct {
	Base        pcfg.Base        `mapstructure:",squash"`
	Logger      pcfg.Logger      `mapstructure:"logger"`
	Runtime     pcfg.Runtime     `mapstructure:"runtime"`
	Storage     pcfg.Storage     `mapstructure:"storage"`
	TelegramBot pcfg.TelegramBot `mapstructure:"telegram_bot"`

	Access Access `mapstructure:"access"`
	Search Search `mapstructure:"search"`
}

type Access struct {
	AllowedUserIDs []int64 `mapstructure:"allowed_user_ids"`
	AllowedChatIDs []int64 `mapstructure:"allowed_chat_ids"`
	DenyMessage    string  `mapstructure:"deny_message"`
}

func (a *Access) setDefaults() {
	if strings.TrimSpace(a.DenyMessage) == "" {
		a.DenyMessage = "⛔ Доступ к боту ограничен."
	}
}

func (a *Access) Validate() error {
	if a == nil {
		return errors.New("access config is nil")
	}

	seenUsers := make(map[int64]struct{}, len(a.AllowedUserIDs))
	for _, id := range a.AllowedUserIDs {
		if id == 0 {
			return errors.New("access.allowed_user_ids must not contain 0")
		}
		if _, ok := seenUsers[id]; ok {
			return fmt.Errorf("duplicate allowed user id: %d", id)
		}
		seenUsers[id] = struct{}{}
	}

	seenChats := make(map[int64]struct{}, len(a.AllowedChatIDs))
	for _, id := range a.AllowedChatIDs {
		if id == 0 {
			return errors.New("access.allowed_chat_ids must not contain 0")
		}
		if _, ok := seenChats[id]; ok {
			return fmt.Errorf("duplicate allowed chat id: %d", id)
		}
		seenChats[id] = struct{}{}
	}

	return nil
}

type Search struct {
	DefaultLookback time.Duration `mapstructure:"default_lookback"`
	MaxResults      int           `mapstructure:"max_results"`
	MaxQueryRunes   int           `mapstructure:"max_query_runes"`
	MaxTextRunes    int           `mapstructure:"max_text_runes"`
}

func (s *Search) setDefaults() {
	if s.DefaultLookback <= 0 {
		s.DefaultLookback = 7 * 24 * time.Hour
	}
	if s.MaxResults <= 0 {
		s.MaxResults = 10
	}
	if s.MaxQueryRunes <= 0 {
		s.MaxQueryRunes = 120
	}
	if s.MaxTextRunes <= 0 {
		s.MaxTextRunes = 300
	}
}

func (s *Search) Validate() error {
	if s == nil {
		return errors.New("search config is nil")
	}
	if s.DefaultLookback <= 0 {
		return errors.New("search.default_lookback must be > 0")
	}
	if s.MaxResults <= 0 {
		return errors.New("search.max_results must be > 0")
	}
	if s.MaxQueryRunes <= 0 {
		return errors.New("search.max_query_runes must be > 0")
	}
	if s.MaxTextRunes <= 0 {
		return errors.New("search.max_text_runes must be > 0")
	}
	return nil
}

func (c *TGSearchBot) setDefaults() {
	if c.Base.AppName == "" {
		c.Base.AppName = "tg-searchbot"
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

	c.Access.setDefaults()
	c.Search.setDefaults()
}

func (c *TGSearchBot) Validate() error {
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
		return fmt.Errorf("tgsearchbot supports only storage.driver=postgres, got %q", c.Storage.Driver)
	}
	if err := c.TelegramBot.Validate(true); err != nil {
		return fmt.Errorf("telegram_bot: %w", err)
	}
	if err := c.Access.Validate(); err != nil {
		return fmt.Errorf("access: %w", err)
	}
	if err := c.Search.Validate(); err != nil {
		return fmt.Errorf("search: %w", err)
	}

	return nil
}

func New() *TGSearchBot {
	c := pcfg.MustLoad[TGSearchBot](pcfg.Options{
		Paths: []string{
			"./services/tgsearchbot/config",
			"./config",
			"./configs",
			"/etc/telegram-bot-scraper",
		},
		Names:         []string{"tgsearchbot", "config", "config.local"},
		Type:          "yaml",
		EnvPrefix:     "TG_SEARCHBOT",
		OptionalFiles: true,
	})

	if err := c.Validate(); err != nil {
		panic(fmt.Errorf("invalid tgsearchbot config: %w", err))
	}

	return c
}
