package searchtext

import (
	"strings"
	"unicode"
)

func Build(parts ...string) (string, string) {
	rawParts := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		rawParts = append(rawParts, p)
	}

	raw := strings.Join(rawParts, " ")
	return raw, Normalize(raw)
}

func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")

	var b []rune
	b = make([]rune, 0, len([]rune(s)))

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b = append(b, r)
			continue
		}
		b = append(b, ' ')
	}

	return strings.Join(strings.Fields(string(b)), " ")
}
