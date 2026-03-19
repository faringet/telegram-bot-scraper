package logger

import (
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

type Options struct {
	AppName string
	Env     string
	Level   string
	JSON    bool
}

func NewLogger(opts Options) *slog.Logger {
	lvl := parseLevel(strings.ToLower(strings.TrimSpace(opts.Level)))
	handlerOpts := handlerOptions(lvl)

	var h slog.Handler
	if opts.JSON {
		h = slog.NewJSONHandler(os.Stdout, handlerOpts)
	} else {
		h = slog.NewTextHandler(os.Stdout, handlerOpts)
	}

	host, _ := os.Hostname()

	app := strings.TrimSpace(opts.AppName)
	if app == "" {
		app = "telegram-bot-scraper"
	}

	env := strings.TrimSpace(opts.Env)
	if env == "" {
		env = "dev"
	}

	return slog.New(h).With(
		slog.String("app", app),
		slog.String("env", env),
		slog.String("host", host),
		slog.Int("pid", os.Getpid()),
		slog.String("goarch", runtime.GOARCH),
		slog.String("goos", runtime.GOOS),
	)
}

func handlerOptions(lvl slog.Level) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level:       lvl,
		AddSource:   lvl <= slog.LevelDebug,
		ReplaceAttr: normalizeCoreAttrs,
	}
}

func normalizeCoreAttrs(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.UTC().Format(time.RFC3339Nano))
		}
		return a
	case slog.LevelKey:
		if lv, ok := a.Value.Any().(slog.Level); ok {
			a.Value = slog.StringValue(strings.ToLower(lv.String()))
		}
		return a
	default:
		return a
	}
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}
