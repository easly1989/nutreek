package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrUserNotFound e' restituito dalle Get quando la riga non esiste.
var ErrUserNotFound = errors.New("user non trovato")

// ErrEmailAlreadyUsed e' restituito da CreateUser quando l'email esiste gia'.
var ErrEmailAlreadyUsed = errors.New("email gia' registrata")

// Users e' il repository degli utenti.
type Users struct {
	db *sql.DB
}

func NewUsers(db *sql.DB) *Users {
	return &Users{db: db}
}

// Create inserisce un nuovo utente. Ritorna l'id assegnato dal DB.
// Se l'email e' gia' presente ritorna ErrEmailAlreadyUsed.
func (u *Users) Create(ctx context.Context, email, passwordHash, displayName string) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	res, err := u.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES (?, ?, ?)`,
		email, passwordHash, displayName,
	)
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
	return id, nil
}

// GetByEmail ritorna l'utente con l'email data o ErrUserNotFound.
func (u *Users) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row := u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, display_name, created_at
		 FROM users WHERE email = ?`, email,
	)
	return scanUser(row)
}

// GetByID ritorna l'utente con l'id dato o ErrUserNotFound.
func (u *Users) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row := u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, display_name, created_at
		 FROM users WHERE id = ?`, id,
	)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DisplayName, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &user, nil
}

// isUniqueViolation identifica i codici errore SQLite per UNIQUE constraint.
// modernc.org/sqlite espone *sqlite.Error con Code() int.
func isUniqueViolation(err error) bool {
	var sqlErr *sqlite.Error
	if !errors.As(err, &sqlErr) {
		return false
	}
	code := sqlErr.Code()
	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
