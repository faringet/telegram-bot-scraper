package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Hit struct {
	Channel     string
	MessageID   int
	MessageDate time.Time
	Text        string
	Link        string
	Keyword     string
}

type Store interface {
	SaveHit(ctx context.Context, h Hit) (inserted bool, err error)

	GetCheckpoint(ctx context.Context, channelUsername string) (lastMessageID int, err error)
	SetCheckpoint(ctx context.Context, channelUsername string, lastMessageID int) error

	Prune(ctx context.Context) error
	Close() error
}

type SQLiteConfig struct {
	Path           string
	DedupWindow    time.Duration
	BusyTimeout    time.Duration
	JournalModeWAL bool
}

type SQLite struct {
	cfg SQLiteConfig
	db  *sql.DB
}

func NewSQLite(cfg SQLiteConfig) (*SQLite, error) {
	if cfg.Path == "" {
		return nil, errors.New("sqlite: path is required")
	}
	if cfg.BusyTimeout <= 0 {
		cfg.BusyTimeout = 5 * time.Second
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: mkdir data dir: %w", err)
	}

	pragma := ""
	if cfg.JournalModeWAL {
		pragma += "&_pragma=journal_mode(WAL)"
	}
	if cfg.BusyTimeout > 0 {
		pragma += fmt.Sprintf("&_pragma=busy_timeout(%d)", cfg.BusyTimeout.Milliseconds())
	}
	dsn := fmt.Sprintf("file:%s?cache=shared%s", cfg.Path, pragma)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite pragma: %w", err)
	}

	s := &SQLite{cfg: cfg, db: db}
	if err := s.migrate(); err != nil {
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

func (s *SQLite) migrate() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS checkpoints (
			channel_username TEXT PRIMARY KEY,
			last_message_id  INTEGER NOT NULL,
			updated_at_unix  INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS hits (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			channel            TEXT NOT NULL,
			message_id         INTEGER NOT NULL,
			message_date_unix  INTEGER NOT NULL,
			text               TEXT NOT NULL,
			link               TEXT NOT NULL,
			keyword            TEXT NOT NULL,
			created_at_unix    INTEGER NOT NULL,
			delivered_at_unix  INTEGER NULL,
			UNIQUE(channel, message_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_hits_delivered ON hits(delivered_at_unix);`,
		`CREATE INDEX IF NOT EXISTS idx_hits_message_date ON hits(message_date_unix);`,
	}

	for _, q := range ddl {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("sqlite migrate: %w", err)
		}
	}
	return nil
}

func (s *SQLite) SaveHit(ctx context.Context, h Hit) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("sqlite: db is nil")
	}
	if h.Channel == "" || h.MessageID <= 0 || h.Text == "" || h.Link == "" || h.Keyword == "" {
		return false, errors.New("sqlite: invalid hit (channel/message_id/text/link/keyword required)")
	}

	now := time.Now().UTC().Unix()
	msgUnix := h.MessageDate.UTC().Unix()

	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO hits
			(channel, message_id, message_date_unix, text, link, keyword, created_at_unix, delivered_at_unix)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, NULL);
	`, h.Channel, h.MessageID, msgUnix, h.Text, h.Link, h.Keyword, now)
	if err != nil {
		return false, fmt.Errorf("sqlite insert hit: %w", err)
	}

	aff, _ := res.RowsAffected()
	return aff > 0, nil
}

func (s *SQLite) GetCheckpoint(ctx context.Context, channelUsername string) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("sqlite: db is nil")
	}
	if channelUsername == "" {
		return 0, errors.New("sqlite: channelUsername is required")
	}

	var lastID int
	err := s.db.QueryRowContext(ctx, `
		SELECT last_message_id
		FROM checkpoints
		WHERE channel_username = ?;
	`, channelUsername).Scan(&lastID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("sqlite get checkpoint: %w", err)
	}

	return lastID, nil
}

func (s *SQLite) SetCheckpoint(ctx context.Context, channelUsername string, lastMessageID int) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite: db is nil")
	}
	if channelUsername == "" {
		return errors.New("sqlite: channelUsername is required")
	}
	if lastMessageID <= 0 {
		return errors.New("sqlite: lastMessageID must be > 0")
	}

	now := time.Now().UTC().Unix()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO checkpoints (channel_username, last_message_id, updated_at_unix)
		VALUES (?, ?, ?)
		ON CONFLICT(channel_username)
		DO UPDATE SET last_message_id=excluded.last_message_id, updated_at_unix=excluded.updated_at_unix;
	`, channelUsername, lastMessageID, now)
	if err != nil {
		return fmt.Errorf("sqlite set checkpoint: %w", err)
	}

	return nil
}

func (s *SQLite) Prune(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite: db is nil")
	}
	if s.cfg.DedupWindow <= 0 {
		return nil
	}

	cutoff := time.Now().UTC().Add(-s.cfg.DedupWindow).Unix()
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM hits
		WHERE created_at_unix < ?;
	`, cutoff)
	if err != nil {
		return fmt.Errorf("sqlite prune hits: %w", err)
	}

	return nil
}
