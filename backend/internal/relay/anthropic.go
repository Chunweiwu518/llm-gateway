package relay

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// AnthropicVersion is sent in the anthropic-version header.
const AnthropicVersion = "2023-06-01"

type anthropicRequest struct {
	Model       string    `json:"model"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature *float64  `json:"temperature,omitempty"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// toAnthropic moves system messages into the system field and fills max_tokens.
func toAnthropic(req ChatRequest, model string) anthropicRequest {
	out := anthropicRequest{Model: model, MaxTokens: DefaultMaxTokens, Temperature: req.Temperature}
	if req.MaxTokens != nil {
		out.MaxTokens = *req.MaxTokens
	}
	var system []string
	for _, m := range req.Messages {
		if m.Role == "system" {
			system = append(system, m.Content)
			continue
		}
		out.Messages = append(out.Messages, m)
	}
	out.System = strings.Join(system, "\n")
	return out
}

var finishReasons = map[string]string{"end_turn": "stop", "stop_sequence": "stop", "max_tokens": "length"}

func fromAnthropic(r anthropicResponse, model string) ChatResponse {
	var text strings.Builder
	for _, c := range r.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	finish := finishReasons[r.StopReason]
	if finish == "" {
		finish = "stop"
	}
	in, out := r.Usage.InputTokens, r.Usage.OutputTokens
	return ChatResponse{
		ID: r.ID, Object: "chat.completion", Created: time.Now().Unix(), Model: model,
		Choices: []Choice{{Message: Message{Role: "assistant", Content: text.String()}, FinishReason: finish}},
		Usage:   Usage{PromptTokens: in, CompletionTokens: out, TotalTokens: in + out},
	}
}

// AnthropicAdapter converts to and from the Anthropic Messages API.
type AnthropicAdapter struct {
	Client *http.Client
}

// Do implements Adapter.
func (a AnthropicAdapter) Do(ctx context.Context, ch store.Channel, upstreamModel string, req ChatRequest) (ChatResponse, error) {
	var out anthropicResponse
	url := strings.TrimRight(ch.BaseURL, "/") + "/v1/messages"
	hdr := map[string]string{"x-api-key": ch.APIKey, "anthropic-version": AnthropicVersion}
	if err := postJSON(ctx, a.Client, url, hdr, toAnthropic(req, upstreamModel), &out); err != nil {
		return ChatResponse{}, err
	}
	return fromAnthropic(out, req.Model), nil
}
