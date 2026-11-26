package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrResetNotFound means the link is unknown, used or expired.
var ErrResetNotFound = errors.New("password reset not found")

// ErrResetRecent means the account got a link a moment ago: asking again does not replace
// it, so nobody can keep an account's link from working by asking over and over.
var ErrResetRecent = errors.New("a reset link was sent a moment ago")

// PasswordResets keeps the links to choose a new password (#334).
type PasswordResets struct{ db *sql.DB }

func NewPasswordResets(db *sql.DB) *PasswordResets { return &PasswordResets{db: db} }

// Create stores the hash of a new token for the account, replacing the one it had, and
// forgets every link that has expired. If the account's link is younger than gap it is
// kept and ErrResetRecent is returned.
func (r *PasswordResets) Create(ctx context.Context, userID int64, tokenHash string, expires time.Time, gap time.Duration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var recent int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM password_resets WHERE user_id = ? AND expires_at > ?`,
		userID, expires.Add(-gap).Unix()).Scan(&recent); err != nil {
		return fmt.Errorf("check recent reset: %w", err)
	}
	if recent > 0 {
		return ErrResetRecent
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_resets WHERE user_id = ? OR expires_at < ?`, userID, time.Now().Unix()); err != nil {
		return fmt.Errorf("clear resets: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES (?, ?, ?)`, userID, tokenHash, expires.Unix()); err != nil {
		return fmt.Errorf("insert reset: %w", err)
	}
	return tx.Commit()
}

// Valid reports whether the link can still be used, without using it.
func (r *PasswordResets) Valid(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM password_resets WHERE token_hash = ? AND expires_at > ?`, tokenHash, now.Unix()).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check reset: %w", err)
	}
	return n > 0, nil
}

// Consume uses the link up and returns its account. A second use finds nothing.
func (r *PasswordResets) Consume(ctx context.Context, tokenHash string, now time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`DELETE FROM password_resets WHERE token_hash = ? AND expires_at > ? RETURNING user_id`, tokenHash, now.Unix()).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrResetNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("use reset: %w", err)
	}
	return id, nil
}
