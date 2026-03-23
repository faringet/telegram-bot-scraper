package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/faringet/telegram-bot-scraper/internal/platform/searchtext"
)

type Postgres struct {
	db          *sql.DB
	dedupWindow time.Duration
}

func NewPostgres(db *sql.DB, dedupWindow time.Duration) (*Postgres, error) {
	if db == nil {
		return nil, errors.New("collector postgres storage: db is nil")
	}
	return &Postgres{
		db:          db,
		dedupWindow: dedupWindow,
	}, nil
}

func (s *Postgres) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Postgres) SaveHit(ctx context.Context, h Hit) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("collector postgres storage: db is nil")
	}
	if h.Channel == "" || h.MessageID <= 0 || h.Text == "" || h.Link == "" || h.Keyword == "" {
		return false, errors.New("collector postgres storage: invalid hit (channel/message_id/text/link/keyword required)")
	}
	if h.MessageDate.IsZero() {
		return false, errors.New("collector postgres storage: message_date is required")
	}

	searchText, searchTextNormalized := searchtext.Build(h.Channel, h.Keyword, h.Text)
	if searchText == "" || searchTextNormalized == "" {
		return false, errors.New("collector postgres storage: search text is empty")
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO hits (
	channel,
	message_id,
	message_date,
	text,
	link,
	keyword,
	search_text,
	search_text_normalized,
	created_at,
	delivered_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NULL)
ON CONFLICT (channel, message_id) DO NOTHING
`, h.Channel, h.MessageID, h.MessageDate.UTC(), h.Text, h.Link, h.Keyword, searchText, searchTextNormalized)
	if err != nil {
		return false, fmt.Errorf("collector postgres save hit: %w", err)
	}

	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func (s *Postgres) GetCheckpoint(ctx context.Context, channelUsername string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("collector postgres storage: db is nil")
	}
	if channelUsername == "" {
		return 0, errors.New("collector postgres storage: channelUsername is required")
	}

	var lastID int64
	err := s.db.QueryRowContext(ctx, `
SELECT last_message_id
FROM checkpoints
WHERE channel_username = $1
`, channelUsername).Scan(&lastID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("collector postgres get checkpoint: %w", err)
	}

	return lastID, nil
}

func (s *Postgres) SetCheckpoint(ctx context.Context, channelUsername string, lastMessageID int64) error {
	if s == nil || s.db == nil {
		return errors.New("collector postgres storage: db is nil")
	}
	if channelUsername == "" {
		return errors.New("collector postgres storage: channelUsername is required")
	}
	if lastMessageID <= 0 {
		return errors.New("collector postgres storage: lastMessageID must be > 0")
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO checkpoints (channel_username, last_message_id, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (channel_username)
DO UPDATE SET
	last_message_id = EXCLUDED.last_message_id,
	updated_at = EXCLUDED.updated_at
`, channelUsername, lastMessageID)
	if err != nil {
		return fmt.Errorf("collector postgres set checkpoint: %w", err)
	}

	return nil
}

func (s *Postgres) Prune(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("collector postgres storage: db is nil")
	}
	if s.dedupWindow <= 0 {
		return nil
	}

	cutoff := time.Now().UTC().Add(-s.dedupWindow)

	_, err := s.db.ExecContext(ctx, `
DELETE FROM hits
WHERE created_at < $1
`, cutoff)
	if err != nil {
		return fmt.Errorf("collector postgres prune hits: %w", err)
	}

	return nil
}
