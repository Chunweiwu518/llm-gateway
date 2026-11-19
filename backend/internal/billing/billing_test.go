package billing

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

func newUser(t *testing.T, db *gorm.DB, balance int64) store.User {
	u := store.User{Email: "u@example.com", PasswordHash: "x", Role: store.RoleUser, Balance: balance}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func balanceOf(db *gorm.DB, id uint) int64 {
	var u store.User
	db.First(&u, id)
	return u.Balance
}

func TestCostRoundsUp(t *testing.T) {
	p := store.ModelPrice{InputPerM: 150_000, OutputPerM: 600_000} // $0.15 / $0.60 per 1M tokens
	if got := Cost(p, 1_000_000, 1_000_000); got != 750_000 {
		t.Fatalf("1M+1M tokens: got %d", got)
	}
	if got := Cost(p, 1, 0); got != 1 {
		t.Fatalf("one token must cost at least 1 micro-USD, got %d", got)
	}
	if got := Cost(p, 0, 0); got != 0 {
		t.Fatalf("zero tokens: got %d", got)
	}
}

func TestReserveInsufficient402(t *testing.T) {
	db := store.OpenTest(t)
	u := newUser(t, db, 100)
	if err := Reserve(db, u.ID, 101); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("want ErrInsufficient got %v", err)
	}
	if err := Reserve(db, u.ID, 100); err != nil {
		t.Fatal(err)
	}
	if err := Reserve(db, u.ID, 0); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("zero balance must be rejected even for a free call, got %v", err)
	}
	if b := balanceOf(db, u.ID); b != 0 {
		t.Fatalf("balance %d", b)
	}
}

func TestConcurrentReserveNeverNegative(t *testing.T) {
	db := store.OpenTest(t)
	u := newUser(t, db, 500)
	var ok int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if Reserve(db, u.ID, 100) == nil {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 5 || balanceOf(db, u.ID) != 0 {
		t.Fatalf("succeeded=%d balance=%d, want 5 and 0", ok, balanceOf(db, u.ID))
	}
}

func TestSettleRefundsDifference(t *testing.T) {
	db := store.OpenTest(t)
	u := newUser(t, db, 1000)
	_ = Reserve(db, u.ID, 300)
	if _, err := Settle(db, u.ID, 300, 120); err != nil {
		t.Fatal(err)
	}
	if b := balanceOf(db, u.ID); b != 880 {
		t.Fatalf("want 880 got %d", b)
	}
	_ = Reserve(db, u.ID, 100)
	_, _ = Settle(db, u.ID, 100, 150)
	if b := balanceOf(db, u.ID); b != 730 {
		t.Fatalf("extra usage must be charged: want 730 got %d", b)
	}
}

func TestTopupRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := store.OpenTest(t)
	u := newUser(t, db, 0)
	r := gin.New()
	(&Handler{DB: db}).Routes(r.Group("/api"), func(c *gin.Context) { c.Set("user", &u) })
	post := func(body string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/billing/topup", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}
	for _, bad := range []string{`{"amount_usd":0}`, `{"amount_usd":1001}`, `{"amount_usd":-5}`, `{}`} {
		if code := post(bad); code != 400 {
			t.Fatalf("%s: want 400 got %d", bad, code)
		}
	}
	if code := post(`{"amount_usd":12.5}`); code != 200 {
		t.Fatalf("valid topup got %d", code)
	}
	var n int64
	db.Model(&store.Topup{}).Where("user_id = ? AND source = ? AND amount = ?", u.ID, store.TopupSelf, 12_500_000).Count(&n)
	if balanceOf(db, u.ID) != 12_500_000 || n != 1 {
		t.Fatalf("balance %d records %d", balanceOf(db, u.ID), n)
	}
}

func TestTopupCannotGoNegative(t *testing.T) {
	db := store.OpenTest(t)
	u := newUser(t, db, 100)
	if err := Topup(db, u.ID, -101, store.TopupAdmin); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("want ErrInsufficient got %v", err)
	}
	if balanceOf(db, u.ID) != 100 {
		t.Fatal("balance changed on rejected adjustment")
	}
}

func TestReserveRejectsNegativeAmount(t *testing.T) {
	db := store.OpenTest(t)
	u := newUser(t, db, 1_000)
	if err := Reserve(db, u.ID, -5_000_000); err == nil {
		t.Fatal("negative reservation must fail")
	}
	if got := balanceOf(db, u.ID); got != 1_000 {
		t.Fatalf("balance changed to %d", got)
	}
}

func TestSettleNeverOverdraws(t *testing.T) {
	db := store.OpenTest(t)
	u := newUser(t, db, 1_000) // balance left after reserving 500
	charged, err := Settle(db, u.ID, 500, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if got := balanceOf(db, u.ID); got != 0 {
		t.Fatalf("balance %d, want 0", got)
	}
	if charged != 1_500 {
		t.Fatalf("charged %d, want reserved 500 + remaining 1000", charged)
	}
}
