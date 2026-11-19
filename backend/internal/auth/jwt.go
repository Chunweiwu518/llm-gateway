// Package auth implements console login with password, TOTP and roles.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token stages.
const (
	StageFull        = "full"
	StagePendingTOTP = "pending_totp"
)

// Session lifetimes.
const (
	SessionTTL = 12 * time.Hour
	PendingTTL = 5 * time.Minute
)

// Claims is the JWT payload.
type Claims struct {
	Role  string `json:"role"`
	Stage string `json:"stage"`
	jwt.RegisteredClaims
}

// Manager issues and verifies HS256 tokens.
type Manager struct {
	secret []byte
}

// NewManager returns a token manager for the given secret.
func NewManager(secret string) *Manager { return &Manager{secret: []byte(secret)} }

// Issue signs a token for the user.
func (m *Manager) Issue(userID uint, role, stage string, ttl time.Duration) (string, error) {
	now := time.Now()
	c := Claims{Role: role, Stage: stage, RegisteredClaims: jwt.RegisteredClaims{
		Subject:   fmt.Sprint(userID),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
}

// Parse verifies a token and returns its claims.
func (m *Manager) Parse(tok string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(tok, c, func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, errors.New("invalid token")
	}
	return c, nil
}
