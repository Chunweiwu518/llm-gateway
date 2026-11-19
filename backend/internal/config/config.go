// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"os"
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
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from the environment and validates required values.
func Load() (Config, error) {
	timeout, err := time.ParseDuration(env("UPSTREAM_TIMEOUT", "30s"))
	if err != nil {
		return Config{}, errors.New("invalid UPSTREAM_TIMEOUT")
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
	}
	if c.DSN == "" {
		return c, errors.New("DB_DSN is required")
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be at least 32 characters")
	}
	return c, nil
}
