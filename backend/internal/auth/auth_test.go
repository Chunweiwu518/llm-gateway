package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/ratelimit"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

type env struct {
	t *testing.T
	r *gin.Engine
	h *Handler
}

func setup(t *testing.T) *env {
	gin.SetMode(gin.TestMode)
	db := store.OpenTest(t)
	h := &Handler{DB: db, JWT: NewManager(strings.Repeat("s", 32))}
	r := gin.New()
	api := r.Group("/api")
	h.Routes(api)
	api.GET("/admin/ping", h.RequireUser(), RequireAdmin(), func(c *gin.Context) { c.Status(204) })
	return &env{t: t, r: r, h: h}
}

func (e *env) do(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w
}

func cookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	return nil
}

func (e *env) register(email string) {
	if w := e.do("POST", "/api/auth/register", `{"email":"`+email+`","password":"password123"}`); w.Code != 201 {
		e.t.Fatalf("register %d %s", w.Code, w.Body.String())
	}
}

func (e *env) login(email, pw string) *httptest.ResponseRecorder {
	return e.do("POST", "/api/auth/login", `{"email":"`+email+`","password":"`+pw+`"}`)
}

// enableTOTP turns on TOTP for the user and returns the secret.
func (e *env) enableTOTP(email string) string {
	sess := cookie(e.login(email, "password123"), SessionCookie)
	w := e.do("POST", "/api/me/totp/setup", "", sess)
	var secret string
	e.h.DB.Model(&store.User{}).Where("email = ?", email).Select("totp_secret").Scan(&secret)
	code, _ := totp.GenerateCode(secret, time.Now())
	if w2 := e.do("POST", "/api/me/totp/enable", `{"code":"`+code+`"}`, sess); w.Code != 200 || w2.Code != 200 {
		e.t.Fatalf("enable totp %d %d %s", w.Code, w2.Code, w2.Body.String())
	}
	return secret
}

func TestLoginWithoutTOTP(t *testing.T) {
	e := setup(t)
	e.register("dev@example.com")
	w := e.login("dev@example.com", "password123")
	sess := cookie(w, SessionCookie)
	if w.Code != 200 || sess == nil || !sess.HttpOnly {
		t.Fatalf("login %d cookie %+v", w.Code, sess)
	}
	if me := e.do("GET", "/api/me", "", sess); me.Code != 200 || !strings.Contains(me.Body.String(), `"role":"user"`) {
		t.Fatalf("me %d %s", me.Code, me.Body.String())
	}
}

func TestLoginWithTOTPRequiresCode(t *testing.T) {
	e := setup(t)
	e.register("dev@example.com")
	secret := e.enableTOTP("dev@example.com")
	w := e.login("dev@example.com", "password123")
	if !strings.Contains(w.Body.String(), `"totp_required":true`) || cookie(w, SessionCookie) != nil {
		t.Fatalf("expected totp step: %s", w.Body.String())
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	w2 := e.do("POST", "/api/auth/login/totp", `{"code":"`+code+`"}`, cookie(w, PendingCookie))
	if w2.Code != 200 || cookie(w2, SessionCookie) == nil {
		t.Fatalf("totp login %d %s", w2.Code, w2.Body.String())
	}
}

func TestWrongTOTPRejected(t *testing.T) {
	e := setup(t)
	e.register("dev@example.com")
	e.enableTOTP("dev@example.com")
	w := e.login("dev@example.com", "password123")
	w2 := e.do("POST", "/api/auth/login/totp", `{"code":"000000"}`, cookie(w, PendingCookie))
	if w2.Code != 401 || cookie(w2, SessionCookie) != nil {
		t.Fatalf("want 401 got %d", w2.Code)
	}
}

func TestPendingTokenCannotAccessMe(t *testing.T) {
	e := setup(t)
	e.register("dev@example.com")
	e.enableTOTP("dev@example.com")
	p := cookie(e.login("dev@example.com", "password123"), PendingCookie)
	forged := &http.Cookie{Name: SessionCookie, Value: p.Value}
	if w := e.do("GET", "/api/me", "", forged); w.Code != 401 {
		t.Fatalf("pending token used as session: want 401 got %d", w.Code)
	}
}

func TestUserCannotAccessAdmin(t *testing.T) {
	e := setup(t)
	e.register("dev@example.com")
	sess := cookie(e.login("dev@example.com", "password123"), SessionCookie)
	if w := e.do("GET", "/api/admin/ping", "", sess); w.Code != 403 {
		t.Fatalf("want 403 got %d", w.Code)
	}
}

func TestDisabledUserLoginRejected(t *testing.T) {
	e := setup(t)
	e.register("dev@example.com")
	sess := cookie(e.login("dev@example.com", "password123"), SessionCookie)
	e.h.DB.Model(&store.User{}).Where("email = ?", "dev@example.com").Update("disabled", true)
	if w := e.login("dev@example.com", "password123"); w.Code != 403 {
		t.Fatalf("login want 403 got %d", w.Code)
	}
	if w := e.do("GET", "/api/me", "", sess); w.Code != 401 {
		t.Fatalf("existing session want 401 got %d", w.Code)
	}
}

func TestWrongPassword401(t *testing.T) {
	e := setup(t)
	e.register("dev@example.com")
	if w := e.login("dev@example.com", "wrong-password"); w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}
	if w := e.login("nobody@example.com", "password123"); w.Code != 401 {
		t.Fatalf("unknown user want 401 got %d", w.Code)
	}
}

func TestRegisterValidation(t *testing.T) {
	e := setup(t)
	if w := e.do("POST", "/api/auth/register", `{"email":"bad","password":"password123"}`); w.Code != 400 {
		t.Fatalf("bad email want 400 got %d", w.Code)
	}
	if w := e.do("POST", "/api/auth/register", `{"email":"a@b.co","password":"short"}`); w.Code != 400 {
		t.Fatalf("short password want 400 got %d", w.Code)
	}
	e.register("dup@example.com")
	if w := e.do("POST", "/api/auth/register", `{"email":"dup@example.com","password":"password123"}`); w.Code != 409 {
		t.Fatalf("duplicate want 409 got %d", w.Code)
	}
}

func setupLimited(t *testing.T) *env {
	e := setup(t)
	mr := miniredis.RunT(t)
	e.h.Limiter = ratelimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	fixed := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	e.h.Limiter.SetClock(func() time.Time { return fixed }) // bcrypt is slow under -race; never cross a minute
	return e
}

func TestLoginAttemptsLimited(t *testing.T) {
	e := setupLimited(t)
	e.register("dev@example.com")
	for i := 1; i <= LoginAttemptsPerMinute; i++ {
		if w := e.login("dev@example.com", "wrong-password"); w.Code != 401 {
			t.Fatalf("attempt %d: want 401 got %d", i, w.Code)
		}
	}
	if w := e.login("dev@example.com", "password123"); w.Code != 429 {
		t.Fatalf("over limit: want 429 even with the right password, got %d", w.Code)
	}
}

func TestTOTPAttemptsLimited(t *testing.T) {
	e := setupLimited(t)
	e.register("dev@example.com")
	secret := e.enableTOTP("dev@example.com")
	p := cookie(e.login("dev@example.com", "password123"), PendingCookie)
	for i := 1; i <= TOTPAttemptsPerMinute; i++ {
		if w := e.do("POST", "/api/auth/login/totp", `{"code":"000000"}`, p); w.Code != 401 {
			t.Fatalf("attempt %d: want 401 got %d", i, w.Code)
		}
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	if w := e.do("POST", "/api/auth/login/totp", `{"code":"`+code+`"}`, p); w.Code != 429 {
		t.Fatalf("over limit: want 429 got %d", w.Code)
	}
}
