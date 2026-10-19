package db

import (
	"context"
	"database/sql"
	"errors"
)

// InstanceSettings manages the settings of the whole installation (#167).
type InstanceSettings struct {
	db *sql.DB
}

// NewInstanceSettings creates an InstanceSettings repository.
func NewInstanceSettings(db *sql.DB) *InstanceSettings {
	return &InstanceSettings{db: db}
}

// Get returns the value of key, and false when it was never set.
func (s *InstanceSettings) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM instance_settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// Set stores value under key.
func (s *InstanceSettings) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO instance_settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
