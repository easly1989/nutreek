package db

import (
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

// Add stores a new passkey. The account keeps at most MaxPasskeys.
func (r *Passkeys) Add(ctx context.Context, userID int64, handle, credID []byte, name, credential string, now time.Time) error {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM passkeys WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return err
	}
	if n >= MaxPasskeys {
		return errors.New("too many passkeys")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO passkeys (user_id, user_handle, credential_id, name, credential, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, handle, credID, name, credential, now.Unix())
	return err
}

// Touch records a sign-in: the credential as the library updated it (its sign count) and the time.
func (r *Passkeys) Touch(ctx context.Context, id int64, credential string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE passkeys SET credential = ?, last_used_at = ? WHERE id = ?`, credential, now.Unix(), id)
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
