// Command mockupstream is a fake OpenAI and Anthropic server for tests and demos.
// It never calls a real model. POST /_control switches a provider into a failure mode.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type server struct {
	mu    sync.Mutex
	modes map[string]string // provider -> ok|500|timeout|400
}

func newServer() *server {
	return &server{modes: map[string]string{"openai": "ok", "anthropic": "ok"}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	System    string    `json:"system"`
	MaxTokens int       `json:"max_tokens"`
}

func tokens(s string) int {
	if n := len([]rune(s)) / 4; n > 0 {
		return n
	}
	return 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

// failure applies the configured mode. It returns true when the request was answered.
func (s *server) failure(w http.ResponseWriter, provider string) bool {
	s.mu.Lock()
	mode := s.modes[provider]
	s.mu.Unlock()
	switch mode {
	case "500":
		writeJSON(w, 500, map[string]any{"error": map[string]string{"message": "mock upstream failure"}})
	case "400":
		writeJSON(w, 400, map[string]any{"error": map[string]string{"message": "mock bad request"}})
	case "timeout":
		time.Sleep(60 * time.Second)
		writeJSON(w, 504, map[string]string{"error": "timeout"})
	default:
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request) (chatRequest, bool) {
	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || len(req.Messages) == 0 {
		writeJSON(w, 400, map[string]any{"error": map[string]string{"message": "invalid request"}})
		return req, false
	}
	return req, true
}

func promptInfo(req chatRequest) (last string, promptTokens int) {
	all := req.System
	for _, m := range req.Messages {
		all += m.Content
		if m.Role == "user" {
			last = m.Content
		}
	}
	return last, tokens(all)
}

func (s *server) openai(w http.ResponseWriter, r *http.Request) {
	if s.failure(w, "openai") {
		return
	}
	req, ok := decode(w, r)
	if !ok {
		return
	}
	last, pt := promptInfo(req)
	content := "[mock-openai] " + last
	ct := tokens(content)
	writeJSON(w, 200, map[string]any{
		"id": "chatcmpl-mock", "object": "chat.completion", "created": time.Now().Unix(), "model": req.Model,
		"choices": []any{map[string]any{"index": 0, "finish_reason": "stop",
			"message": map[string]string{"role": "assistant", "content": content}}},
		"usage": map[string]int{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct},
	})
}

func (s *server) anthropic(w http.ResponseWriter, r *http.Request) {
	if s.failure(w, "anthropic") {
		return
	}
	if r.Header.Get("x-api-key") == "" || r.Header.Get("anthropic-version") == "" {
		writeJSON(w, 400, map[string]any{"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": "missing x-api-key or anthropic-version"}})
		return
	}
	req, ok := decode(w, r)
	if !ok {
		return
	}
	last, pt := promptInfo(req)
	text := "[mock-anthropic] " + last
	writeJSON(w, 200, map[string]any{
		"id": "msg_mock", "type": "message", "role": "assistant", "model": req.Model,
		"content":     []any{map[string]string{"type": "text", "text": text}},
		"stop_reason": "end_turn",
		"usage":       map[string]int{"input_tokens": pt, "output_tokens": tokens(text)},
	})
}

func (s *server) control(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct{ Provider, Mode string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid json"})
			return
		}
		valid := map[string]bool{"ok": true, "500": true, "400": true, "timeout": true}
		if _, known := s.modes[body.Provider]; !known || !valid[body.Mode] {
			writeJSON(w, 400, map[string]string{"error": "provider must be openai|anthropic, mode ok|500|400|timeout"})
			return
		}
		s.mu.Lock()
		s.modes[body.Provider] = body.Mode
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, 200, s.modes)
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /openai/v1/chat/completions", s.openai)
	mux.HandleFunc("POST /anthropic/v1/messages", s.anthropic)
	mux.HandleFunc("/_control", s.control)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	return mux
}

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":9000"
	}
	srv := &http.Server{Addr: addr, Handler: newServer().routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("mockupstream listening on %q", addr) // #nosec G706 -- addr comes from the operator's LISTEN_ADDR and is quoted
	log.Fatal(srv.ListenAndServe())
}
