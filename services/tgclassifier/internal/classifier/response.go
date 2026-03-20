package classifier

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

type llmResult struct {
	Category   string   `json:"category"`
	Confidence *float64 `json:"confidence,omitempty"`
	Reason     string   `json:"reason"`
}

var jsonObjectRE = regexp.MustCompile(`(?s)\{.*\}`)

func parseLLMJSON(raw string) (llmResult, error) {
	raw = strings.TrimSpace(raw)

	obj := extractFirstJSONObject(raw)
	if obj == "" {
		return llmResult{}, errors.New("no JSON object found in response")
	}

	var r llmResult
	if err := json.Unmarshal([]byte(obj), &r); err != nil {
		return llmResult{}, err
	}

	r.Category = strings.TrimSpace(r.Category)
	r.Reason = strings.TrimSpace(r.Reason)

	if r.Reason == "" {
		return llmResult{}, errors.New("reason is empty")
	}

	return r, nil
}

func extractFirstJSONObject(s string) string {
	m := jsonObjectRE.FindString(s)
	return strings.TrimSpace(m)
}

func normalizeCategory(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "hr":
		return "hr"
	case "ai_auto", "ai-auto", "ai", "automation":
		return "ai_auto"
	case "ecommerce", "e-commerce", "e_commerce", "ecom":
		return "ecommerce"
	case "other", "misc":
		return "other"
	default:
		return ""
	}
}
