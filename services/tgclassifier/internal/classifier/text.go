package classifier

import "strings"

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
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

func safeSnippet(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}

	r := []rune(s)
	if len(r) <= max {
		return s
	}

	return string(r[:max]) + "…"
}
