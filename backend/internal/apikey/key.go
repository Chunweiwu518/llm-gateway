// Package apikey manages gateway API keys and authenticates /v1 requests.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// Generate returns a new key "sk-" + 48 random URL-safe characters,
// its SHA-256 hex hash, and a short display prefix.
func Generate() (plain, hash, prefix string, err error) {
	b := make([]byte, 36)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	plain = "sk-" + base64.RawURLEncoding.EncodeToString(b)
	return plain, Hash(plain), plain[:8], nil
}

// Hash returns the SHA-256 hex digest stored in the database.
func Hash(plain string) string {
	s := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(s[:])
}
