package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newLimiter(t *testing.T) (*Limiter, *time.Time) {
	mr := miniredis.RunT(t)
	l := New(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	now := time.Date(2026, 1, 1, 10, 0, 5, 0, time.UTC)
	l.now = func() time.Time { return now }
	return l, &now
}

func TestLimiterBlocksAfterRPM(t *testing.T) {
	l, _ := newLimiter(t)
	for i := 1; i <= 4; i++ {
		ok, err := l.Allow(context.Background(), 7, 3)
		if err != nil {
			t.Fatal(err)
		}
		if want := i <= 3; ok != want {
			t.Fatalf("request %d: allowed=%v want %v", i, ok, want)
		}
	}
	if ok, _ := l.Allow(context.Background(), 8, 3); !ok {
		t.Fatal("other key must have its own window")
	}
}

func TestLimiterResetsNextMinute(t *testing.T) {
	l, now := newLimiter(t)
	for i := 0; i < 3; i++ {
		_, _ = l.Allow(context.Background(), 7, 3)
	}
	*now = now.Add(time.Minute)
	if ok, _ := l.Allow(context.Background(), 7, 3); !ok {
		t.Fatal("new minute should allow")
	}
}

func TestLimiterRedisDownFailsOpen(t *testing.T) {
	l := New(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond}))
	ok, err := l.Allow(context.Background(), 1, 1)
	if !ok || err == nil {
		t.Fatalf("want allowed with error, got %v %v", ok, err)
	}
}
