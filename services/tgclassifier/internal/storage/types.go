package storage

import (
	"context"
	"errors"
	"time"
)

var ErrClaimLost = errors.New("storage: claim lost")

type Hit struct {
	ID            int64
	Channel       string
	MessageID     int64
	MessageDate   time.Time
	Text          string
	Link          string
	Keyword       string
	CreatedAt     time.Time
	DeliveredAt   *time.Time
	Category      *string
	ClassifiedAt  *time.Time
	LLMModel      *string
	LLMConfidence *float64
	LLMReason     *string
}

type Classification struct {
	Category     string
	LLMModel     string
	Confidence   *float64
	Reason       *string
	ClassifiedAt time.Time
}

type ClaimOptions struct {
	Limit           int
	WorkerID        string
	Lease           time.Duration
	OnlyUndelivered bool
}

type Store interface {
	ClaimUnclassifiedHits(ctx context.Context, opts ClaimOptions) ([]Hit, error)
	UpdateClassification(ctx context.Context, id int64, workerID string, c Classification) error
	ReleaseProcessing(ctx context.Context, id int64, workerID string) error
	Close() error
}
