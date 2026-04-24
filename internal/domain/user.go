package domain

import "time"

// User rappresenta un account registrato.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time
}
