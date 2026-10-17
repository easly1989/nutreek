package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned when email/password do not match.
// It does not say which of the two is wrong, so as not to help enumeration.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Password length limits: handlers map these to a message in the user's language.
var (
	ErrPasswordTooShort = errors.New("password: minimum 8 characters")
	ErrPasswordTooLong  = errors.New("password: maximum 72 characters")
)

// bcryptCost is the hash's exponential cost. 12 is a good 2024/2026
// trade-off between security and latency (~250ms on a consumer CPU).
const bcryptCost = 12

// HashPassword produces a bcrypt hash ready to be stored.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", ErrPasswordTooShort
	}
	if len(password) > 72 {
		// bcrypt silently truncates beyond 72 bytes.
		return "", ErrPasswordTooLong
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(h), nil
}

// VerifyPassword returns nil when the password matches the hash, otherwise
// ErrInvalidCredentials. It propagates unexpected bcrypt errors.
func VerifyPassword(hash, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrInvalidCredentials
	}
	return fmt.Errorf("bcrypt compare: %w", err)
}
