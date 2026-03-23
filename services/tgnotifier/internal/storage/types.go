package storage

import (
	"context"
	"database/sql"
	"time"
)

type Hit struct {
	ID            int64
	Channel       string
	MessageID     int64
	MessageDate   time.Time
	Text          string
	Link          string
	Keyword       string
	DeliveredAt   sql.NullTime
	Category      sql.NullString
	ClassifiedAt  sql.NullTime
	LLMModel      sql.NullString
	LLMConfidence sql.NullFloat64
	LLMReason     sql.NullString
}

type Store interface {
	ListUndeliveredBefore(ctx context.Context, limit int, classifiedBefore time.Time) ([]Hit, error)
	MarkDelivered(ctx context.Context, ids []int64) error
	Close() error
}
