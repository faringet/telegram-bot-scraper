package scraper

import "strings"

type Group struct {
	Tag      string
	Keywords []string
}

type Classifier struct {
	groups []Group
}

func NewClassifier(groups []Group) *Classifier {
	return &Classifier{groups: groups}
}

func (c *Classifier) Classify(text string) (tag string, matchedKeyword string) {
	if c == nil || len(c.groups) == 0 {
		return "", ""
	}

	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return "", ""
	}

	for _, g := range c.groups {
		for _, kw := range g.Keywords {
			if kw == "" {
				continue
			}
			if strings.Contains(t, kw) {
				return g.Tag, kw
			}
		}
	}

	return "", ""
}
