package apikey

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// AuthenticateWith skips the last_used_at write while the stored value is recent,
// and writes again once it is older than the interval.
func TestLastUsedWriteIsThrottled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := store.OpenTest(t)
	u := store.User{Email: "t@example.com", PasswordHash: "x", Role: store.RoleUser}
	db.Create(&u)
	plain, hash, prefix, _ := Generate()
	k := store.APIKey{UserID: u.ID, Name: "t", KeyHash: hash, KeyPrefix: prefix, RPMLimit: 60}
	db.Create(&k)
	r := gin.New()
	r.GET("/v1/ping", AuthenticateWith(db, time.Hour), func(c *gin.Context) { c.Status(204) })

	lastUsed := func() time.Time {
		var got store.APIKey
		db.First(&got, k.ID)
		if got.LastUsedAt == nil {
			return time.Time{}
		}
		return *got.LastUsedAt
	}
	if w := call(r, "GET", "/v1/ping", "", bearer(plain)); w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
	first := lastUsed()
	if first.IsZero() {
		t.Fatal("first request did not stamp last_used_at")
	}
	time.Sleep(1100 * time.Millisecond)
	call(r, "GET", "/v1/ping", "", bearer(plain))
	if got := lastUsed(); !got.Equal(first) {
		t.Fatalf("last_used_at rewritten inside the interval: %v -> %v", first, got)
	}
	// An old value is refreshed.
	db.Model(&k).UpdateColumn("last_used_at", time.Now().Add(-2*time.Hour))
	call(r, "GET", "/v1/ping", "", bearer(plain))
	if got := lastUsed(); time.Since(got) > time.Minute {
		t.Fatalf("stale last_used_at not refreshed: %v", got)
	}
}
