package config

import (
	"fmt"
	"time"

	pcfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

type TGClassifier struct {
	Base    pcfg.Base    `mapstructure:",squash"`
	Logger  pcfg.Logger  `mapstructure:"logger"`
	Runtime pcfg.Runtime `mapstructure:"runtime"`
	Storage pcfg.Storage `mapstructure:"storage"`
	Ollama  pcfg.Ollama  `mapstructure:"ollama"`

	Classifier Classifier `mapstructure:"classifier"`
}

type Classifier struct {
	Interval        time.Duration `mapstructure:"interval"`
	BatchSize       int           `mapstructure:"batch_size"`
	Lease           time.Duration `mapstructure:"lease"`
	WorkerID        string        `mapstructure:"worker_id"`
	MaxTextRunes    int           `mapstructure:"max_text_runes"`
	MaxRetries      int           `mapstructure:"max_retries"`
	RetryBackoff    time.Duration `mapstructure:"retry_backoff"`
	OnlyUndelivered bool          `mapstructure:"only_undelivered"`
	WhitelistPath   string        `mapstructure:"whitelist_path"`
	PromptPath      string        `mapstructure:"prompt_path"`
}

func (c *Classifier) setDefaults() {
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 20
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.MaxTextRunes <= 0 {
		c.MaxTextRunes = 1800
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = 750 * time.Millisecond
	}
}

func (c *Classifier) Validate() error {
	if c == nil {
		return fmt.Errorf("classifier config is nil")
	}
	if c.Interval <= 0 {
		return fmt.Errorf("classifier.interval must be > 0")
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("classifier.batch_size must be > 0")
	}
	if c.Lease <= 0 {
		return fmt.Errorf("classifier.lease must be > 0")
	}
	if c.MaxTextRunes <= 0 {
		return fmt.Errorf("classifier.max_text_runes must be > 0")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("classifier.max_retries must be >= 0")
	}
	if c.RetryBackoff < 0 {
		return fmt.Errorf("classifier.retry_backoff must be >= 0")
	}
	return nil
}

func (c *TGClassifier) setDefaults() {
	if c.Base.AppName == "" {
		c.Base.AppName = "tg-classifier"
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

	if c.Ollama.BaseURL == "" {
		c.Ollama.BaseURL = "http://127.0.0.1:11434"
	}
	if c.Ollama.Timeout <= 0 {
		c.Ollama.Timeout = 60 * time.Second
	}
	if c.Ollama.Model == "" {
		c.Ollama.Model = "qwen2.5:7b"
	}

	c.Classifier.setDefaults()
}

func (c *TGClassifier) Validate() error {
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
		return fmt.Errorf("tgclassifier supports only storage.driver=postgres, got %q", c.Storage.Driver)
	}
	if err := c.Ollama.Validate(); err != nil {
		return fmt.Errorf("ollama: %w", err)
	}
	if err := c.Classifier.Validate(); err != nil {
		return fmt.Errorf("classifier: %w", err)
	}

	return nil
}

func New() *TGClassifier {
	c := pcfg.MustLoad[TGClassifier](pcfg.Options{
		Paths: []string{
			"./services/tgclassifier/config",
			"./config",
			"./configs",
			"/etc/telegram-bot-scraper",
		},
		Names:         []string{"tgclassifier", "config", "config.local"},
		Type:          "yaml",
		EnvPrefix:     "TG_CLASSIFIER",
		OptionalFiles: true,
	})

	if err := c.Validate(); err != nil {
		panic(fmt.Errorf("invalid tgclassifier config: %w", err))
	}

	return c
}
