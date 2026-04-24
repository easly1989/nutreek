package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials viene restituito quando email/password non matchano.
// Non specifica quale dei due e' errato per non facilitare enumeration.
var ErrInvalidCredentials = errors.New("credenziali non valide")

// bcryptCost e' il costo esponenziale dell'hash. 12 e' un buon trade-off
// 2024/2026 fra sicurezza e latenza (~250ms su CPU consumer).
const bcryptCost = 12

// HashPassword produce un hash bcrypt pronto per la persistenza.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password: minimo 8 caratteri")
	}
	if len(password) > 72 {
		// bcrypt tronca silenziosamente oltre i 72 byte.
		return "", errors.New("password: massimo 72 caratteri")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(h), nil
}

// VerifyPassword restituisce nil se la password matcha l'hash, altrimenti
// ErrInvalidCredentials. Propaga errori non previsti dal bcrypt.
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
