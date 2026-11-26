package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrIdentityNotFound means no account is linked to that provider identity.
var ErrIdentityNotFound = errors.New("identity not found")

// Identities links accounts to their identity at a sign-in provider (#333).
type Identities struct{ db *sql.DB }

func NewIdentities(db *sql.DB) *Identities { return &Identities{db: db} }

// UserID returns the account linked to (issuer, subject).
func (r *Identities) UserID(ctx context.Context, issuer, subject string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id FROM user_identities WHERE issuer = ? AND subject = ?`, issuer, subject).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrIdentityNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find identity: %w", err)
	}
	return id, nil
}

// Link ties an account to an identity. An account has one identity per provider, and an
// identity belongs to one account: a second link is an error.
func (r *Identities) Link(ctx context.Context, userID int64, issuer, subject string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO user_identities (user_id, issuer, subject) VALUES (?, ?, ?)`, userID, issuer, subject); err != nil {
		return fmt.Errorf("link identity: %w", err)
	}
	return nil
}
