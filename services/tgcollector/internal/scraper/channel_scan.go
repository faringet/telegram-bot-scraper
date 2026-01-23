package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gotd/td/tg"

	"github.com/faringet/telegram-bot-scraper/services/tgcollector/internal/storage"
)

func (s *Scraper) scanChannel(ctx context.Context, api *tg.Client, username string, keywords []string) error {
	peer, linkBase, err := ResolvePublicChannel(ctx, api, username)
	if err != nil {
		return err
	}

	var cutoff time.Time
	if s.cfg.Lookback > 0 {
		cutoff = time.Now().Add(-s.cfg.Lookback)
	}

	lastID, err := s.store.GetCheckpoint(ctx, username)
	if err != nil {
		return fmt.Errorf("get checkpoint @%s: %w", username, err)
	}

	//todoo в конфиг
	const batchLimit = 100
	scanned := 0
	hitsNew := 0

	offsetID := 0
	addOffset := 0
	maxSeen := lastID

	stopReason := "max_scan"

	for scanned < s.cfg.PerChannelMaxScan {
		if err := sleepCtx(ctx, s.cfg.MinDelay); err != nil {
			return err
		}

		res, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:      peer,
			Limit:     batchLimit,
			OffsetID:  offsetID,
			AddOffset: addOffset,
		})
		if err != nil {
			return fmt.Errorf("history(@%s, offset=%d): %w", username, offsetID, err)
		}

		msgs := extractMessages(res)
		if len(msgs) == 0 {
			stopReason = "no_messages"
			break
		}

		oldest := 0

		for _, mc := range msgs {
			m, ok := mc.(*tg.Message)
			if !ok {
				continue
			}

			scanned++
			if oldest == 0 || m.ID < oldest {
				oldest = m.ID
			}
			if m.ID > maxSeen {
				maxSeen = m.ID
			}

			if lastID > 0 && m.ID <= lastID {
				stopReason = "reached_last_id"
				scanned = s.cfg.PerChannelMaxScan
				break
			}

			msgTime := time.Unix(int64(m.Date), 0)
			if !cutoff.IsZero() && msgTime.Before(cutoff) {
				stopReason = "reached_cutoff"
				scanned = s.cfg.PerChannelMaxScan
				break
			}

			text := m.Message
			kw, ok := containsAnyKeyword(text, keywords)
			if !ok {
				continue
			}

			h := storage.Hit{
				Channel:     "@" + username,
				MessageID:   m.ID,
				MessageDate: msgTime.UTC(),
				Text:        text,
				Link:        fmt.Sprintf("%s/%d", linkBase, m.ID),
				Keyword:     kw,
			}

			inserted, err := s.store.SaveHit(ctx, h)
			if err != nil {
				return fmt.Errorf("save hit: %w", err)
			}
			if inserted {
				hitsNew++
			}
		}

		if oldest == 0 {
			stopReason = "no_ids"
			break
		}

		if offsetID == oldest && addOffset == -1 {
			stopReason = "stuck_offset"
			break
		}
		offsetID = oldest
		addOffset = -1
	}

	if maxSeen > lastID {
		if err := s.store.SetCheckpoint(ctx, username, maxSeen); err != nil {
			return fmt.Errorf("set checkpoint @%s: %w", username, err)
		}
	}

	s.log.Info("scan channel done",
		slog.String("channel", "@"+username),
		slog.Int("scanned", scanned),
		slog.Int("hits_new", hitsNew),
		slog.Int("new_last_id", maxSeen),
		slog.String("stop_reason", stopReason),
	)

	return nil
}
