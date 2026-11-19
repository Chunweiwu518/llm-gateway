package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

func intp(n int) *int { return &n }

func TestAnthropicRequestConversion(t *testing.T) {
	req := ChatRequest{Model: "gpt-4o-mini", Messages: []Message{
		{Role: "system", Content: "be brief"}, {Role: "user", Content: "hi"},
	}}
	got := toAnthropic(req, "claude-haiku-4-5")
	if got.System != "be brief" || len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Fatalf("system not extracted: %+v", got)
	}
	if got.MaxTokens != DefaultMaxTokens || got.Model != "claude-haiku-4-5" {
		t.Fatalf("defaults wrong: %+v", got)
	}
	req.MaxTokens = intp(50)
	if toAnthropic(req, "m").MaxTokens != 50 {
		t.Fatal("max_tokens not kept")
	}
}

func TestAnthropicResponseConversion(t *testing.T) {
	in := anthropicResponse{ID: "msg_1", StopReason: "max_tokens",
		Content: []anthropicContent{{Type: "text", Text: "a"}, {Type: "text", Text: "b"}}}
	in.Usage.InputTokens, in.Usage.OutputTokens = 10, 4
	got := fromAnthropic(in, "gpt-4o-mini")
	c := got.Choices[0]
	if c.Message.Content != "ab" || c.FinishReason != "length" || got.Model != "gpt-4o-mini" {
		t.Fatalf("bad conversion %+v", got)
	}
	if got.Usage.PromptTokens != 10 || got.Usage.CompletionTokens != 4 || got.Usage.TotalTokens != 14 {
		t.Fatalf("bad usage %+v", got.Usage)
	}
}

// fakeUpstream answers like OpenAI with the given status and counts hits.
func fakeUpstream(t *testing.T, status int, delay time.Duration, hits *int32) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		body, _ := io.ReadAll(r.Body)
		var req ChatRequest
		_ = json.Unmarshal(body, &req)
		time.Sleep(delay)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(ChatResponse{Model: req.Model, Choices: []Choice{{Message: Message{Role: "assistant", Content: "from " + req.Model}}},
			Usage: Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}})
	}))
	t.Cleanup(s.Close)
	return s
}

func newRouter(t *testing.T, upstreams ...*httptest.Server) *Router {
	db := store.OpenTest(t)
	for i, u := range upstreams {
		db.Create(&store.Channel{Name: "ch" + string(rune('A'+i)), Type: store.ChannelOpenAI, BaseURL: u.URL, APIKey: "k",
			Models: datatypes.JSONSlice[string]{"gpt-4o-mini"}, Priority: i + 1, Enabled: true,
			ModelMap: datatypes.NewJSONType(map[string]string{})})
	}
	return NewRouter(db, 300*time.Millisecond)
}

var basic = ChatRequest{Model: "gpt-4o-mini", Messages: []Message{{Role: "user", Content: "hi"}}}

func TestFailoverOn500(t *testing.T) {
	var a, b int32
	r := newRouter(t, fakeUpstream(t, 500, 0, &a), fakeUpstream(t, 200, 0, &b))
	resp, ch, failovers, err := r.Dispatch(context.Background(), basic)
	if err != nil || ch.Name != "chB" || failovers != 1 || resp.Usage.TotalTokens != 5 {
		t.Fatalf("err=%v ch=%s failovers=%d", err, ch.Name, failovers)
	}
}

func TestFailoverOnTimeout(t *testing.T) {
	var a, b int32
	r := newRouter(t, fakeUpstream(t, 200, time.Second, &a), fakeUpstream(t, 200, 0, &b))
	_, ch, failovers, err := r.Dispatch(context.Background(), basic)
	if err != nil || ch.Name != "chB" || failovers != 1 {
		t.Fatalf("err=%v ch=%s failovers=%d", err, ch.Name, failovers)
	}
}

func TestNoFailoverOn400(t *testing.T) {
	var a, b int32
	r := newRouter(t, fakeUpstream(t, 400, 0, &a), fakeUpstream(t, 200, 0, &b))
	_, _, _, err := r.Dispatch(context.Background(), basic)
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != 400 || ue.Retryable {
		t.Fatalf("want non-retryable 400, got %v", err)
	}
	if atomic.LoadInt32(&b) != 0 {
		t.Fatal("second channel must not be called on a 4xx")
	}
}

func TestAllChannelsDown(t *testing.T) {
	var a, b int32
	r := newRouter(t, fakeUpstream(t, 503, 0, &a), fakeUpstream(t, 500, 0, &b))
	_, _, failovers, err := r.Dispatch(context.Background(), basic)
	var ue *UpstreamError
	if !errors.As(err, &ue) || !ue.Retryable || failovers != 1 || a != 1 || b != 1 {
		t.Fatalf("err=%v failovers=%d hits=%d,%d", err, failovers, a, b)
	}
}

func TestNoChannelForModel(t *testing.T) {
	r := newRouter(t)
	if _, _, _, err := r.Dispatch(context.Background(), basic); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("want ErrNoChannel got %v", err)
	}
}

func TestModelMapApplied(t *testing.T) {
	var hits int32
	up := fakeUpstream(t, 200, 0, &hits)
	r := newRouter(t)
	r.db.Create(&store.Channel{Name: "mapped", Type: store.ChannelOpenAI, BaseURL: up.URL, APIKey: "k",
		Models: datatypes.JSONSlice[string]{"gpt-4o-mini"}, Priority: 1, Enabled: true,
		ModelMap: datatypes.NewJSONType(map[string]string{"gpt-4o-mini": "other-model"})})
	resp, _, _, err := r.Dispatch(context.Background(), basic)
	if err != nil || resp.Choices[0].Message.Content != "from other-model" || resp.Model != "gpt-4o-mini" {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
}

func TestAnthropicAdapterAgainstServer(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			w.WriteHeader(400)
			return
		}
		_, _ = io.WriteString(w, `{"id":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer s.Close()
	resp, err := AnthropicAdapter{Client: http.DefaultClient}.Do(context.Background(),
		store.Channel{BaseURL: s.URL, APIKey: "k"}, "claude-haiku-4-5", basic)
	if err != nil || resp.Choices[0].Message.Content != "ok" || resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("err=%v resp=%+v", err, resp)
	}
}
