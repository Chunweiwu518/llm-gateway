package relay

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// ErrNoChannel means no enabled channel serves the requested model.
var ErrNoChannel = errors.New("no channel available for model")

// Router picks channels for a model and fails over between them.
type Router struct {
	db       *gorm.DB
	timeout  time.Duration
	adapters map[string]Adapter
}

// NewRouter returns a router with OpenAI and Anthropic adapters.
// timeout applies to each upstream attempt.
func NewRouter(db *gorm.DB, timeout time.Duration) *Router {
	client := &http.Client{}
	return &Router{db: db, timeout: timeout, adapters: map[string]Adapter{
		store.ChannelOpenAI:    OpenAIAdapter{Client: client},
		store.ChannelAnthropic: AnthropicAdapter{Client: client},
	}}
}

// Candidates returns enabled channels serving model, ordered by priority.
func (r *Router) Candidates(model string) ([]store.Channel, error) {
	var all []store.Channel
	if err := r.db.Where("enabled = ?", true).Find(&all).Error; err != nil {
		return nil, err
	}
	var out []store.Channel
	for _, ch := range all {
		if slices.Contains(ch.Models, model) {
			out = append(out, ch)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Dispatch tries each candidate channel in order. It returns the response,
// the channel that answered (or the last one tried), and how many times it
// moved on to another channel.
func (r *Router) Dispatch(ctx context.Context, req ChatRequest) (ChatResponse, store.Channel, int, error) {
	chans, err := r.Candidates(req.Model)
	if err != nil {
		return ChatResponse{}, store.Channel{}, 0, err
	}
	if len(chans) == 0 {
		return ChatResponse{}, store.Channel{}, 0, ErrNoChannel
	}
	var lastErr error
	for i, ch := range chans {
		resp, err := r.try(ctx, ch, req)
		if err == nil {
			return resp, ch, i, nil
		}
		lastErr = err
		var ue *UpstreamError
		if !errors.As(err, &ue) || !ue.Retryable || i == len(chans)-1 {
			return ChatResponse{}, ch, i, err
		}
		log.Printf("channel %q failed, trying next: %v", ch.Name, err)
	}
	return ChatResponse{}, chans[len(chans)-1], len(chans) - 1, lastErr
}

func (r *Router) try(ctx context.Context, ch store.Channel, req ChatRequest) (ChatResponse, error) {
	adapter, ok := r.adapters[ch.Type]
	if !ok {
		return ChatResponse{}, &UpstreamError{Status: http.StatusBadGateway, Retryable: true, Body: "unknown channel type " + ch.Type}
	}
	model := req.Model
	if mapped := ch.ModelMap.Data()[req.Model]; mapped != "" {
		model = mapped
	}
	actx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	resp, err := adapter.Do(actx, ch, model, req)
	if err != nil {
		return resp, err
	}
	resp.Model = req.Model
	if resp.Object == "" {
		resp.Object = "chat.completion"
	}
	return resp, nil
}
