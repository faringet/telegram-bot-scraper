package scraper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"
)

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

func containsAnyKeyword(text string, keywords []string) (string, bool) {
	s := strings.ToLower(text)
	for _, k := range keywords {
		if k != "" && strings.Contains(s, k) {
			return k, true
		}
	}
	return "", false
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

func ResolvePublicChannel(ctx context.Context, api *tg.Client, username string) (tg.InputPeerClass, string, error) {
	r, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, "", fmt.Errorf("resolve @%s: %w", username, err)
	}

	for _, cc := range r.Chats {
		ch, ok := cc.(*tg.Channel)
		if !ok {
			continue
		}
		return &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}, "https://t.me/" + username, nil
	}

	return nil, "", fmt.Errorf("resolve @%s: channel not found", username)
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
