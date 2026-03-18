package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/classifier"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/ollama"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/storage"
)

func main() {
	os.Exit(run())
}

func run() int {
	// ---- flags (env-friendly defaults) ----
	var (
		dbPath          = flag.String("db", getenv("TG_DB_PATH", "data/scraper.db"), "path to sqlite db (same file as tgcollector/tgnotifier)")
		ollamaURL       = flag.String("ollama-url", getenv("OLLAMA_URL", "http://127.0.0.1:11434"), "ollama base url")
		model           = flag.String("model", getenv("OLLAMA_MODEL", "qwen2.5:7b"), "ollama model name")
		interval        = flag.Duration("interval", getenvDuration("CLASSIFIER_INTERVAL", 5*time.Second), "worker tick interval")
		batch           = flag.Int("batch", getenvInt("CLASSIFIER_BATCH", 20), "batch size per tick")
		lease           = flag.Duration("lease", getenvDuration("CLASSIFIER_LEASE", 2*time.Minute), "processing lease (lock) time")
		workerID        = flag.String("worker-id", getenv("CLASSIFIER_WORKER_ID", ""), "worker id (optional)")
		onlyUndelivered = flag.Bool("only-undelivered", getenvBool("CLASSIFIER_ONLY_UNDELIVERED", false), "classify only undelivered hits")
		maxTextRunes    = flag.Int("max-text-runes", getenvInt("CLASSIFIER_MAX_TEXT_RUNES", 1800), "truncate text to this rune length before sending to LLM")
		maxRetries      = flag.Int("max-retries", getenvInt("CLASSIFIER_MAX_RETRIES", 2), "max retries per hit on LLM/parse/db errors")
		retryBackoff    = flag.Duration("retry-backoff", getenvDuration("CLASSIFIER_RETRY_BACKOFF", 750*time.Millisecond), "retry backoff base")
		ollamaTimeout   = flag.Duration("ollama-timeout", getenvDuration("OLLAMA_TIMEOUT", 60*time.Second), "ollama request timeout")
		logLevel        = flag.String("log-level", getenv("LOG_LEVEL", "info"), "log level: debug|info|warn|error")
	)
	flag.Parse()

	// ---- logger ----
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(*logLevel),
	}))
	slog.SetDefault(logger)

	// ---- validate ----
	if strings.TrimSpace(*dbPath) == "" {
		logger.Error("db path is empty")
		return 2
	}
	if strings.TrimSpace(*ollamaURL) == "" {
		logger.Error("ollama url is empty")
		return 2
	}
	if strings.TrimSpace(*model) == "" {
		logger.Error("model is empty")
		return 2
	}

	// ---- graceful shutdown context ----
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ---- open storage ----
	st, err := storage.NewSQLite(storage.SQLiteConfig{
		Path:           *dbPath,
		BusyTimeout:    10 * time.Second,
		JournalModeWAL: true,
	})
	if err != nil {
		logger.Error("open sqlite failed", "err", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	// ---- ollama client ----
	oc, err := ollama.NewClient(ollama.Config{
		BaseURL: *ollamaURL,
		Timeout: *ollamaTimeout,
	})
	if err != nil {
		logger.Error("create ollama client failed", "err", err)
		return 1
	}

	// ---- worker ----
	w, err := classifier.NewWorker(logger, classifier.Config{
		Interval:        *interval,
		BatchSize:       *batch,
		Lease:           *lease,
		WorkerID:        strings.TrimSpace(*workerID),
		Model:           *model,
		MaxTextRunes:    *maxTextRunes,
		MaxRetries:      *maxRetries,
		RetryBackoff:    *retryBackoff,
		OnlyUndelivered: *onlyUndelivered,
	}, st, oc)
	if err != nil {
		logger.Error("create worker failed", "err", err)
		return 1
	}

	logger.Info("tgclassifier starting",
		"db", *dbPath,
		"ollama_url", *ollamaURL,
		"model", *model,
		"interval", interval.String(),
		"batch", *batch,
	)

	err = w.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("worker stopped with error", "err", err)
		return 1
	}

	logger.Info("tgclassifier stopped")
	return 0
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getenv(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func getenvInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	var x int
	_, err := fmt.Sscanf(v, "%d", &x)
	if err != nil {
		return def
	}
	return x
}

func getenvBool(k string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func getenvDuration(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
