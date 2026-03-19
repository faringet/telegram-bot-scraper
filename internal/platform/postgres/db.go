package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func Open(cfg Config) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, errors.New("postgres: dsn is required")
	}

	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 10
	}
	if cfg.MaxIdleConns < 0 {
		cfg.MaxIdleConns = 0
	}
	if cfg.ConnMaxLifetime < 0 {
		cfg.ConnMaxLifetime = 0
	}
	if cfg.ConnMaxIdleTime < 0 {
		cfg.ConnMaxIdleTime = 0
	}

	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	return db, nil
}

func WaitReady(ctx context.Context, db *sql.DB, interval time.Duration) error {
	if db == nil {
		return errors.New("postgres: db is nil")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.PingContext(pingCtx)
		cancel()

		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres wait ready: %w", ctx.Err())
		case <-t.C:
		}
	}
}
