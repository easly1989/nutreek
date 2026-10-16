package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("empty hash")
	}
	if err := VerifyPassword(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("VerifyPassword (match): %v", err)
	}
	err = VerifyPassword(hash, "wrong password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestHashPasswordValidation(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Error("want an error on a too short password")
	}
	if _, err := HashPassword(strings.Repeat("a", 73)); err == nil {
		t.Error("want an error on a too long password")
	}
}
