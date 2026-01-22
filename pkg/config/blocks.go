// pkg/config/blocks.go
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Logger struct {
	Level   string `mapstructure:"level"`    // debug/info/warn/error
	JSON    bool   `mapstructure:"json"`     // true -> JSON logs
	AppName string `mapstructure:"app_name"` // будет проставлен сервисом (или задан явно)
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

type Mode struct {
	Kind string `mapstructure:"kind"` // mtproto | botapi (бот как UI, опционально)
}

func (m *Mode) Validate() error {
	if m == nil {
		return errors.New("mode config is nil")
	}
	if strings.TrimSpace(m.Kind) == "" {
		return errors.New("mode.kind is required (mtproto|botapi)")
	}
	switch m.Kind {
	case "mtproto", "botapi":
		return nil
	default:
		return fmt.Errorf("mode.kind must be one of [mtproto, botapi], got %q", m.Kind)
	}
}

type TelegramBot struct {
	Token       string        `mapstructure:"token"`        // токен бота (если используем botapi)
	Debug       bool          `mapstructure:"debug"`        // подробные логи апдейтов
	UseWebhook  bool          `mapstructure:"use_webhook"`  // webhook или long polling
	WebhookURL  string        `mapstructure:"webhook_url"`  // если webhook
	PollTimeout time.Duration `mapstructure:"poll_timeout"` // long polling timeout
}

func (t *TelegramBot) Validate(enabled bool) error {
	if !enabled {
		return nil
	}
	if t == nil {
		return errors.New("telegram_bot config is nil")
	}
	if strings.TrimSpace(t.Token) == "" {
		return errors.New("telegram_bot.token is required when mode=botapi")
	}
	if t.UseWebhook && strings.TrimSpace(t.WebhookURL) == "" {
		return errors.New("telegram_bot.webhook_url is required when use_webhook=true")
	}
	if !t.UseWebhook && t.PollTimeout <= 0 {
		return errors.New("telegram_bot.poll_timeout must be > 0 when use_webhook=false")
	}
	return nil
}

// - api_id / api_hash выдаются Telegram на https://my.telegram.org (это ключи приложения)
// - session — локальный файл, где сохраняется авторизация (чтобы не логиниться каждый раз)
// - phone — номер для первичной авторизации
// - password — 2FA пароль (если включен)
// - device_* — помогает сделать сессию "похожей на обычный клиент"
type MTProto struct {
	APIID     int    `mapstructure:"api_id"`
	APIHash   string `mapstructure:"api_hash"`
	Phone     string `mapstructure:"phone"`    // +49123...
	Password  string `mapstructure:"password"` // 2FA (если включено)
	AppID     string `mapstructure:"app_id"`   // произвольный идентификатор приложения в логах
	Session   string `mapstructure:"session"`  // путь к session file, напр. "data/session.json"
	Device    Device `mapstructure:"device"`
	RateLimit Rate   `mapstructure:"rate_limit"`
}

type Device struct {
	Model      string `mapstructure:"model"`       // "PC"
	System     string `mapstructure:"system"`      // "Windows 10"
	AppVersion string `mapstructure:"app_version"` // "1.0.0"
	LangCode   string `mapstructure:"lang_code"`   // "en" / "ru"
	SystemLang string `mapstructure:"system_lang"` // "en" / "ru"
}

type Rate struct {
	MinDelay    time.Duration `mapstructure:"min_delay"`   // 400ms
	Concurrency int           `mapstructure:"concurrency"` // 1..N
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

	// Phone/Password могут быть пустыми, если ты будешь вводить их интерактивно при первом логине.
	// Но для "строгого" режима можно сделать обязательным phone:
	// if strings.TrimSpace(m.Phone) == "" { return errors.New("mtproto.phone is required") }

	// Device: всё обязательное, чтобы не было "магии".
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

	// Rate-limit: строгий режим
	if m.RateLimit.MinDelay <= 0 {
		return errors.New("mtproto.rate_limit.min_delay must be > 0 (e.g. 400ms)")
	}
	if m.RateLimit.Concurrency <= 0 {
		return errors.New("mtproto.rate_limit.concurrency must be > 0")
	}

	return nil
}

// Scrape — что и где ищем
type Scrape struct {
	Keywords    []string      `mapstructure:"keywords"`     // ключевые слова (будем приводить к lower)
	Channels    []string      `mapstructure:"channels"`     // @channel или https://t.me/...
	Lookback    time.Duration `mapstructure:"lookback"`     // например 168h
	DedupWindow time.Duration `mapstructure:"dedup_window"` // например 720h
}

func (s *Scrape) Validate() error {
	if s == nil {
		return errors.New("scrape config is nil")
	}

	// keywords (normalize + strict)
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

	// channels (normalize + strict)
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

	// durations strict
	if s.Lookback <= 0 {
		return errors.New("scrape.lookback must be > 0 (e.g. 168h)")
	}
	if s.DedupWindow <= 0 {
		return errors.New("scrape.dedup_window must be > 0 (e.g. 720h)")
	}

	return nil
}
