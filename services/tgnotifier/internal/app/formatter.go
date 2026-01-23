package app

import (
	"fmt"
	"strings"

	"github.com/faringet/telegram-bot-scraper/services/tgnotifier/internal/storage"
)

type Formatter struct {
	maxTextRunes int
}

func NewFormatter(maxTextRunes int) *Formatter {
	return &Formatter{maxTextRunes: maxTextRunes}
}

func (f *Formatter) HitMessage(h storage.Hit) string {
	ch := normalizeChannel(h.Channel)
	txt := truncateRunes(strings.TrimSpace(h.Text), f.maxTextRunes)

	return fmt.Sprintf(
		"%s\nkeyword: %s\n\n%s\n\n%s",
		ch,
		strings.TrimSpace(h.Keyword),
		txt,
		strings.TrimSpace(h.Link),
	)
}

func normalizeChannel(ch string) string {
	ch = strings.TrimSpace(ch)
	if ch == "" {
		return "@unknown"
	}
	if !strings.HasPrefix(ch, "@") {
		return "@" + ch
	}
	return ch
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
