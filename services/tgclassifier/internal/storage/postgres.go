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
		return nil, errors.New("postgres storage: db is nil")
	}
	return &Postgres{db: db}, nil
}

func (s *Postgres) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Postgres) ClaimUnclassifiedHits(ctx context.Context, opts ClaimOptions) ([]Hit, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("postgres storage: db is nil")
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.WorkerID == "" {
		return nil, errors.New("postgres storage: workerID is required")
	}
	if opts.Lease <= 0 {
		opts.Lease = 2 * time.Minute
	}

	now := time.Now().UTC()
	until := now.Add(opts.Lease)

	rows, err := s.db.QueryContext(ctx, `
WITH candidates AS (
	SELECT h.id
	FROM hits h
	WHERE
		(
			h.category IS NULL
			OR h.llm_reason IS NULL
			OR BTRIM(h.llm_reason) = ''
		)
		AND (h.processing_until IS NULL OR h.processing_until < $1)
		AND (NOT $2 OR h.delivered_at IS NULL)
	ORDER BY h.message_date DESC
	FOR UPDATE SKIP LOCKED
	LIMIT $3
),
claimed AS (
	UPDATE hits h
	SET processing_by = $4,
	    processing_until = $5
	FROM candidates c
	WHERE h.id = c.id
	RETURNING
		h.id,
		h.channel,
		h.message_id,
		h.message_date,
		h.text,
		h.link,
		h.keyword,
		h.created_at,
		h.delivered_at,
		h.category,
		h.classified_at,
		h.llm_model,
		h.llm_confidence,
		h.llm_reason
)
SELECT
	id,
	channel,
	message_id,
	message_date,
	text,
	link,
	keyword,
	created_at,
	delivered_at,
	category,
	classified_at,
	llm_model,
	llm_confidence,
	llm_reason
FROM claimed
ORDER BY message_date DESC
`, now, opts.OnlyUndelivered, opts.Limit, opts.WorkerID, until)
	if err != nil {
		return nil, fmt.Errorf("postgres claim unclassified hits: %w", err)
	}
	defer rows.Close()

	hits, err := scanPostgresHits(rows)
	if err != nil {
		return nil, err
	}

	return hits, nil
}

func (s *Postgres) UpdateClassification(ctx context.Context, id int64, workerID string, c Classification) error {
	if s == nil || s.db == nil {
		return errors.New("postgres storage: db is nil")
	}
	if id <= 0 {
		return errors.New("postgres storage: id must be > 0")
	}
	if workerID == "" {
		return errors.New("postgres storage: workerID is required")
	}
	if c.Category == "" {
		return errors.New("postgres storage: classification.category is required")
	}
	if c.LLMModel == "" {
		c.LLMModel = "unknown"
	}
	if c.ClassifiedAt.IsZero() {
		c.ClassifiedAt = time.Now().UTC()
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE hits
SET category = $1,
    classified_at = $2,
    llm_model = $3,
    llm_confidence = $4,
    llm_reason = $5,
    processing_by = NULL,
    processing_until = NULL
WHERE id = $6
  AND processing_by = $7
`, c.Category, c.ClassifiedAt.UTC(), c.LLMModel, c.Confidence, c.Reason, id, workerID)
	if err != nil {
		return fmt.Errorf("postgres update classification: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrClaimLost
	}

	return nil
}

func (s *Postgres) ReleaseProcessing(ctx context.Context, id int64, workerID string) error {
	if s == nil || s.db == nil {
		return errors.New("postgres storage: db is nil")
	}
	if id <= 0 {
		return errors.New("postgres storage: id must be > 0")
	}
	if workerID == "" {
		return errors.New("postgres storage: workerID is required")
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE hits
SET processing_by = NULL,
    processing_until = NULL
WHERE id = $1
  AND processing_by = $2
`, id, workerID)
	if err != nil {
		return fmt.Errorf("postgres release processing: %w", err)
	}

	return nil
}

func scanPostgresHits(rows *sql.Rows) ([]Hit, error) {
	out := make([]Hit, 0, 16)

	for rows.Next() {
		var (
			h             Hit
			deliveredAt   sql.NullTime
			category      sql.NullString
			classifiedAt  sql.NullTime
			llmModel      sql.NullString
			llmConfidence sql.NullFloat64
			llmReason     sql.NullString
		)

		if err := rows.Scan(
			&h.ID,
			&h.Channel,
			&h.MessageID,
			&h.MessageDate,
			&h.Text,
			&h.Link,
			&h.Keyword,
			&h.CreatedAt,
			&deliveredAt,
			&category,
			&classifiedAt,
			&llmModel,
			&llmConfidence,
			&llmReason,
		); err != nil {
			return nil, fmt.Errorf("postgres scan hit: %w", err)
		}

		h.MessageDate = h.MessageDate.UTC()
		h.CreatedAt = h.CreatedAt.UTC()

		if deliveredAt.Valid {
			t := deliveredAt.Time.UTC()
			h.DeliveredAt = &t
		}
		if category.Valid {
			v := category.String
			h.Category = &v
		}
		if classifiedAt.Valid {
			t := classifiedAt.Time.UTC()
			h.ClassifiedAt = &t
		}
		if llmModel.Valid {
			v := llmModel.String
			h.LLMModel = &v
		}
		if llmConfidence.Valid {
			v := llmConfidence.Float64
			h.LLMConfidence = &v
		}
		if llmReason.Valid {
			v := llmReason.String
			h.LLMReason = &v
		}

		out = append(out, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres hit rows: %w", err)
	}

	return out, nil
}
