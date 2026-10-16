package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrWaterLogNotFound is returned when there is no log to delete.
var ErrWaterLogNotFound = errors.New("log idratazione non trovato")

type WaterLogs struct {
	db *sql.DB
}

func NewWaterLogs(db *sql.DB) *WaterLogs {
	return &WaterLogs{db: db}
}

// Log records a water intake on the given date. Every call is a new record
// (the "+250ml" quick logs add up in the daily total).
func (w *WaterLogs) Log(ctx context.Context, userID int64, date time.Time, amountML int) error {
	if amountML <= 0 {
		return fmt.Errorf("quantita' non valida")
	}
	_, err := w.db.ExecContext(ctx,
		`INSERT INTO water_logs (user_id, logged_at, amount_ml) VALUES (?, ?, ?)`,
		userID, date.Format("2006-01-02"), amountML,
	)
	if err != nil {
		return fmt.Errorf("insert water log: %w", err)
	}
	return nil
}

// TotalForDate returns the total ml the user drank on a date.
func (w *WaterLogs) TotalForDate(ctx context.Context, userID int64, date time.Time) (int, error) {
	var total int
	err := w.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount_ml), 0) FROM water_logs WHERE user_id = ? AND logged_at = ?`,
		userID, date.Format("2006-01-02"),
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("total water for date: %w", err)
	}
	return total, nil
}

// ForDateRange returns a map date(YYYY-MM-DD) -> total ml for the inclusive
// range [from, to]. Dates without logs are absent from the map.
func (w *WaterLogs) ForDateRange(ctx context.Context, userID int64, from, to time.Time) (map[string]int, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT logged_at, SUM(amount_ml) FROM water_logs
		 WHERE user_id = ? AND logged_at BETWEEN ? AND ?
		 GROUP BY logged_at`,
		userID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("water for range: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var day string
		var total int
		if err := rows.Scan(&day, &total); err != nil {
			return nil, fmt.Errorf("scan water range: %w", err)
		}
		out[day] = total
	}
	return out, rows.Err()
}

// DeleteLast deletes the day's latest log (undo for the quick log).
// It returns ErrWaterLogNotFound when there is nothing to delete.
func (w *WaterLogs) DeleteLast(ctx context.Context, userID int64, date time.Time) error {
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM water_logs WHERE id = (
			SELECT id FROM water_logs
			WHERE user_id = ? AND logged_at = ?
			ORDER BY id DESC LIMIT 1
		)`,
		userID, date.Format("2006-01-02"),
	)
	if err != nil {
		return fmt.Errorf("delete last water log: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWaterLogNotFound
	}
	return nil
}
