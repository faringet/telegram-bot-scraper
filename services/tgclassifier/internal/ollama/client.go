// services/tgclassifier/internal/ollama/client.go
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client implements classifier.OllamaClient (Classify method).
type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

type Config struct {
	BaseURL string        // e.g. http://127.0.0.1:11434
	Timeout time.Duration // request timeout
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("ollama: base_url is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		timeout: cfg.Timeout,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
	}, nil
}

// Classify calls Ollama /api/generate with stream=false and returns the "response" text.
// The worker expects the model to output strict JSON (one line), but it can also extract {..} from noisy output.
func (c *Client) Classify(ctx context.Context, model string, prompt string) (string, error) {
	if c == nil || c.httpClient == nil {
		return "", errors.New("ollama: client is nil")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", errors.New("ollama: model is required")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("ollama: prompt is required")
	}

	reqBody := generateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
		// You can tune these later from config if you want.
		Options: map[string]any{
			"temperature": 0.2,
			"top_p":       0.9,
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama marshal request: %w", err)
	}

	// Ensure context has deadline.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	url := c.baseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("ollama create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readAllLimit(resp.Body, 4<<20) // 4MB
	if err != nil {
		return "", fmt.Errorf("ollama read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Ollama often returns {"error":"..."}.
		msg := strings.TrimSpace(string(body))
		return "", fmt.Errorf("ollama http %d: %s", resp.StatusCode, msg)
	}

	var out generateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("ollama unmarshal response: %w", err)
	}

	return strings.TrimSpace(out.Response), nil
}

// --- Ollama /api/generate schema ---

type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

type generateResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason,omitempty"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
	Error              string `json:"error,omitempty"`
}

// readAllLimit prevents OOM on unexpected large responses.
func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	lr := &io.LimitedReader{R: r, N: limit}
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	// If N==0, we hit the limit exactly; not necessarily an error but suspicious.
	return b, nil
}
