// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings for the gateway.
type Config struct {
	DSN             string
	RedisAddr       string
	JWTSecret       string
	AdminEmail      string
	AdminPassword   string
	ListenAddr      string
	MockUpstreamURL string
	UpstreamTimeout time.Duration
	CookieSecure    bool

	// BillingAddr is the gRPC billing service (host:port). Empty settles in-process.
	BillingAddr string
	// KafkaBrokers receive usage events. Empty writes usage rows synchronously.
	KafkaBrokers []string
	KafkaTopic   string

	// MySQL pool (DBMaxOpen <= 0 keeps the database/sql defaults) and the
	// minimum gap between two last_used_at writes for one key (0 = every request).
	DBMaxOpen     int
	DBMaxIdle     int
	DBMaxLifetime time.Duration
	LastUsedEvery time.Duration
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, errors.New("invalid " + key)
	}
	return n, nil
}

func envDuration(key, def string) (time.Duration, error) {
	d, err := time.ParseDuration(env(key, def))
	if err != nil {
		return 0, errors.New("invalid " + key)
	}
	return d, nil
}

// Brokers splits a comma separated broker list, dropping blanks.
func Brokers(list string) []string {
	var out []string
	for _, b := range strings.Split(list, ",") {
		if b = strings.TrimSpace(b); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// Load reads configuration from the environment and validates required values.
func Load() (Config, error) {
	timeout, err := time.ParseDuration(env("UPSTREAM_TIMEOUT", "30s"))
	if err != nil {
		return Config{}, errors.New("invalid UPSTREAM_TIMEOUT")
	}
	maxOpen, err := envInt("DB_MAX_OPEN_CONNS", 64)
	if err != nil {
		return Config{}, err
	}
	maxIdle, err := envInt("DB_MAX_IDLE_CONNS", 64)
	if err != nil {
		return Config{}, err
	}
	lifetime, err := envDuration("DB_CONN_MAX_LIFETIME", "5m")
	if err != nil {
		return Config{}, err
	}
	lastUsed, err := envDuration("LAST_USED_INTERVAL", "1m")
	if err != nil {
		return Config{}, err
	}
	c := Config{
		DSN:             os.Getenv("DB_DSN"),
		RedisAddr:       env("REDIS_ADDR", "localhost:6379"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AdminEmail:      os.Getenv("ADMIN_EMAIL"),
		AdminPassword:   os.Getenv("ADMIN_PASSWORD"),
		ListenAddr:      env("LISTEN_ADDR", ":8080"),
		MockUpstreamURL: env("MOCK_UPSTREAM_URL", "http://mockupstream:9000"),
		UpstreamTimeout: timeout,
		CookieSecure:    os.Getenv("COOKIE_SECURE") == "true",
		BillingAddr:     os.Getenv("BILLING_ADDR"),
		KafkaBrokers:    Brokers(os.Getenv("KAFKA_BROKERS")),
		KafkaTopic:      env("KAFKA_USAGE_TOPIC", "usage.events"),
		DBMaxOpen:       maxOpen,
		DBMaxIdle:       maxIdle,
		DBMaxLifetime:   lifetime,
		LastUsedEvery:   lastUsed,
	}
	if c.DSN == "" {
		return c, errors.New("DB_DSN is required")
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be at least 32 characters")
	}
	return c, nil
}
