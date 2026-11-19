package apikey

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// setup routes console requests as the user named in the X-Test-User header.
func setup(t *testing.T) (*gin.Engine, *Handler) {
	gin.SetMode(gin.TestMode)
	db := store.OpenTest(t)
	h := &Handler{DB: db}
	fakeAuth := func(c *gin.Context) {
		var u store.User
		db.Where("email = ?", c.GetHeader("X-Test-User")).First(&u)
		c.Set("user", &u)
	}
	r := gin.New()
	h.Routes(r.Group("/api"), fakeAuth)
	r.GET("/v1/ping", Authenticate(db), func(c *gin.Context) { c.Status(204) })
	for _, e := range []string{"a@example.com", "b@example.com"} {
		db.Create(&store.User{Email: e, PasswordHash: "x", Role: store.RoleUser})
	}
	return r, h
}

func call(r *gin.Engine, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type created struct {
	ID  uint   `json:"id"`
	Key string `json:"key"`
}

func createKey(t *testing.T, r *gin.Engine, user string) created {
	w := call(r, "POST", "/api/keys", `{"name":"test"}`, map[string]string{"X-Test-User": user})
	var c created
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if w.Code != 201 || !strings.HasPrefix(c.Key, "sk-") || len(c.Key) != 51 {
		t.Fatalf("create key %d %s", w.Code, w.Body.String())
	}
	return c
}

func bearer(k string) map[string]string { return map[string]string{"Authorization": "Bearer " + k} }

func TestValidKeyAccepted(t *testing.T) {
	r, _ := setup(t)
	k := createKey(t, r, "a@example.com")
	if w := call(r, "GET", "/v1/ping", "", bearer(k.Key)); w.Code != 204 {
		t.Fatalf("want 204 got %d", w.Code)
	}
	if w := call(r, "GET", "/v1/ping", "", bearer("sk-nope")); w.Code != 401 || !strings.Contains(w.Body.String(), `"type":"invalid_request_error"`) {
		t.Fatalf("bad key want 401 openai error, got %d %s", w.Code, w.Body.String())
	}
}

func TestDisabledKey401(t *testing.T) {
	r, _ := setup(t)
	k := createKey(t, r, "a@example.com")
	w := call(r, "PATCH", "/api/keys/"+itoa(k.ID), `{"disabled":true}`, map[string]string{"X-Test-User": "a@example.com"})
	if w.Code != 200 {
		t.Fatalf("disable %d", w.Code)
	}
	if w := call(r, "GET", "/v1/ping", "", bearer(k.Key)); w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestDisabledUserKey401(t *testing.T) {
	r, h := setup(t)
	k := createKey(t, r, "a@example.com")
	h.DB.Model(&store.User{}).Where("email = ?", "a@example.com").Update("disabled", true)
	if w := call(r, "GET", "/v1/ping", "", bearer(k.Key)); w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestCannotDisableOthersKey404(t *testing.T) {
	r, _ := setup(t)
	k := createKey(t, r, "a@example.com")
	w := call(r, "PATCH", "/api/keys/"+itoa(k.ID), `{"disabled":true}`, map[string]string{"X-Test-User": "b@example.com"})
	if w.Code != 404 {
		t.Fatalf("want 404 got %d", w.Code)
	}
	list := call(r, "GET", "/api/keys", "", map[string]string{"X-Test-User": "b@example.com"})
	if strings.Contains(list.Body.String(), `"id"`) {
		t.Fatalf("b sees a's keys: %s", list.Body.String())
	}
}

func TestKeyStoredHashed(t *testing.T) {
	r, h := setup(t)
	k := createKey(t, r, "a@example.com")
	var n int64
	h.DB.Model(&store.APIKey{}).Where("key_hash = ? OR key_prefix = ?", k.Key, k.Key).Count(&n)
	if n != 0 {
		t.Fatal("plaintext key found in database")
	}
	list := call(r, "GET", "/api/keys", "", map[string]string{"X-Test-User": "a@example.com"})
	if strings.Contains(list.Body.String(), k.Key) {
		t.Fatal("list leaks plaintext key")
	}
}
