package admin

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/auth"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// setup authenticates as the user whose email is in X-Test-User and applies
// the real RequireAdmin check.
func setup(t *testing.T) (*gin.Engine, *gorm.DB, store.User, store.User) {
	gin.SetMode(gin.TestMode)
	db := store.OpenTest(t)
	adm := store.User{Email: "admin@example.com", PasswordHash: "x", Role: store.RoleAdmin}
	dev := store.User{Email: "dev@example.com", PasswordHash: "x", Role: store.RoleUser, Balance: 1_000_000}
	db.Create(&adm)
	db.Create(&dev)
	fakeUser := func(c *gin.Context) {
		var u store.User
		db.Where("email = ?", c.GetHeader("X-Test-User")).First(&u)
		c.Set("user", &u)
	}
	r := gin.New()
	(&Handler{DB: db}).Routes(r.Group("/api/admin", fakeUser, auth.RequireAdmin()))
	return r, db, adm, dev
}

func call(r *gin.Engine, as, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", as)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func id(u store.User) string { return strconv.FormatUint(uint64(u.ID), 10) }

func TestAdminCreditAndAudit(t *testing.T) {
	r, db, adm, dev := setup(t)
	if w := call(r, adm.Email, "POST", "/api/admin/users/"+id(dev)+"/credit", `{"amount_usd":-0.5}`); w.Code != 200 {
		t.Fatalf("credit %d %s", w.Code, w.Body.String())
	}
	var u store.User
	db.First(&u, dev.ID)
	var n int64
	db.Model(&store.Topup{}).Where("user_id = ? AND source = ? AND amount = ?", dev.ID, store.TopupAdmin, -500_000).Count(&n)
	if u.Balance != 500_000 || n != 1 {
		t.Fatalf("balance %d audit rows %d", u.Balance, n)
	}
	if w := call(r, adm.Email, "POST", "/api/admin/users/"+id(dev)+"/credit", `{"amount_usd":-1}`); w.Code != 400 {
		t.Fatalf("overdraw want 400 got %d", w.Code)
	}
}

func TestChannelKeyMasked(t *testing.T) {
	r, _, adm, _ := setup(t)
	body := `{"name":"oa","type":"openai","base_url":"http://mock:9000/openai","api_key":"sk-secret-abcd1234",
		"models":["gpt-4o-mini"],"model_map":{},"priority":1,"enabled":true}`
	if w := call(r, adm.Email, "POST", "/api/admin/channels", body); w.Code != 201 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	w := call(r, adm.Email, "GET", "/api/admin/channels", "")
	if strings.Contains(w.Body.String(), "sk-secret") || !strings.Contains(w.Body.String(), `"api_key_masked":"****1234"`) {
		t.Fatalf("list %s", w.Body.String())
	}
	bad := strings.Replace(body, `"openai"`, `"gemini"`, 1)
	if w := call(r, adm.Email, "POST", "/api/admin/channels", bad); w.Code != 400 {
		t.Fatalf("unknown type want 400 got %d", w.Code)
	}
}

func TestChannelUpdateKeepsKeyWhenBlank(t *testing.T) {
	r, db, adm, _ := setup(t)
	ch := store.Channel{Name: "oa", Type: store.ChannelOpenAI, BaseURL: "http://x", APIKey: "keep-me", Priority: 1, Enabled: true}
	db.Create(&ch)
	cid := strconv.FormatUint(uint64(ch.ID), 10)
	w := call(r, adm.Email, "PATCH", "/api/admin/channels/"+cid, `{"enabled":false,"priority":5}`)
	db.First(&ch, ch.ID)
	if w.Code != 200 || ch.APIKey != "keep-me" || ch.Enabled || ch.Priority != 5 {
		t.Fatalf("%d %+v", w.Code, ch)
	}
}

func TestPrices(t *testing.T) {
	r, db, adm, _ := setup(t)
	if w := call(r, adm.Email, "PUT", "/api/admin/prices/gpt-4o-mini", `{"input_per_m":150000,"output_per_m":600000}`); w.Code != 200 {
		t.Fatalf("put %d", w.Code)
	}
	if w := call(r, adm.Email, "PUT", "/api/admin/prices/gpt-4o-mini", `{"input_per_m":-1,"output_per_m":1}`); w.Code != 400 {
		t.Fatalf("negative price want 400 got %d", w.Code)
	}
	var p store.ModelPrice
	db.First(&p, "model = ?", "gpt-4o-mini")
	if p.OutputPerM != 600_000 {
		t.Fatalf("%+v", p)
	}
	if w := call(r, adm.Email, "DELETE", "/api/admin/prices/gpt-4o-mini", ""); w.Code != 204 {
		t.Fatalf("delete %d", w.Code)
	}
}

func TestAdminCannotDemoteSelf(t *testing.T) {
	r, _, adm, dev := setup(t)
	if w := call(r, adm.Email, "PATCH", "/api/admin/users/"+id(adm), `{"role":"user"}`); w.Code != 400 {
		t.Fatalf("self demote want 400 got %d", w.Code)
	}
	if w := call(r, adm.Email, "PATCH", "/api/admin/users/"+id(adm), `{"disabled":true}`); w.Code != 400 {
		t.Fatalf("self disable want 400 got %d", w.Code)
	}
	if w := call(r, adm.Email, "PATCH", "/api/admin/users/"+id(dev), `{"role":"admin"}`); w.Code != 200 {
		t.Fatalf("promote other %d", w.Code)
	}
}

func TestNonAdmin403(t *testing.T) {
	r, _, _, dev := setup(t)
	routes := [][2]string{
		{"GET", "/api/admin/users"}, {"PATCH", "/api/admin/users/1"}, {"POST", "/api/admin/users/1/credit"},
		{"GET", "/api/admin/channels"}, {"POST", "/api/admin/channels"}, {"PATCH", "/api/admin/channels/1"},
		{"DELETE", "/api/admin/channels/1"}, {"GET", "/api/admin/prices"}, {"PUT", "/api/admin/prices/x"},
		{"DELETE", "/api/admin/prices/x"}, {"GET", "/api/admin/usage"},
	}
	for _, rt := range routes {
		if w := call(r, dev.Email, rt[0], rt[1], `{}`); w.Code != http.StatusForbidden {
			t.Errorf("%s %s: want 403 got %d", rt[0], rt[1], w.Code)
		}
	}
}

func TestSeedDefaultsOnce(t *testing.T) {
	_, db, _, _ := setup(t)
	for i := 0; i < 2; i++ {
		if err := store.SeedDefaults(db, "http://mockupstream:9000"); err != nil {
			t.Fatal(err)
		}
	}
	var chans []store.Channel
	db.Order("priority").Find(&chans)
	if len(chans) != 2 || chans[0].Type != store.ChannelOpenAI || chans[1].ModelMap.Data()["gpt-4o-mini"] != "claude-haiku-4-5" {
		t.Fatalf("%+v", chans)
	}
}
