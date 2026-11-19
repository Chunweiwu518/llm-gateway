// Package store defines the persistent data model and database helpers.
package store

import (
	"time"

	"gorm.io/datatypes"
)

// Roles.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// MicroPerUSD is the number of balance units in one US dollar.
const MicroPerUSD int64 = 1_000_000

// User is a developer or administrator account. Balance is in micro-USD.
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"size:191;uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"size:100;not null" json:"-"`
	Role         string    `gorm:"size:10;not null;default:user" json:"role"`
	TOTPSecret   *string   `gorm:"size:64" json:"-"`
	TOTPEnabled  bool      `gorm:"not null;default:false" json:"totp_enabled"`
	Balance      int64     `gorm:"not null;default:0" json:"balance"`
	Disabled     bool      `gorm:"not null;default:false" json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
}

// APIKey is a gateway key. Only its SHA-256 hash is stored.
type APIKey struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"index;not null" json:"user_id"`
	Name       string     `gorm:"size:100;not null" json:"name"`
	KeyHash    string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	KeyPrefix  string     `gorm:"size:16;not null" json:"key_prefix"`
	RPMLimit   int        `gorm:"not null;default:60" json:"rpm_limit"`
	Disabled   bool       `gorm:"not null;default:false" json:"disabled"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Channel types.
const (
	ChannelOpenAI    = "openai"
	ChannelAnthropic = "anthropic"
)

// Channel is an upstream provider endpoint.
type Channel struct {
	ID       uint                                  `gorm:"primaryKey" json:"id"`
	Name     string                                `gorm:"size:100;not null" json:"name"`
	Type     string                                `gorm:"size:20;not null" json:"type"`
	BaseURL  string                                `gorm:"size:255;not null" json:"base_url"`
	APIKey   string                                `gorm:"size:255;not null" json:"-"`
	Models   datatypes.JSONSlice[string]           `json:"models"`
	ModelMap datatypes.JSONType[map[string]string] `json:"model_map"`
	Priority int                                   `gorm:"not null;default:1" json:"priority"`
	Enabled  bool                                  `gorm:"not null;default:true" json:"enabled"`
}

// ModelPrice is the price per one million tokens, in micro-USD.
type ModelPrice struct {
	Model      string `gorm:"primaryKey;size:100" json:"model"`
	InputPerM  int64  `gorm:"not null" json:"input_per_m"`
	OutputPerM int64  `gorm:"not null" json:"output_per_m"`
}

// UsageLog records one /v1 call.
type UsageLog struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	UserID           uint      `gorm:"index:idx_usage_user_time,priority:1;not null" json:"user_id"`
	APIKeyID         uint      `gorm:"not null" json:"api_key_id"`
	ChannelID        *uint     `json:"channel_id"`
	ChannelName      string    `gorm:"size:100" json:"channel_name"`
	Model            string    `gorm:"size:100;not null" json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	Cost             int64     `json:"cost"`
	StatusCode       int       `json:"status_code"`
	LatencyMs        int64     `json:"latency_ms"`
	FailoverCount    int       `json:"failover_count"`
	CreatedAt        time.Time `gorm:"index:idx_usage_user_time,priority:2" json:"created_at"`
}

// Topup sources.
const (
	TopupSelf  = "self"
	TopupAdmin = "admin"
)

// Topup records a balance change from a top-up or an admin adjustment.
type Topup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Amount    int64     `gorm:"not null" json:"amount"`
	Source    string    `gorm:"size:10;not null" json:"source"`
	CreatedAt time.Time `json:"created_at"`
}
