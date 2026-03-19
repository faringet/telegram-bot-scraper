package main

import (
	"fmt"
	"time"

	pcfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type Config struct {
	Base    pcfg.Base    `mapstructure:",squash"`
	Logger  pcfg.Logger  `mapstructure:"logger"`
	Runtime pcfg.Runtime `mapstructure:"runtime"`
	Storage pcfg.Storage `mapstructure:"storage"`

	Migrator Migrator `mapstructure:"migrator"`
}

type Migrator struct {
	LockNamespace  string        `mapstructure:"lock_namespace"`
	LockResource   string        `mapstructure:"lock_resource"`
	StartupTimeout time.Duration `mapstructure:"startup_timeout"`
	PingInterval   time.Duration `mapstructure:"ping_interval"`
}

func (c *Config) setDefaults() {
	if c.Base.AppName == "" {
		c.Base.AppName = "migrator"
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
		c.Storage.Postgres.MaxOpenConns = 4
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

	if c.Migrator.LockNamespace == "" {
		c.Migrator.LockNamespace = "telegram-bot-scraper"
	}
	if c.Migrator.LockResource == "" {
		c.Migrator.LockResource = "schema-migrations"
	}
	if c.Migrator.StartupTimeout <= 0 {
		c.Migrator.StartupTimeout = 60 * time.Second
	}
	if c.Migrator.PingInterval <= 0 {
		c.Migrator.PingInterval = 2 * time.Second
	}
}

func (c *Config) Validate() error {
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
		return fmt.Errorf("migrator supports only storage.driver=postgres, got %q", c.Storage.Driver)
	}
	if c.Migrator.LockNamespace == "" {
		return fmt.Errorf("migrator.lock_namespace is required")
	}
	if c.Migrator.LockResource == "" {
		return fmt.Errorf("migrator.lock_resource is required")
	}
	if c.Migrator.StartupTimeout <= 0 {
		return fmt.Errorf("migrator.startup_timeout must be > 0")
	}
	if c.Migrator.PingInterval <= 0 {
		return fmt.Errorf("migrator.ping_interval must be > 0")
	}

	return nil
}

func NewConfig() *Config {
	c := pcfg.MustLoad[Config](pcfg.Options{
		Paths: []string{
			"./cmd/migrator",
			"./config",
			"./configs",
			"/etc/telegram-bot-scraper",
		},
		Names:         []string{"migrator", "config", "config.local"},
		Type:          "yaml",
		EnvPrefix:     "MIGRATOR",
		OptionalFiles: true,
	})

	if err := c.Validate(); err != nil {
		panic(fmt.Errorf("invalid migrator config: %w", err))
	}

	return c
}
