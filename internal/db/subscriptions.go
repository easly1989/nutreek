package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrSubscriptionNotFound e' restituito quando l'utente non ha sottoscrizioni.
var ErrSubscriptionNotFound = errors.New("sottoscrizione non trovata")

// Subscriptions e' il repository delle sottoscrizioni premium.
type Subscriptions struct {
	db *sql.DB
}

func NewSubscriptions(db *sql.DB) *Subscriptions {
	return &Subscriptions{db: db}
}

// GetByUser ritorna la sottoscrizione dell'utente o ErrSubscriptionNotFound.
func (s *Subscriptions) GetByUser(ctx context.Context, userID int64) (*domain.Subscription, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, plan, status, source, license_key, started_at, expires_at, COALESCE(notes,'')
		 FROM subscriptions WHERE user_id = ?`, userID)
	return scanSubscription(row)
}

// IsActive ritorna true se l'utente ha una sottoscrizione attiva e non scaduta.
// Il confronto scadenza avviene in Go per evitare ambiguita' di formato
// timestamp in SQLite.
func (s *Subscriptions) IsActive(ctx context.Context, userID int64) (bool, error) {
	sub, err := s.GetByUser(ctx, userID)
	if errors.Is(err, ErrSubscriptionNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return sub.IsActive(time.Now()), nil
}

// Grant crea o riattiva la sottoscrizione premium dell'utente (upsert).
// expiresAt nil = senza scadenza. licenseKey vuota = nessuna licenza associata.
func (s *Subscriptions) Grant(ctx context.Context, userID int64, source string, expiresAt *time.Time, licenseKey, notes string) error {
	var key *string
	if licenseKey != "" {
		key = &licenseKey
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO subscriptions (user_id, plan, status, source, license_key, expires_at, notes)
		 VALUES (?, 'premium', 'active', ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   status      = 'active',
		   source      = excluded.source,
		   license_key = excluded.license_key,
		   expires_at  = excluded.expires_at,
		   notes       = excluded.notes,
		   started_at  = CURRENT_TIMESTAMP`,
		userID, source, key, expiresAt, notes)
	if err != nil {
		return fmt.Errorf("grant subscription: %w", err)
	}
	return nil
}

// Revoke disattiva la sottoscrizione dell'utente (la riga resta per audit).
func (s *Subscriptions) Revoke(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE subscriptions SET status = 'revoked' WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("revoke subscription: %w", err)
	}
	return nil
}

// MapByUser ritorna tutte le sottoscrizioni indicizzate per user_id,
// per la pagina admin utenti.
func (s *Subscriptions) MapByUser(ctx context.Context) (map[int64]*domain.Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, plan, status, source, license_key, started_at, expires_at, COALESCE(notes,'')
		 FROM subscriptions`)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]*domain.Subscription)
	for rows.Next() {
		var sub domain.Subscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Plan, &sub.Status, &sub.Source,
			&sub.LicenseKey, &sub.StartedAt, &sub.ExpiresAt, &sub.Notes); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		out[sub.UserID] = &sub
	}
	return out, rows.Err()
}

// CountActive conta le sottoscrizioni con status='active' (la scadenza non
// viene valutata qui: e' un contatore indicativo per la dashboard admin).
func (s *Subscriptions) CountActive(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscriptions WHERE status = 'active'`).Scan(&n)
	return n, err
}

func scanSubscription(row *sql.Row) (*domain.Subscription, error) {
	var sub domain.Subscription
	err := row.Scan(&sub.ID, &sub.UserID, &sub.Plan, &sub.Status, &sub.Source,
		&sub.LicenseKey, &sub.StartedAt, &sub.ExpiresAt, &sub.Notes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan subscription: %w", err)
	}
	return &sub, nil
}
