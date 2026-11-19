package auth

import (
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Issuer is shown in authenticator apps.
const Issuer = "LLM Gateway"

// NewTOTPKey generates a new TOTP secret for the account.
func NewTOTPKey(email string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{Issuer: Issuer, AccountName: email})
}

// ValidTOTP reports whether code is valid for secret at the current time.
func ValidTOTP(code, secret string) bool {
	return totp.Validate(code, secret)
}
