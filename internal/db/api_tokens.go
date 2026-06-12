package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrAPITokenNotFound e' restituito quando il token non esiste o non
// appartiene all'utente.
var ErrAPITokenNotFound = errors.New("token API non trovato")

// tokenPrefix identifica i token Nutreek (facilita secret-scanning e revoca).
const tokenPrefix = "nutreek_"

// APITokens e' il repository dei token API personali.
type APITokens struct {
	db    *sql.DB
	users *Users
}

func NewAPITokens(db *sql.DB, users *Users) *APITokens {
	return &APITokens{db: db, users: users}
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// Create genera un nuovo token per l'utente e ritorna il valore in chiaro.
// Il chiaro viene mostrato una sola volta: a DB resta solo SHA-256.
func (t *APITokens) Create(ctx context.Context, userID int64, name string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	plain := tokenPrefix + hex.EncodeToString(raw)
	_, err := t.db.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash) VALUES (?, ?, ?)`,
		userID, name, hashToken(plain))
	if err != nil {
		return "", fmt.Errorf("insert token: %w", err)
	}
	return plain, nil
}

// List ritorna i token dell'utente (senza hash), i piu' recenti per primi.
func (t *APITokens) List(ctx context.Context, userID int64) ([]domain.APIToken, error) {
	rows, err := t.db.QueryContext(ctx,
		`SELECT id, user_id, name, created_at, last_used_at
		 FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()
	var out []domain.APIToken
	for rows.Next() {
		var tok domain.APIToken
		if err := rows.Scan(&tok.ID, &tok.UserID, &tok.Name, &tok.CreatedAt, &tok.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		out = append(out, tok)
	}
	return out, rows.Err()
}

// Revoke cancella il token dell'utente.
func (t *APITokens) Revoke(ctx context.Context, id, userID int64) error {
	res, err := t.db.ExecContext(ctx,
		`DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrAPITokenNotFound
	}
	return nil
}

// Authenticate risolve il token in chiaro nell'utente proprietario e
// aggiorna last_used_at (best-effort). ErrAPITokenNotFound se non valido.
func (t *APITokens) Authenticate(ctx context.Context, plain string) (*domain.User, error) {
	hash := hashToken(plain)
	var userID int64
	err := t.db.QueryRowContext(ctx,
		`SELECT user_id FROM api_tokens WHERE token_hash = ?`, hash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAPITokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate token: %w", err)
	}
	_, _ = t.db.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE token_hash = ?`, hash)
	return t.users.GetByID(ctx, userID)
}
