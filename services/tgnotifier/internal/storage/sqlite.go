package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Hit struct {
	ID              int64
	Channel         string
	MessageID       int
	MessageDateUnix int64
	Text            string
	Link            string
	Keyword         string
	CreatedAtUnix   int64
	DeliveredAtUnix sql.NullInt64
}

type Store interface {
	ListUndelivered(ctx context.Context, limit int) ([]Hit, error)
	MarkDelivered(ctx context.Context, ids []int64) error
	Close() error
}

type SQLiteConfig struct {
	Path        string
	BusyTimeout time.Duration
}

type SQLite struct {
	cfg SQLiteConfig
	db  *sql.DB
}

func NewSQLite(cfg SQLiteConfig) (*SQLite, error) {
	if cfg.BusyTimeout <= 0 {
		cfg.BusyTimeout = 5 * time.Second
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: mkdir data dir: %w", err)
	}

	dsn := fmt.Sprintf(
		"file:%s?cache=shared&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)",
		cfg.Path,
		cfg.BusyTimeout.Milliseconds(),
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	s := &SQLite{cfg: cfg, db: db}

	if err := s.sanityCheck(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLite) sanityCheck() error {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='hits';`).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("sqlite: table 'hits' not found (wrong db_path or collector not initialized)")
		}
		return fmt.Errorf("sqlite: sanity check: %w", err)
	}
	return nil
}

func (s *SQLite) ListUndelivered(ctx context.Context, limit int) ([]Hit, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("sqlite: db is nil")
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, channel, message_id, message_date_unix, text, link, keyword, created_at_unix, delivered_at_unix
		FROM hits
		WHERE delivered_at_unix IS NULL
		ORDER BY message_date_unix DESC
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite list undelivered: %w", err)
	}
	defer rows.Close()

	out := make([]Hit, 0, limit)
	for rows.Next() {
		var h Hit
		if err := rows.Scan(
			&h.ID,
			&h.Channel,
			&h.MessageID,
			&h.MessageDateUnix,
			&h.Text,
			&h.Link,
			&h.Keyword,
			&h.CreatedAtUnix,
			&h.DeliveredAtUnix,
		); err != nil {
			return nil, fmt.Errorf("sqlite scan hit: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite rows: %w", err)
	}

	return out, nil
}

func (s *SQLite) MarkDelivered(ctx context.Context, ids []int64) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite: db is nil")
	}
	if len(ids) == 0 {
		return nil
	}

	clean := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return nil
	}

	now := time.Now().UTC().Unix()

	placeholders := make([]string, 0, len(clean))
	args := make([]any, 0, 1+len(clean))
	args = append(args, now)

	for _, id := range clean {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	q := fmt.Sprintf(
		`UPDATE hits SET delivered_at_unix = ? WHERE id IN (%s);`,
		strings.Join(placeholders, ","),
	)

	_, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("sqlite mark delivered: %w", err)
	}

	return nil
}
