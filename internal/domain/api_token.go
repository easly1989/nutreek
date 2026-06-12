package domain

import "time"

// APIToken e' un token di accesso API personale. Il valore in chiaro non
// viene mai persistito: a DB resta solo l'hash SHA-256.
type APIToken struct {
	ID         int64
	UserID     int64
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}
