package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrPasskeyNotFound means no passkey has that credential id.
var ErrPasskeyNotFound = errors.New("passkey not found")

// MaxPasskeys is how many passkeys one account may keep.
const MaxPasskeys = 10

// Passkey is one WebAuthn credential of an account (#336).
type Passkey struct {
	ID           int64
	UserID       int64
	Handle       []byte
	CredentialID []byte
	Name         string
	Credential   string // the library's JSON for the credential
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// Passkeys keeps the credentials.
type Passkeys struct{ db *sql.DB }

func NewPasskeys(db *sql.DB) *Passkeys { return &Passkeys{db: db} }

const passkeyCols = `id, user_id, user_handle, credential_id, name, credential, created_at, last_used_at`

func scanPasskey(sc interface{ Scan(...any) error }) (Passkey, error) {
	var p Passkey
	var created int64
	var last sql.NullInt64
	if err := sc.Scan(&p.ID, &p.UserID, &p.Handle, &p.CredentialID, &p.Name, &p.Credential, &created, &last); err != nil {
		return Passkey{}, err
	}
	p.CreatedAt = time.Unix(created, 0)
	if last.Valid {
		t := time.Unix(last.Int64, 0)
		p.LastUsedAt = &t
	}
	return p, nil
}

// List returns the account's passkeys, oldest first.
func (r *Passkeys) List(ctx context.Context, userID int64) ([]Passkey, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+passkeyCols+` FROM passkeys WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list passkeys: %w", err)
	}
	defer rows.Close()
	var out []Passkey
	for rows.Next() {
		p, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ByCredentialID finds the passkey a sign-in used.
func (r *Passkeys) ByCredentialID(ctx context.Context, credID []byte) (Passkey, error) {
	p, err := scanPasskey(r.db.QueryRowContext(ctx, `SELECT `+passkeyCols+` FROM passkeys WHERE credential_id = ?`, credID))
	if errors.Is(err, sql.ErrNoRows) {
		return Passkey{}, ErrPasskeyNotFound
	}
	return p, err
}

// Add stores a new passkey. The account keeps at most MaxPasskeys, and every passkey of an
// account carries the same user handle: counting, checking the handle and inserting are one
// step, so two registrations at once cannot split it or pass the limit.
func (r *Passkeys) Add(ctx context.Context, userID int64, handle, credID []byte, name, credential string, signCount uint32, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	var have []byte
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(user_handle), x'') FROM passkeys WHERE user_id = ?`, userID).Scan(&n, &have); err != nil {
		return err
	}
	if n >= MaxPasskeys {
		return errors.New("too many passkeys")
	}
	if n > 0 && !bytes.Equal(have, handle) {
		return errors.New("passkey: the account already has another user handle")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO passkeys (user_id, user_handle, credential_id, name, credential, sign_count, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, handle, credID, name, credential, signCount, now.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// Touch records a sign-in: the credential as the library updated it and the time. The sign
// count can only go forward; it is false when it did not (a parallel sign-in with the same
// count, or a copy of the key). An authenticator that keeps no count always sends 0, and
// then only the time moves.
func (r *Passkeys) Touch(ctx context.Context, id int64, credential string, signCount uint32, now time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE passkeys SET credential = ?, last_used_at = ?, sign_count = MAX(sign_count, ?)
		WHERE id = ? AND (? = 0 OR sign_count < ?)`, credential, now.Unix(), signCount, id, signCount, signCount)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// DeleteAll removes every passkey of the account (a password reset, #334).
func (r *Passkeys) DeleteAll(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM passkeys WHERE user_id = ?`, userID)
	return err
}

// Delete removes one of the account's passkeys; it is false when it is not theirs.
func (r *Passkeys) Delete(ctx context.Context, id, userID int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM passkeys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
