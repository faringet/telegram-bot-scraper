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

type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

type Config struct {
	BaseURL string
	Timeout time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
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
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		timeout: cfg.Timeout,
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
	}, nil
}

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
		Options: map[string]any{
			"temperature": 0.2,
			"top_p":       0.9,
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama marshal request: %w", err)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("ollama create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readAllLimit(resp.Body, 4<<20)
	if err != nil {
		return "", fmt.Errorf("ollama read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", parseHTTPError(resp.StatusCode, body)
	}

	var out generateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("ollama unmarshal response: %w", err)
	}

	if strings.TrimSpace(out.Error) != "" {
		return "", fmt.Errorf("ollama response error: %s", strings.TrimSpace(out.Error))
	}

	return strings.TrimSpace(out.Response), nil
}

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

type errorResponse struct {
	Error string `json:"error"`
}

func parseHTTPError(statusCode int, body []byte) error {
	var er errorResponse
	if err := json.Unmarshal(body, &er); err == nil {
		if msg := strings.TrimSpace(er.Error); msg != "" {
			return fmt.Errorf("ollama http %d: %s", statusCode, msg)
		}
	}

	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(statusCode)
	}
	return fmt.Errorf("ollama http %d: %s", statusCode, msg)
}

func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("readAllLimit: limit must be > 0")
	}

	lr := &io.LimitedReader{R: r, N: limit + 1}
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("response exceeds limit of %d bytes", limit)
	}
	return b, nil
}
