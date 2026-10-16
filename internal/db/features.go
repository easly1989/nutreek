package db

import (
	"context"
	"database/sql"

	"github.com/amon2126/nutreek/internal/domain"
)

// Features manages the features table (feature flags).
type Features struct {
	db *sql.DB
}

// NewFeatures creates a Features repository.
func NewFeatures(db *sql.DB) *Features {
	return &Features{db: db}
}

// List returns every feature ordered by sort_order.
func (f *Features) List(ctx context.Context) ([]domain.Feature, error) {
	rows, err := f.db.QueryContext(ctx,
		`SELECT key, name, COALESCE(description,''), is_premium, enabled, sort_order
		 FROM features ORDER BY sort_order, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Feature
	for rows.Next() {
		var feat domain.Feature
		var isPremium, enabled int
		if err := rows.Scan(&feat.Key, &feat.Name, &feat.Description,
			&isPremium, &enabled, &feat.SortOrder); err != nil {
			return nil, err
		}
		feat.IsPremium = isPremium == 1
		feat.Enabled = enabled == 1
		out = append(out, feat)
	}
	return out, rows.Err()
}

// SetEnabled sets a feature's global kill switch.
func (f *Features) SetEnabled(ctx context.Context, key string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := f.db.ExecContext(ctx,
		`UPDATE features SET enabled = ? WHERE key = ?`, v, key)
	return err
}

// SetPremium changes a feature's is_premium flag.
func (f *Features) SetPremium(ctx context.Context, key string, premium bool) error {
	v := 0
	if premium {
		v = 1
	}
	_, err := f.db.ExecContext(ctx,
		`UPDATE features SET is_premium = ? WHERE key = ?`, v, key)
	return err
}
