// Package relay converts OpenAI-format requests for each upstream type and
// dispatches them across channels with priority failover.
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultMaxTokens is used when the client does not send max_tokens.
const DefaultMaxTokens = 1024

// Message is one chat message. Only plain-text content is supported.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the OpenAI chat completions request subset the gateway accepts.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Usage is token accounting in OpenAI format.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Choice is one completion choice.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// ChatResponse is the OpenAI chat completions response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// UpstreamError describes a failed upstream call. Retryable errors trigger failover.
type UpstreamError struct {
	Status    int
	Retryable bool
	Body      string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream status %d (retryable=%v): %s", e.Status, e.Retryable, e.Body)
}

// maxUpstreamBody caps how much of an upstream response is read.
const maxUpstreamBody = 4 << 20

// postJSON sends body to url and decodes a 2xx response into out.
// Network errors, timeouts, 429 and 5xx are retryable; other 4xx are not.
func postJSON(ctx context.Context, client *http.Client, url string, hdr map[string]string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return &UpstreamError{Status: http.StatusBadGateway, Retryable: true, Body: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return &UpstreamError{Status: http.StatusBadGateway, Retryable: true, Body: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBody))
	if err != nil {
		return &UpstreamError{Status: http.StatusBadGateway, Retryable: true, Body: err.Error()}
	}
	if resp.StatusCode >= 300 {
		retry := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return &UpstreamError{Status: resp.StatusCode, Retryable: retry, Body: truncate(string(data), 500)}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &UpstreamError{Status: http.StatusBadGateway, Retryable: true, Body: "invalid upstream JSON"}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
