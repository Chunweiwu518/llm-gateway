package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, h http.Handler, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

const chatBody = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello world"}]}`
const msgBody = `{"model":"claude-haiku-4-5","max_tokens":100,"messages":[{"role":"user","content":"hello world"}]}`

var anthropicHdr = map[string]string{"x-api-key": "k", "anthropic-version": "2023-06-01"}

func TestOpenAIOK(t *testing.T) {
	h := newServer().routes()
	w := post(t, h, "/openai/v1/chat/completions", chatBody, map[string]string{"Authorization": "Bearer k"})
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var r struct {
		Choices []struct{ Message struct{ Content string } }
	}
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if r.Choices[0].Message.Content != "[mock-openai] hello world" {
		t.Fatalf("content %q", r.Choices[0].Message.Content)
	}
	if !strings.Contains(w.Body.String(), `"prompt_tokens":2`) {
		t.Fatalf("usage %s", w.Body.String())
	}
}

func TestAnthropicOK(t *testing.T) {
	h := newServer().routes()
	w := post(t, h, "/anthropic/v1/messages", msgBody, anthropicHdr)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	b := w.Body.String()
	if !strings.Contains(b, `"text":"[mock-anthropic] hello world"`) || !strings.Contains(b, `"input_tokens":2`) {
		t.Fatalf("body %s", b)
	}
}

func TestControlMode500(t *testing.T) {
	h := newServer().routes()
	if w := post(t, h, "/_control", `{"provider":"openai","mode":"500"}`, nil); w.Code != 200 {
		t.Fatalf("control %d", w.Code)
	}
	if w := post(t, h, "/openai/v1/chat/completions", chatBody, nil); w.Code != 500 {
		t.Fatalf("want 500 got %d", w.Code)
	}
	if w := post(t, h, "/anthropic/v1/messages", msgBody, anthropicHdr); w.Code != 200 {
		t.Fatalf("anthropic should stay ok, got %d", w.Code)
	}
}

func TestAnthropicMissingVersion400(t *testing.T) {
	h := newServer().routes()
	w := post(t, h, "/anthropic/v1/messages", msgBody, map[string]string{"x-api-key": "k"})
	if w.Code != 400 {
		t.Fatalf("want 400 got %d", w.Code)
	}
}
