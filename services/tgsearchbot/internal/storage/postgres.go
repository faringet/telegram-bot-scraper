package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) (*Postgres, error) {
	if db == nil {
		return nil, errors.New("searchbot postgres storage: db is nil")
	}
	return &Postgres{db: db}, nil
}

func (s *Postgres) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Postgres) SearchRecent(ctx context.Context, normalizedQuery string, since time.Time, limit int) ([]Hit, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("searchbot postgres storage: db is nil")
	}
	if normalizedQuery == "" {
		return nil, errors.New("searchbot postgres storage: normalized query is required")
	}
	if limit <= 0 {
		limit = 10
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
	category,
	llm_reason,
	llm_confidence,
	classified_at
FROM hits
WHERE message_date >= $1
  AND classified_at IS NOT NULL
  AND category IS NOT NULL
  AND search_text_normalized <> ''
  AND (
        search_text_normalized ILIKE '%' || $2 || '%'
        OR search_text_normalized % $2
      )
ORDER BY
	CASE WHEN search_text_normalized ILIKE '%' || $2 || '%' THEN 0 ELSE 1 END,
	similarity(search_text_normalized, $2) DESC,
	message_date DESC,
	id DESC
LIMIT $3
`, since.UTC(), normalizedQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("searchbot postgres search recent: %w", err)
	}
	defer rows.Close()

	out := make([]Hit, 0, limit)
	for rows.Next() {
		var (
			h          Hit
			reason     sql.NullString
			confidence sql.NullFloat64
		)

		if err := rows.Scan(
			&h.ID,
			&h.Channel,
			&h.MessageID,
			&h.MessageDate,
			&h.Text,
			&h.Link,
			&h.Keyword,
			&h.Category,
			&reason,
			&confidence,
			&h.ClassifiedAt,
		); err != nil {
			return nil, fmt.Errorf("searchbot postgres scan hit: %w", err)
		}

		h.MessageDate = h.MessageDate.UTC()
		h.ClassifiedAt = h.ClassifiedAt.UTC()

		if reason.Valid {
			h.Reason = reason.String
		}
		if confidence.Valid {
			v := confidence.Float64
			h.Confidence = &v
		}

		out = append(out, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("searchbot postgres rows: %w", err)
	}

	return out, nil
}
