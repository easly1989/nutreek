package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amon2126/nutreek/internal/mfa"
)

// MaxMFAFailures wrong codes in a row lock the second step for MFALock.
const (
	MaxMFAFailures = 5
	MFALock        = 5 * time.Minute
)

// TwoFactorState is what an account has set up for the second step (#335). The zero value
// is an account with nothing.
type TwoFactorState struct {
	TOTPSecret       string // sealed
	TOTPOn           bool
	LastStep         int64
	EmailOn          bool
	EmailCodeSalt    string
	EmailCodeHash    string
	EmailCodeExpires int64
	EmailCodeSent    int64
	Failed           int
	LockedUntil      int64
}

// Enabled says whether signing in asks for a second step.
func (s TwoFactorState) Enabled() bool { return s.TOTPOn || s.EmailOn }

// Locked says whether wrong codes have locked the step.
func (s TwoFactorState) Locked(now time.Time) bool { return s.LockedUntil > now.Unix() }

// TwoFactor keeps the second step of each account.
type TwoFactor struct{ db *sql.DB }

func NewTwoFactor(db *sql.DB) *TwoFactor { return &TwoFactor{db: db} }

// Get returns the account's state; an account with no row has the zero state.
func (r *TwoFactor) Get(ctx context.Context, userID int64) (TwoFactorState, error) {
	var s TwoFactorState
	var totp, email int
	err := r.db.QueryRowContext(ctx, `SELECT totp_secret, totp_on, last_step, email_on, email_code_salt, email_code_hash,
		email_code_expires, email_code_sent, failed, locked_until FROM user_2fa WHERE user_id = ?`, userID).
		Scan(&s.TOTPSecret, &totp, &s.LastStep, &email, &s.EmailCodeSalt, &s.EmailCodeHash, &s.EmailCodeExpires,
			&s.EmailCodeSent, &s.Failed, &s.LockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return TwoFactorState{}, nil
	}
	if err != nil {
		return TwoFactorState{}, fmt.Errorf("read 2fa: %w", err)
	}
	s.TOTPOn, s.EmailOn = totp == 1, email == 1
	return s, nil
}

func (r *TwoFactor) ensure(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO user_2fa (user_id) VALUES (?)`, userID)
	return err
}

// SetPendingTOTP stores a sealed secret the person has not confirmed yet.
func (r *TwoFactor) SetPendingTOTP(ctx context.Context, userID int64, sealed string) error {
	if err := r.ensure(ctx, userID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET totp_secret = ?, totp_on = 0, last_step = 0 WHERE user_id = ?`, sealed, userID)
	return err
}

// EnableTOTP turns the app on, remembering the step of the code that confirmed it.
func (r *TwoFactor) EnableTOTP(ctx context.Context, userID, step int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET totp_on = 1, last_step = ?, failed = 0, locked_until = 0 WHERE user_id = ? AND totp_secret != ''`, step, userID)
	return err
}

// DisableTOTP forgets the secret and turns the app off.
func (r *TwoFactor) DisableTOTP(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET totp_secret = '', totp_on = 0, last_step = 0 WHERE user_id = ?`, userID)
	return err
}

// UseStep records a time-based code as used. It is false when the step (or a later one)
// was already taken, so two requests with the same code cannot both pass.
func (r *TwoFactor) UseStep(ctx context.Context, userID, step int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET last_step = ? WHERE user_id = ? AND last_step < ?`, step, userID, step)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// SetEmailOn turns the email code on or off for sign-in.
func (r *TwoFactor) SetEmailOn(ctx context.Context, userID int64, on bool) error {
	if err := r.ensure(ctx, userID); err != nil {
		return err
	}
	v := 0
	if on {
		v = 1
	}
	_, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET email_on = ?, email_code_hash = '', email_code_salt = '', email_code_expires = 0 WHERE user_id = ?`, v, userID)
	return err
}

// StoreEmailCode keeps the hash of a code just mailed, with its expiry.
func (r *TwoFactor) StoreEmailCode(ctx context.Context, userID int64, salt, hash string, expires, sent time.Time) error {
	if err := r.ensure(ctx, userID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET email_code_salt = ?, email_code_hash = ?, email_code_expires = ?, email_code_sent = ? WHERE user_id = ?`,
		salt, hash, expires.Unix(), sent.Unix(), userID)
	return err
}

// UseEmailCode spends the mailed code if the hash matches and it has not expired. It
// reports whether it did; a code works once.
func (r *TwoFactor) UseEmailCode(ctx context.Context, userID int64, code string, now time.Time) (bool, error) {
	s, err := r.Get(ctx, userID)
	if err != nil || s.EmailCodeHash == "" || s.EmailCodeExpires <= now.Unix() {
		return false, err
	}
	if !mfa.Equal(mfa.HashCode(s.EmailCodeSalt, code), s.EmailCodeHash) {
		return false, nil
	}
	res, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET email_code_hash = '', email_code_salt = '', email_code_expires = 0
		WHERE user_id = ? AND email_code_hash = ?`, userID, s.EmailCodeHash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Fail counts a wrong code; the fifth in a row locks the step for MFALock.
func (r *TwoFactor) Fail(ctx context.Context, userID int64, now time.Time) (TwoFactorState, error) {
	if err := r.ensure(ctx, userID); err != nil {
		return TwoFactorState{}, err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET failed = failed + 1,
		locked_until = CASE WHEN failed + 1 >= ? THEN ? ELSE locked_until END WHERE user_id = ?`,
		MaxMFAFailures, now.Add(MFALock).Unix(), userID); err != nil {
		return TwoFactorState{}, err
	}
	return r.Get(ctx, userID)
}

// ResetFailures clears the count after a right code.
func (r *TwoFactor) ResetFailures(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_2fa SET failed = 0, locked_until = 0 WHERE user_id = ?`, userID)
	return err
}

// ReplaceRecoveryCodes stores a new set of codes (as given, plain) and forgets the old.
func (r *TwoFactor) ReplaceRecoveryCodes(ctx context.Context, userID int64, codes []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, c := range codes {
		salt, err := mfa.NewSalt()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO recovery_codes (user_id, salt, hash) VALUES (?, ?, ?)`, userID, salt, mfa.HashCode(salt, c)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteRecoveryCodes forgets every code (the last method was turned off).
func (r *TwoFactor) DeleteRecoveryCodes(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID)
	return err
}

// RecoveryLeft is how many codes are still unused.
func (r *TwoFactor) RecoveryLeft(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID).Scan(&n)
	return n, err
}

// UseRecoveryCode spends a recovery code (already normalised). Every unused code is
// compared, so the time taken does not say which one matched.
func (r *TwoFactor) UseRecoveryCode(ctx context.Context, userID int64, code string, now time.Time) (bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, salt, hash FROM recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var hit int64
	for rows.Next() {
		var id int64
		var salt, hash string
		if err := rows.Scan(&id, &salt, &hash); err != nil {
			return false, err
		}
		if mfa.Equal(mfa.HashCode(salt, code), hash) {
			hit = id
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	_ = rows.Close()
	if hit == 0 {
		return false, nil
	}
	res, err := r.db.ExecContext(ctx, `UPDATE recovery_codes SET used_at = ? WHERE id = ? AND used_at IS NULL`, now.Unix(), hit)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
