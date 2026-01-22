// services/tgbot/internal/app/app.go
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	tgbotcfg "github.com/faringet/telegram-bot-scraper/services/tgbot/config"
	"github.com/faringet/telegram-bot-scraper/services/tgbot/internal/mtproto"
)

type App struct {
	cfg    *tgbotcfg.TGBot
	log    *slog.Logger
	client *mtproto.Client
}

func New(cfg *tgbotcfg.TGBot, log *slog.Logger) (*App, error) {
	if cfg == nil {
		return nil, errors.New("app: config is nil")
	}
	if log == nil {
		log = slog.Default()
	}

	client, err := mtproto.New(cfg.MTProto, log)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:    cfg,
		log:    log.With(slog.String("component", "app")),
		client: client,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.log.Info("app run started")

	return a.client.WithClient(ctx, func(ctx context.Context, td *telegram.Client) error {
		if err := a.logSelf(ctx, td); err != nil {
			return err
		}

		if len(a.cfg.Scrape.Channels) == 0 {
			return errors.New("scrape config: channels is required")
		}
		if len(a.cfg.Scrape.Keywords) == 0 {
			return errors.New("scrape config: keywords is required")
		}

		// На один запуск ограничим “сколько сообщений максимум на канал”.
		const perChannelMaxToScan = 500

		// Прогоняем все каналы из конфига (аккуратно, с паузой).
		if err := a.scanAllChannelsOnce(ctx, td, perChannelMaxToScan); err != nil {
			return err
		}

		<-ctx.Done()
		return ctx.Err()
	})
}

// scanAllChannelsOnce сканирует все каналы из конфига последовательно.
// Последовательно = максимально безопасно и “по-юзерски”.
func (a *App) scanAllChannelsOnce(ctx context.Context, td *telegram.Client, perChannelMaxToScan int) error {
	// Пауза между каналами (чтобы это выглядело не как скрипт-сканер).
	// Можно сделать больше, если хочешь.
	betweenChannelsDelay := a.cfg.MTProto.RateLimit.MinDelay
	if betweenChannelsDelay <= 0 {
		betweenChannelsDelay = 2 * time.Second
	}

	for i, ch := range a.cfg.Scrape.Channels {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}

		a.log.Info("channel scan queued",
			slog.Int("i", i),
			slog.String("channel", ch),
		)

		if err := a.scanChannelOnce(ctx, td, ch, a.cfg.Scrape.Keywords, a.cfg.Scrape.Lookback, perChannelMaxToScan); err != nil {
			// По умолчанию — фейлим запуск, чтобы не “молчать” на ошибках.
			// Если хочешь “пропускать и продолжать”, скажи — сделаем режим soft-fail.
			return err
		}

		// Пауза между каналами (кроме последнего).
		if i < len(a.cfg.Scrape.Channels)-1 {
			if err := sleepCtx(ctx, betweenChannelsDelay); err != nil {
				return err
			}
		}
	}

	a.log.Info("all channels scanned")
	return nil
}

func (a *App) logSelf(ctx context.Context, td *telegram.Client) error {
	u, err := td.Self(ctx)
	if err != nil {
		return fmt.Errorf("telegram self: %w", err)
	}
	if u == nil {
		return errors.New("telegram self: nil user")
	}

	a.log.Info("authorized as user",
		slog.Int64("id", u.ID),
		slog.String("username", u.Username),
		slog.String("first_name", u.FirstName),
		slog.String("last_name", u.LastName),
		slog.String("phone", u.Phone),
		slog.Bool("bot", u.Bot),
		slog.Bool("verified", u.Verified),
	)
	return nil
}

// -------------------- scan (single channel, аккуратно) --------------------

func (a *App) scanChannelOnce(
	ctx context.Context,
	td *telegram.Client,
	channelRef string,
	keywords []string,
	lookback time.Duration,
	maxToScan int,
) error {
	api := tg.NewClient(td)

	if err := os.MkdirAll("data", 0o755); err != nil {
		return fmt.Errorf("mkdir data: %w", err)
	}

	kw := normalizeKeywords(keywords)
	if len(kw) == 0 {
		return errors.New("scan: keywords empty after normalization")
	}

	username := normalizeUsername(channelRef)
	if username == "" {
		return fmt.Errorf("scan: channel must be @username or t.me link (for now), got %q", channelRef)
	}

	peer, linkBase, err := resolvePublicChannel(ctx, api, username)
	if err != nil {
		return err
	}

	cutoff := time.Time{}
	if lookback > 0 {
		cutoff = time.Now().Add(-lookback)
	}

	st, err := readState("data/state.json")
	if err != nil {
		return err
	}
	lastID := st.LastMessageID[username]

	minDelay := a.cfg.MTProto.RateLimit.MinDelay
	if minDelay <= 0 {
		minDelay = 800 * time.Millisecond
	}

	a.log.Info("scan start",
		slog.String("channel", "@"+username),
		slog.Any("keywords", kw),
		slog.Duration("lookback", lookback),
		slog.String("cutoff", formatTimeOrEmpty(cutoff)),
		slog.Int("last_id", lastID),
		slog.Int("max_to_scan", maxToScan),
		slog.Duration("min_delay", minDelay),
	)

	const batchLimit = 100
	scanned := 0
	hits := 0
	offsetID := 0
	maxSeen := lastID
	stopReason := "max_to_scan"

	for scanned < maxToScan {
		if err := sleepCtx(ctx, minDelay); err != nil {
			return err
		}

		res, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:       peer,
			Limit:      batchLimit,
			OffsetID:   offsetID,
			OffsetDate: 0,
			AddOffset:  0,
			MaxID:      0,
			MinID:      0,
			Hash:       0,
		})
		if err != nil {
			return fmt.Errorf("messages.getHistory(@%s, offset_id=%d): %w", username, offsetID, err)
		}

		msgs := extractMessages(res)
		if len(msgs) == 0 {
			stopReason = "no_messages"
			break
		}

		oldestInBatch := 0

		for _, mc := range msgs {
			m, ok := mc.(*tg.Message)
			if !ok {
				continue
			}

			scanned++
			if oldestInBatch == 0 || m.ID < oldestInBatch {
				oldestInBatch = m.ID
			}
			if m.ID > maxSeen {
				maxSeen = m.ID
			}

			if lastID > 0 && m.ID <= lastID {
				stopReason = "reached_last_id"
				scanned = maxToScan
				break
			}

			if !cutoff.IsZero() {
				msgTime := time.Unix(int64(m.Date), 0)
				if msgTime.Before(cutoff) {
					stopReason = "reached_cutoff"
					scanned = maxToScan
					break
				}
			}

			text := m.Message
			if text == "" {
				continue
			}
			if !containsAnyKeyword(text, kw) {
				continue
			}

			hit := Hit{
				TS:        time.Now().UTC().Format(time.RFC3339Nano),
				Channel:   "@" + username,
				MessageID: m.ID,
				Date:      time.Unix(int64(m.Date), 0).UTC().Format(time.RFC3339),
				Text:      text,
				Link:      fmt.Sprintf("%s/%d", linkBase, m.ID),
			}
			if err := appendNDJSON("data/hits.ndjson", hit); err != nil {
				return err
			}
			hits++
		}

		if oldestInBatch == 0 {
			stopReason = "no_message_ids"
			break
		}
		if offsetID == oldestInBatch {
			stopReason = "stuck_offset"
			break
		}
		offsetID = oldestInBatch
	}

	if maxSeen > lastID {
		if st.LastMessageID == nil {
			st.LastMessageID = map[string]int{}
		}
		st.LastMessageID[username] = maxSeen
		if err := writeStateAtomic("data/state.json", st); err != nil {
			return err
		}
	}

	a.log.Info("scan done",
		slog.String("channel", "@"+username),
		slog.Int("scanned", minInt(scanned, maxToScan)),
		slog.Int("hits", hits),
		slog.Int("new_last_id", st.LastMessageID[username]),
		slog.String("stop_reason", stopReason),
	)

	return nil
}

func normalizeKeywords(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, k := range in {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

func containsAnyKeyword(text string, keywords []string) bool {
	s := strings.ToLower(text)
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func normalizeUsername(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "@") {
		return strings.TrimPrefix(s, "@")
	}
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if strings.HasPrefix(s, "t.me/") {
		return strings.TrimPrefix(s, "t.me/")
	}
	if strings.HasPrefix(s, "telegram.me/") {
		return strings.TrimPrefix(s, "telegram.me/")
	}
	return ""
}

func resolvePublicChannel(ctx context.Context, api *tg.Client, username string) (tg.InputPeerClass, string, error) {
	r, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return nil, "", fmt.Errorf("contacts.resolveUsername(@%s): %w", username, err)
	}

	for _, cc := range r.Chats {
		ch, ok := cc.(*tg.Channel)
		if !ok {
			continue
		}
		return &tg.InputPeerChannel{
			ChannelID:  ch.ID,
			AccessHash: ch.AccessHash,
		}, "https://t.me/" + username, nil
	}

	return nil, "", fmt.Errorf("resolve @%s: channel not found in response", username)
}

func extractMessages(res tg.MessagesMessagesClass) []tg.MessageClass {
	switch v := res.(type) {
	case *tg.MessagesMessages:
		return v.Messages
	case *tg.MessagesMessagesSlice:
		return v.Messages
	case *tg.MessagesChannelMessages:
		return v.Messages
	default:
		return nil
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func formatTimeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// -------------------- persistence: hits + state --------------------

type Hit struct {
	TS        string `json:"ts"`
	Channel   string `json:"channel"`
	MessageID int    `json:"message_id"`
	Date      string `json:"date"`
	Text      string `json:"text"`
	Link      string `json:"link"`
}

func appendNDJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open hits file: %w", err)
	}
	defer f.Close()

	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal hit: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write hit: %w", err)
	}
	return nil
}

type State struct {
	LastMessageID map[string]int `json:"last_message_id"` // ключ: username (без @)
}

func readState(path string) (State, error) {
	var st State
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{LastMessageID: map[string]int{}}, nil
		}
		return st, fmt.Errorf("read state: %w", err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return State{LastMessageID: map[string]int{}}, nil
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, fmt.Errorf("unmarshal state: %w", err)
	}
	if st.LastMessageID == nil {
		st.LastMessageID = map[string]int{}
	}
	return st, nil
}

func writeStateAtomic(path string, st State) error {
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, "."+filepath.Base(path)+".tmp")

	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
