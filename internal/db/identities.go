package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrIdentityNotFound means no account is linked to that provider identity.
var ErrIdentityNotFound = errors.New("identity not found")

// ErrIdentityTaken means the identity is already linked, or the account already has
// an identity at that provider.
var ErrIdentityTaken = errors.New("identity already linked")

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
// identity belongs to one account: a second link is ErrIdentityTaken.
func (r *Identities) Link(ctx context.Context, userID int64, issuer, subject string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO user_identities (user_id, issuer, subject) VALUES (?, ?, ?)`, userID, issuer, subject); err != nil {
		if isUniqueViolation(err) {
			return ErrIdentityTaken
		}
		return fmt.Errorf("link identity: %w", err)
	}
	return nil
}

// UnlinkAll forgets every identity of the account (a password reset, #334): who got in
// through the provider before must prove the account again.
func (r *Identities) UnlinkAll(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_identities WHERE user_id = ?`, userID)
	return err
}

// CreateAccount makes a user and links its identity in one step: an account that cannot
// be linked is not left behind. ErrEmailAlreadyUsed when the email is taken.
func (r *Identities) CreateAccount(ctx context.Context, email, passwordHash, displayName, issuer, subject string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (email, password_hash, display_name) VALUES (?, ?, ?)`,
		strings.ToLower(strings.TrimSpace(email)), passwordHash, displayName)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrEmailAlreadyUsed
		}
		return 0, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_identities (user_id, issuer, subject) VALUES (?, ?, ?)`, id, issuer, subject); err != nil {
		if isUniqueViolation(err) {
			return 0, ErrIdentityTaken
		}
		return 0, fmt.Errorf("link identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}
