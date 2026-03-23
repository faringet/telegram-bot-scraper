package newsfmt

import "time"

type HitView struct {
	ID          int64
	Channel     string
	MessageID   int64
	MessageDate time.Time
	Text        string
	Link        string
	Keyword     string
	Category    string
	Reason      string
	Confidence  *float64
}
