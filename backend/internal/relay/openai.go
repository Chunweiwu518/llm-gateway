package relay

import (
	"context"
	"net/http"
	"strings"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Adapter calls one upstream type with an OpenAI-format request.
type Adapter interface {
	Do(ctx context.Context, ch store.Channel, upstreamModel string, req ChatRequest) (ChatResponse, error)
}

// OpenAIAdapter forwards to an OpenAI-compatible /v1/chat/completions.
type OpenAIAdapter struct {
	Client *http.Client
}

// Do implements Adapter.
func (a OpenAIAdapter) Do(ctx context.Context, ch store.Channel, upstreamModel string, req ChatRequest) (ChatResponse, error) {
	req.Model = upstreamModel
	var out ChatResponse
	url := strings.TrimRight(ch.BaseURL, "/") + "/v1/chat/completions"
	err := postJSON(ctx, a.Client, url, map[string]string{"Authorization": "Bearer " + ch.APIKey}, req, &out)
	return out, err
}
