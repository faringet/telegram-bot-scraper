package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) (*Postgres, error) {
	if db == nil {
		return nil, errors.New("notifier postgres storage: db is nil")
	}
	return &Postgres{db: db}, nil
}

func (s *Postgres) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Postgres) ListUndeliveredBefore(ctx context.Context, limit int, classifiedBefore time.Time) ([]Hit, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("notifier postgres storage: db is nil")
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT
	id,
	channel,
	message_id,
	message_date,
	text,
	link,
	keyword,
	delivered_at,
	category,
	classified_at,
	llm_model,
	llm_confidence,
	llm_reason
FROM hits
WHERE delivered_at IS NULL
  AND category IS NOT NULL
  AND classified_at IS NOT NULL
  AND classified_at <= $1
ORDER BY classified_at ASC, message_date ASC, id ASC
LIMIT $2
`, classifiedBefore.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("notifier postgres list undelivered before: %w", err)
	}
	defer rows.Close()

	out := make([]Hit, 0, limit)

	for rows.Next() {
		var h Hit
		if err := rows.Scan(
			&h.ID,
			&h.Channel,
			&h.MessageID,
			&h.MessageDate,
			&h.Text,
			&h.Link,
			&h.Keyword,
			&h.DeliveredAt,
			&h.Category,
			&h.ClassifiedAt,
			&h.LLMModel,
			&h.LLMConfidence,
			&h.LLMReason,
		); err != nil {
			return nil, fmt.Errorf("notifier postgres scan hit: %w", err)
		}

		h.MessageDate = h.MessageDate.UTC()
		if h.DeliveredAt.Valid {
			h.DeliveredAt.Time = h.DeliveredAt.Time.UTC()
		}
		if h.ClassifiedAt.Valid {
			h.ClassifiedAt.Time = h.ClassifiedAt.Time.UTC()
		}

		out = append(out, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifier postgres rows: %w", err)
	}

	return out, nil
}

func (s *Postgres) MarkDelivered(ctx context.Context, ids []int64) error {
	if s == nil || s.db == nil {
		return errors.New("notifier postgres storage: db is nil")
	}
	if len(ids) == 0 {
		return nil
	}

	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	query := `
UPDATE hits
SET delivered_at = NOW()
WHERE delivered_at IS NULL
  AND id IN (` + pgPlaceholders(1, len(ids)) + `)
`

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("notifier postgres mark delivered: %w", err)
	}

	return nil
}

func pgPlaceholders(start, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(parts, ",")
}
