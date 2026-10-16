package domain

import "time"

// APIToken is a personal API access token. The plaintext value is never
// stored: the DB keeps only the SHA-256 hash.
type APIToken struct {
	ID         int64
	UserID     int64
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}
