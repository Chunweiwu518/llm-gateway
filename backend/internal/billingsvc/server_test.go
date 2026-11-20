package billingsvc

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/billingpb"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/events"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

type fakeAlerts struct {
	mu   sync.Mutex
	msgs []events.LowBalance
	ch   chan struct{}
}

func (f *fakeAlerts) Publish(_ context.Context, m events.LowBalance) error {
	f.mu.Lock()
	f.msgs = append(f.msgs, m)
	f.mu.Unlock()
	f.ch <- struct{}{}
	return nil
}

func TestSettleRefundsAndReportsBalance(t *testing.T) {
	db := store.OpenTest(t)
	u := store.User{Email: "a@example.com", PasswordHash: "x", Balance: 900} // 1000 minus a 100 reservation
	db.Create(&u)
	s := &Server{DB: db}
	res, err := s.Settle(context.Background(), &billingpb.SettleRequest{UserId: uint64(u.ID), Reserved: 100, Actual: 40})
	if err != nil || res.Charged != 40 || res.Balance != 960 {
		t.Fatalf("res %+v err %v", res, err)
	}
	got, err := s.GetBalance(context.Background(), &billingpb.GetBalanceRequest{UserId: uint64(u.ID)})
	if err != nil || got.Balance != 960 {
		t.Fatalf("balance %+v err %v", got, err)
	}
}

func TestSettleNeverOverdraws(t *testing.T) {
	db := store.OpenTest(t)
	u := store.User{Email: "b@example.com", PasswordHash: "x", Balance: 10}
	db.Create(&u)
	res, err := (&Server{DB: db}).Settle(context.Background(), &billingpb.SettleRequest{UserId: uint64(u.ID), Reserved: 100, Actual: 500})
	if err != nil || res.Charged != 110 || res.Balance != 0 {
		t.Fatalf("res %+v err %v", res, err)
	}
}

func TestSettleRejectsBadInput(t *testing.T) {
	db := store.OpenTest(t)
	s := &Server{DB: db}
	if _, err := s.Settle(context.Background(), &billingpb.SettleRequest{UserId: 1, Reserved: -1}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("negative reserved: %v", err)
	}
	if _, err := s.Settle(context.Background(), &billingpb.SettleRequest{UserId: 99999, Reserved: 1, Actual: 1}); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown user: %v", err)
	}
}

func TestLowBalanceAlertIsPublishedOncePerMinute(t *testing.T) {
	db := store.OpenTest(t)
	u := store.User{Email: "c@example.com", PasswordHash: "x", Balance: 400}
	db.Create(&u)
	alerts := &fakeAlerts{ch: make(chan struct{}, 4)}
	s := &Server{DB: db, Alerts: alerts, Threshold: 500}
	for range 3 {
		if _, err := s.Settle(context.Background(), &billingpb.SettleRequest{UserId: uint64(u.ID), Reserved: 10, Actual: 10}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-alerts.ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no alert published")
	}
	time.Sleep(100 * time.Millisecond)
	alerts.mu.Lock()
	defer alerts.mu.Unlock()
	if len(alerts.msgs) != 1 || alerts.msgs[0].UserID != u.ID || alerts.msgs[0].Balance != 400 {
		t.Fatalf("alerts %+v", alerts.msgs)
	}
}
