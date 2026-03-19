package storage

import (
	"context"
	"time"
)

type Hit struct {
	Channel     string
	MessageID   int64
	MessageDate time.Time
	Text        string
	Link        string
	Keyword     string
}

type Store interface {
	SaveHit(ctx context.Context, h Hit) (inserted bool, err error)

	GetCheckpoint(ctx context.Context, channelUsername string) (lastMessageID int64, err error)
	SetCheckpoint(ctx context.Context, channelUsername string, lastMessageID int64) error

	Prune(ctx context.Context) error
	Close() error
}
