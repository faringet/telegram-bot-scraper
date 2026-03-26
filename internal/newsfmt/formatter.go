package newsfmt

import (
	"fmt"
	"html"
	"strings"
)

type Formatter struct {
	maxTextRunes int
}

func NewFormatter(maxTextRunes int) *Formatter {
	return &Formatter{maxTextRunes: maxTextRunes}
}

func (f *Formatter) HitMessage(h HitView) string {
	kw := strings.TrimSpace(h.Keyword)

	reason := strings.TrimSpace(h.Reason)
	if reason == "" {
		reason = "—"
	}

	txt := truncateRunes(strings.TrimSpace(h.Text), f.maxTextRunes)
	link := strings.TrimSpace(h.Link)

	tag := ""
	if cat := strings.TrimSpace(h.Category); cat != "" {
		cat = strings.ReplaceAll(cat, " ", "_")
		tag = "#" + cat
	}

	kwEsc := html.EscapeString(kw)
	reasonEsc := html.EscapeString(reason)
	txtEsc := html.EscapeString(txt)
	linkEsc := html.EscapeString(link)
	tagEsc := html.EscapeString(tag)

	b := &strings.Builder{}
	fmt.Fprintf(
		b,
		"keyword: %s\n\n%s\n\n<b>reason: %s</b>\n\n%s",
		kwEsc, txtEsc, reasonEsc, linkEsc,
	)

	if tagEsc != "" {
		fmt.Fprintf(b, "\n\n%s", tagEsc)
	}

	return b.String()
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
