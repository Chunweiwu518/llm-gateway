package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/apikey"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/billing"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

type gw struct {
	r   *gin.Engine
	db  *gorm.DB
	key string
	uid uint
}

// newGateway wires /v1 with one channel per upstream and a priced model.
func newGateway(t *testing.T, balance int64, upstreams ...*httptest.Server) *gw {
	gin.SetMode(gin.TestMode)
	db := store.OpenTest(t)
	for i, u := range upstreams {
		db.Create(&store.Channel{Name: "ch" + string(rune('A'+i)), Type: store.ChannelOpenAI, BaseURL: u.URL, APIKey: "k",
			Models: datatypes.JSONSlice[string]{"gpt-4o-mini"}, Priority: i + 1, Enabled: true,
			ModelMap: datatypes.NewJSONType(map[string]string{})})
	}
	db.Create(&store.ModelPrice{Model: "gpt-4o-mini", InputPerM: 150_000, OutputPerM: 600_000})
	u := store.User{Email: "dev@example.com", PasswordHash: "x", Role: store.RoleUser, Balance: balance}
	db.Create(&u)
	plain, hash, prefix, _ := apikey.Generate()
	db.Create(&store.APIKey{UserID: u.ID, Name: "t", KeyHash: hash, KeyPrefix: prefix, RPMLimit: 60})
	r := gin.New()
	h := &Handler{DB: db, Router: NewRouter(db, 300*time.Millisecond)}
	h.Routes(r.Group("/v1", apikey.Authenticate(db)))
	return &gw{r: r, db: db, key: plain, uid: u.ID}
}

func (g *gw) chat(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+g.key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.r.ServeHTTP(w, req)
	return w
}

func (g *gw) balance() int64 {
	var u store.User
	g.db.First(&u, g.uid)
	return u.Balance
}

const hello = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`

func TestChatCompletionEndToEnd(t *testing.T) {
	var hits int32
	g := newGateway(t, 10_000_000, fakeUpstream(t, 200, 0, &hits))
	w := g.chat(hello)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total_tokens":5`) {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	want := billing.Cost(store.ModelPrice{InputPerM: 150_000, OutputPerM: 600_000}, 3, 2)
	if got := 10_000_000 - g.balance(); got != want {
		t.Fatalf("charged %d want %d", got, want)
	}
	var log store.UsageLog
	g.db.Where("user_id = ?", g.uid).First(&log)
	if log.PromptTokens != 3 || log.CompletionTokens != 2 || log.Cost != want || log.StatusCode != 200 || log.ChannelName != "chA" {
		t.Fatalf("usage log %+v", log)
	}
}

func TestAllUpstreamDownRefundsAndLogs502(t *testing.T) {
	var a, b int32
	g := newGateway(t, 10_000_000, fakeUpstream(t, 500, 0, &a), fakeUpstream(t, 503, 0, &b))
	w := g.chat(hello)
	if w.Code != 502 || !strings.Contains(w.Body.String(), `"type":"upstream_error"`) {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	if g.balance() != 10_000_000 {
		t.Fatalf("reservation not refunded: %d", g.balance())
	}
	var log store.UsageLog
	g.db.Where("user_id = ?", g.uid).First(&log)
	if log.StatusCode != 502 || log.Cost != 0 || log.FailoverCount != 1 {
		t.Fatalf("usage log %+v", log)
	}
}

func TestUpstream400NotCharged(t *testing.T) {
	var a int32
	g := newGateway(t, 10_000_000, fakeUpstream(t, 400, 0, &a))
	if w := g.chat(hello); w.Code != 400 {
		t.Fatalf("want 400 got %d", w.Code)
	}
	if g.balance() != 10_000_000 {
		t.Fatal("charged for a failed request")
	}
}

func TestInsufficientBalance402(t *testing.T) {
	var a int32
	g := newGateway(t, 0, fakeUpstream(t, 200, 0, &a))
	w := g.chat(hello)
	if w.Code != 402 || !strings.Contains(w.Body.String(), `"code":"insufficient_quota"`) || a != 0 {
		t.Fatalf("status %d hits %d %s", w.Code, a, w.Body.String())
	}
}

func TestBadRequests400(t *testing.T) {
	var a int32
	g := newGateway(t, 10_000_000, fakeUpstream(t, 200, 0, &a))
	cases := map[string]string{
		"stream":        `{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hi"}]}`,
		"unknown model": `{"model":"nope","messages":[{"role":"user","content":"hi"}]}`,
		"no messages":   `{"model":"gpt-4o-mini","messages":[]}`,
		"bad json":      `{"model":`,
		"array content": `{"model":"gpt-4o-mini","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`,
	}
	for name, body := range cases {
		if w := g.chat(body); w.Code != 400 || !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("%s: want 400 got %d %s", name, w.Code, w.Body.String())
		}
	}
	if a != 0 || g.balance() != 10_000_000 {
		t.Fatalf("bad requests reached upstream (%d) or charged", a)
	}
}

func TestListModels(t *testing.T) {
	var a int32
	g := newGateway(t, 1, fakeUpstream(t, 200, 0, &a))
	g.db.Create(&store.ModelPrice{Model: "priced-no-channel", InputPerM: 1, OutputPerM: 1})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+g.key)
	w := httptest.NewRecorder()
	g.r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"gpt-4o-mini"`) || strings.Contains(w.Body.String(), "priced-no-channel") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
