package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrSubscriptionNotFound is returned when the user has no subscription.
var ErrSubscriptionNotFound = errors.New("sottoscrizione non trovata")

// Subscriptions is the premium subscriptions repository.
type Subscriptions struct {
	db *sql.DB
}

func NewSubscriptions(db *sql.DB) *Subscriptions {
	return &Subscriptions{db: db}
}

// GetByUser returns the user's subscription or ErrSubscriptionNotFound.
func (s *Subscriptions) GetByUser(ctx context.Context, userID int64) (*domain.Subscription, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, plan, status, source, license_key, started_at, expires_at, COALESCE(notes,'')
		 FROM subscriptions WHERE user_id = ?`, userID)
	return scanSubscription(row)
}

// IsActive reports whether the user has an active, unexpired subscription.
// The expiry comparison happens in Go to avoid ambiguity over the timestamp
// format in SQLite.
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

// Grant creates or reactivates the user's premium subscription (upsert).
// expiresAt nil = no expiry. An empty licenseKey = no license attached.
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

// Revoke deactivates the user's subscription (the row stays for auditing).
func (s *Subscriptions) Revoke(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE subscriptions SET status = 'revoked' WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("revoke subscription: %w", err)
	}
	return nil
}

// MapByUser returns every subscription indexed by user_id,
// for the admin users page.
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

// CountActive counts the subscriptions with status='active' (expiry is not
// evaluated here: it is an indicative counter for the admin dashboard).
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
