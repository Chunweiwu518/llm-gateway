// Package billingsvc is the gRPC billing service (go-zero zrpc) and its client.
package billingsvc

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/billing"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/billingpb"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/events"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Alerter publishes low-balance notifications.
type Alerter interface {
	Publish(ctx context.Context, m events.LowBalance) error
}

// alertEvery limits low-balance alerts to one per user per interval.
const alertEvery = time.Minute

// Server implements billingpb.BillingServer on top of the shared MySQL balance.
type Server struct {
	billingpb.UnimplementedBillingServer
	DB        *gorm.DB
	Alerts    Alerter // nil disables alerts
	Threshold int64   // micro-USD; alert when the balance after settlement is below it

	mu        sync.Mutex
	lastAlert map[uint]time.Time
}

// Settle closes a reservation and reports the charged amount and new balance.
func (s *Server) Settle(ctx context.Context, req *billingpb.SettleRequest) (*billingpb.SettleResponse, error) {
	if req.Reserved < 0 || req.Actual < 0 {
		return nil, status.Error(codes.InvalidArgument, "reserved and actual must not be negative")
	}
	uid := uint(req.UserId)
	db := s.DB.WithContext(ctx)
	charged, err := billing.Settle(db, uid, req.Reserved, req.Actual)
	if err != nil {
		log.Printf("settle user %d (reserved %d actual %d) failed: %v", uid, req.Reserved, req.Actual, err)
		return nil, status.Error(codes.Internal, "settle failed")
	}
	bal, err := s.balance(db, uid)
	if err != nil {
		return nil, err
	}
	s.maybeAlert(uid, bal)
	return &billingpb.SettleResponse{Charged: charged, Balance: bal}, nil
}

// GetBalance returns the user's current balance.
func (s *Server) GetBalance(ctx context.Context, req *billingpb.GetBalanceRequest) (*billingpb.GetBalanceResponse, error) {
	bal, err := s.balance(s.DB.WithContext(ctx), uint(req.UserId))
	if err != nil {
		return nil, err
	}
	return &billingpb.GetBalanceResponse{Balance: bal}, nil
}

func (s *Server) balance(db *gorm.DB, uid uint) (int64, error) {
	var u store.User
	if err := db.Select("balance").First(&u, uid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, status.Error(codes.NotFound, "user not found")
		}
		log.Printf("read balance of user %d failed: %v", uid, err)
		return 0, status.Error(codes.Internal, "balance unavailable")
	}
	return u.Balance, nil
}

// maybeAlert publishes a low-balance alert in the background, at most once a minute per user.
func (s *Server) maybeAlert(uid uint, balance int64) {
	if s.Alerts == nil || balance >= s.Threshold {
		return
	}
	s.mu.Lock()
	if s.lastAlert == nil {
		s.lastAlert = map[uint]time.Time{}
	}
	if time.Since(s.lastAlert[uid]) < alertEvery {
		s.mu.Unlock()
		return
	}
	s.lastAlert[uid] = time.Now()
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		m := events.LowBalance{UserID: uid, Balance: balance, Threshold: s.Threshold, At: time.Now().UTC()}
		if err := s.Alerts.Publish(ctx, m); err != nil {
			log.Printf("low-balance alert for user %d not sent: %v", uid, err)
		}
	}()
}
