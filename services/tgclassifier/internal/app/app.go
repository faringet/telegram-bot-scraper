package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	platformpg "github.com/faringet/telegram-bot-scraper/internal/platform/postgres"
	tgcfg "github.com/faringet/telegram-bot-scraper/services/tgclassifier/config"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/classifier"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/ollama"
	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/storage"
)

type App struct {
	cfg *tgcfg.TGClassifier
	log *slog.Logger

	store  storage.Store
	worker *classifier.Worker
}

func New(cfg *tgcfg.TGClassifier, log *slog.Logger) (*App, error) {
	if cfg == nil {
		return nil, errors.New("classifier app: config is nil")
	}
	if log == nil {
		return nil, errors.New("classifier app: logger is nil")
	}

	log = log.With(
		slog.String("layer", "app"),
		slog.String("module", "classifier.app"),
	)

	st, err := openStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	oc, err := ollama.NewClient(ollama.Config{
		BaseURL: cfg.Ollama.BaseURL,
		Timeout: cfg.Ollama.Timeout,
	})
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("create ollama client: %w", err)
	}

	w, err := classifier.NewWorker(log, classifier.Config{
		Interval:        cfg.Classifier.Interval,
		BatchSize:       cfg.Classifier.BatchSize,
		Lease:           cfg.Classifier.Lease,
		WorkerID:        cfg.Classifier.WorkerID,
		Model:           cfg.Ollama.Model,
		MaxTextRunes:    cfg.Classifier.MaxTextRunes,
		MaxRetries:      cfg.Classifier.MaxRetries,
		RetryBackoff:    cfg.Classifier.RetryBackoff,
		OnlyUndelivered: cfg.Classifier.OnlyUndelivered,
		WhitelistPath:   cfg.Classifier.WhitelistPath,
		PromptPath:      cfg.Classifier.PromptPath,
	}, st, oc)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("create worker: %w", err)
	}

	return &App{
		cfg:    cfg,
		log:    log,
		store:  st,
		worker: w,
	}, nil
}

func openStore(cfg *tgcfg.TGClassifier) (storage.Store, error) {
	switch cfg.Storage.Driver {
	case "sqlite":
		return nil, errors.New("sqlite storage is not implemented yet; use storage.driver=postgres")

	case "postgres":
		db, err := platformpg.Open(platformpg.Config{
			DSN:             cfg.Storage.Postgres.DSN,
			MaxOpenConns:    cfg.Storage.Postgres.MaxOpenConns,
			MaxIdleConns:    cfg.Storage.Postgres.MaxIdleConns,
			ConnMaxLifetime: cfg.Storage.Postgres.ConnMaxLifetime,
			ConnMaxIdleTime: cfg.Storage.Postgres.ConnMaxIdleTime,
		})
		if err != nil {
			return nil, fmt.Errorf("open postgres db: %w", err)
		}

		st, err := storage.NewPostgres(db)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("create postgres storage: %w", err)
		}
		return st, nil

	default:
		return nil, fmt.Errorf("unsupported storage driver: %s", cfg.Storage.Driver)
	}
}

func (a *App) Run(ctx context.Context) error {
	a.log.Info("run started",
		slog.String("storage_driver", a.cfg.Storage.Driver),
		slog.String("ollama_url", a.cfg.Ollama.BaseURL),
		slog.String("model", a.cfg.Ollama.Model),
		slog.Duration("interval", a.cfg.Classifier.Interval),
		slog.Int("batch_size", a.cfg.Classifier.BatchSize),
	)

	switch a.cfg.Storage.Driver {
	case "sqlite":
		a.log.Info("sqlite storage configured",
			slog.String("sqlite_path", a.cfg.Storage.SQLite.Path),
		)
	case "postgres":
		a.log.Info("postgres storage configured",
			slog.Int("max_open_conns", a.cfg.Storage.Postgres.MaxOpenConns),
			slog.Int("max_idle_conns", a.cfg.Storage.Postgres.MaxIdleConns),
		)
	}

	return a.worker.Run(ctx)
}

func (a *App) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}
