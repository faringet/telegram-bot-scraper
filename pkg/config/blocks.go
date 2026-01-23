package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Logger struct {
	Level   string `mapstructure:"level"`
	JSON    bool   `mapstructure:"json"`
	AppName string `mapstructure:"app_name"`
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
		return errors.New("scrape.interval must be > 0 ")
	}

	return nil
}
