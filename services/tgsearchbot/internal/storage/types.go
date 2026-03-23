package storage

import (
	"context"
	"time"
)

type Hit struct {
	ID           int64
	Channel      string
	MessageID    int64
	MessageDate  time.Time
	Text         string
	Link         string
	Keyword      string
	Category     string
	Reason       string
	Confidence   *float64
	ClassifiedAt time.Time
}

type Store interface {
	SearchRecent(ctx context.Context, normalizedQuery string, since time.Time, limit int) ([]Hit, error)
	Close() error
}
