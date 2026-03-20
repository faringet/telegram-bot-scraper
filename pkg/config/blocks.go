package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Base struct {
	AppName string `mapstructure:"app_name"`
	Env     string `mapstructure:"env"`
}

func (b *Base) Validate() error {
	if b == nil {
		return errors.New("base config is nil")
	}

	b.AppName = strings.TrimSpace(b.AppName)
	b.Env = strings.TrimSpace(strings.ToLower(b.Env))

	return nil
}

type Runtime struct {
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

func (r *Runtime) Validate() error {
	if r == nil {
		return errors.New("runtime config is nil")
	}
	if r.ShutdownTimeout < 0 {
		return errors.New("runtime.shutdown_timeout must be >= 0")
	}
	return nil
}

type Logger struct {
	Level string `mapstructure:"level"`
	JSON  bool   `mapstructure:"json"`
}

func (l *Logger) Validate() error {
	if l == nil {
		return errors.New("logger config is nil")
	}
	if strings.TrimSpace(l.Level) == "" {
		return errors.New("logger.level is required")
	}
	switch strings.ToLower(strings.TrimSpace(l.Level)) {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("logger.level must be one of [debug, info, warn, error], got %q", l.Level)
	}
	return nil
}

type Storage struct {
	Driver   string          `mapstructure:"driver"`
	SQLite   SQLiteStorage   `mapstructure:"sqlite"`
	Postgres PostgresStorage `mapstructure:"postgres"`
}

func (s *Storage) Validate() error {
	if s == nil {
		return errors.New("storage config is nil")
	}

	driver := strings.ToLower(strings.TrimSpace(s.Driver))
	switch driver {
	case "sqlite":
		s.Driver = driver
		return s.SQLite.Validate()
	case "postgres", "postgresql":
		s.Driver = "postgres"
		return s.Postgres.Validate()
	default:
		return fmt.Errorf("storage.driver must be one of [sqlite, postgres], got %q", s.Driver)
	}
}

type SQLiteStorage struct {
	Path           string        `mapstructure:"path"`
	BusyTimeout    time.Duration `mapstructure:"busy_timeout"`
	JournalModeWAL bool          `mapstructure:"journal_mode_wal"`
}

func (s *SQLiteStorage) Validate() error {
	if s == nil {
		return errors.New("storage.sqlite config is nil")
	}
	if strings.TrimSpace(s.Path) == "" {
		return errors.New("storage.sqlite.path is required")
	}
	if s.BusyTimeout < 0 {
		return errors.New("storage.sqlite.busy_timeout must be >= 0")
	}
	return nil
}

type PostgresStorage struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

func (p *PostgresStorage) Validate() error {
	if p == nil {
		return errors.New("storage.postgres config is nil")
	}
	if strings.TrimSpace(p.DSN) == "" {
		return errors.New("storage.postgres.dsn is required")
	}
	if p.MaxOpenConns < 0 {
		return errors.New("storage.postgres.max_open_conns must be >= 0")
	}
	if p.MaxIdleConns < 0 {
		return errors.New("storage.postgres.max_idle_conns must be >= 0")
	}
	if p.ConnMaxLifetime < 0 {
		return errors.New("storage.postgres.conn_max_lifetime must be >= 0")
	}
	if p.ConnMaxIdleTime < 0 {
		return errors.New("storage.postgres.conn_max_idle_time must be >= 0")
	}
	return nil
}

type Ollama struct {
	BaseURL string        `mapstructure:"base_url"`
	Timeout time.Duration `mapstructure:"timeout"`
	Model   string        `mapstructure:"model"`
}

func (o *Ollama) Validate() error {
	if o == nil {
		return errors.New("ollama config is nil")
	}
	if strings.TrimSpace(o.BaseURL) == "" {
		return errors.New("ollama.base_url is required")
	}
	if strings.TrimSpace(o.Model) == "" {
		return errors.New("ollama.model is required")
	}
	if o.Timeout <= 0 {
		return errors.New("ollama.timeout must be > 0")
	}
	return nil
}

type TelegramBot struct {
	Token       string        `mapstructure:"token"`
	Debug       bool          `mapstructure:"debug"`
	UseWebhook  bool          `mapstructure:"use_webhook"`
	WebhookURL  string        `mapstructure:"webhook_url"`
	PollTimeout time.Duration `mapstructure:"poll_timeout"`
}

func (t *TelegramBot) Validate(enabled bool) error {
	if !enabled {
		return nil
	}
	if t == nil {
		return errors.New("telegram_bot config is nil")
	}
	if strings.TrimSpace(t.Token) == "" {
		return errors.New("telegram_bot.token is required")
	}
	if t.UseWebhook && strings.TrimSpace(t.WebhookURL) == "" {
		return errors.New("telegram_bot.webhook_url is required when use_webhook=true")
	}
	if !t.UseWebhook && t.PollTimeout <= 0 {
		return errors.New("telegram_bot.poll_timeout must be > 0 when use_webhook=false")
	}
	return nil
}

type MTProto struct {
	APIID     int    `mapstructure:"api_id"`
	APIHash   string `mapstructure:"api_hash"`
	Phone     string `mapstructure:"phone"`
	Password  string `mapstructure:"password"`
	AppID     string `mapstructure:"app_id"`
	Session   string `mapstructure:"session"`
	Device    Device `mapstructure:"device"`
	RateLimit Rate   `mapstructure:"rate_limit"`
}

type Device struct {
	Model      string `mapstructure:"model"`
	System     string `mapstructure:"system"`
	AppVersion string `mapstructure:"app_version"`
	LangCode   string `mapstructure:"lang_code"`
	SystemLang string `mapstructure:"system_lang"`
}

type Rate struct {
	MinDelay    time.Duration `mapstructure:"min_delay"`
	Concurrency int           `mapstructure:"concurrency"`
}

func (m *MTProto) Validate() error {
	if m == nil {
		return errors.New("mtproto config is nil")
	}
	if m.APIID == 0 {
		return errors.New("mtproto.api_id is required")
	}
	if strings.TrimSpace(m.APIHash) == "" {
		return errors.New("mtproto.api_hash is required")
	}
	if strings.TrimSpace(m.Session) == "" {
		return errors.New("mtproto.session is required (e.g. data/session.json)")
	}

	if strings.TrimSpace(m.Device.Model) == "" {
		return errors.New("mtproto.device.model is required")
	}
	if strings.TrimSpace(m.Device.System) == "" {
		return errors.New("mtproto.device.system is required")
	}
	if strings.TrimSpace(m.Device.AppVersion) == "" {
		return errors.New("mtproto.device.app_version is required")
	}
	if strings.TrimSpace(m.Device.LangCode) == "" {
		return errors.New("mtproto.device.lang_code is required")
	}
	if strings.TrimSpace(m.Device.SystemLang) == "" {
		return errors.New("mtproto.device.system_lang is required")
	}

	if m.RateLimit.MinDelay <= 0 {
		return errors.New("mtproto.rate_limit.min_delay must be > 0 (e.g. 400ms)")
	}
	if m.RateLimit.Concurrency <= 0 {
		return errors.New("mtproto.rate_limit.concurrency must be > 0")
	}

	return nil
}

type Scrape struct {
	Keywords []string `mapstructure:"keywords"`
	Channels []string `mapstructure:"channels"`

	Lookback    time.Duration `mapstructure:"lookback"`
	DedupWindow time.Duration `mapstructure:"dedup_window"`

	PerChannelMaxScan    int           `mapstructure:"per_channel_max_scan"`
	MinDelay             time.Duration `mapstructure:"min_delay"`
	BetweenChannelsDelay time.Duration `mapstructure:"between_channels_delay"`

	Interval time.Duration `mapstructure:"interval"`
}

func (s *Scrape) Validate() error {
	if s == nil {
		return errors.New("scrape config is nil")
	}

	for i := range s.Keywords {
		s.Keywords[i] = strings.TrimSpace(strings.ToLower(s.Keywords[i]))
	}
	kw := s.Keywords[:0]
	for _, k := range s.Keywords {
		if k != "" {
			kw = append(kw, k)
		}
	}
	s.Keywords = kw
	if len(s.Keywords) == 0 {
		return errors.New("scrape.keywords must contain at least 1 keyword")
	}

	for i := range s.Channels {
		s.Channels[i] = strings.TrimSpace(s.Channels[i])
	}
	ch := s.Channels[:0]
	for _, c := range s.Channels {
		if c != "" {
			ch = append(ch, c)
		}
	}
	s.Channels = ch
	if len(s.Channels) == 0 {
		return errors.New("scrape.channels must contain at least 1 channel")
	}

	if s.Lookback <= 0 {
		return errors.New("scrape.lookback must be > 0")
	}
	if s.DedupWindow <= 0 {
		return errors.New("scrape.dedup_window must be > 0")
	}

	if s.PerChannelMaxScan < 0 {
		return errors.New("scrape.per_channel_max_scan must be >= 0")
	}
	if s.MinDelay < 0 {
		return errors.New("scrape.min_delay must be >= 0")
	}
	if s.BetweenChannelsDelay < 0 {
		return errors.New("scrape.between_channels_delay must be >= 0")
	}
	if s.Interval <= 0 {
		return errors.New("scrape.interval must be > 0")
	}

	return nil
}
