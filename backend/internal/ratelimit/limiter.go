// Package ratelimit enforces a per-API-key requests-per-minute limit in Redis.
package ratelimit

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/apikey"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
)

// Limiter is a fixed one-minute window counter per key.
type Limiter struct {
	rdb *redis.Client
	now func() time.Time
}

// New returns a limiter backed by rdb.
func New(rdb *redis.Client) *Limiter { return &Limiter{rdb: rdb, now: time.Now} }

// Allow counts one request for keyID. When Redis fails it allows the request
// and returns the error so the caller can log it (fail-open).
func (l *Limiter) Allow(ctx context.Context, keyID uint, rpm int) (bool, error) {
	k := fmt.Sprintf("rl:%d:%d", keyID, l.now().Unix()/60)
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.Expire(ctx, k, 70*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, err
	}
	return incr.Val() <= int64(rpm), nil
}

// Middleware rejects requests over the key's rpm_limit with 429.
// It must run after apikey.Authenticate.
func Middleware(l *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		k, _ := apikey.FromContext(c)
		ok, err := l.Allow(c.Request.Context(), k.ID, k.RPMLimit)
		if err != nil {
			log.Printf("rate limiter unavailable, allowing request: %v", err)
		}
		if !ok {
			httpx.OpenAIError(c, http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded",
				fmt.Sprintf("rate limit of %d requests per minute exceeded", k.RPMLimit))
			return
		}
		c.Next()
	}
}
