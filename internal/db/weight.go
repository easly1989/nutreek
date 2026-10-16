package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

var ErrWeightLogNotFound = errors.New("misurazione peso non trovata")

type WeightLogs struct {
	db *sql.DB
}

func NewWeightLogs(db *sql.DB) *WeightLogs {
	return &WeightLogs{db: db}
}

func (w *WeightLogs) LogWeight(ctx context.Context, userID int64, date time.Time, weightKg float64, note *string) (int64, error) {
	dateStr := date.Format("2006-01-02")
	res, err := w.db.ExecContext(ctx,
		`INSERT INTO weight_logs (user_id, measured_at, weight_kg, note)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id, measured_at) DO UPDATE SET weight_kg=excluded.weight_kg, note=excluded.note`,
		userID, dateStr, weightKg, note,
	)
	if err != nil {
		return 0, fmt.Errorf("insert weight log: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func (w *WeightLogs) ListWeightLogs(ctx context.Context, userID int64, limit int) ([]domain.WeightLog, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT id, user_id, measured_at, weight_kg, note, created_at
		 FROM weight_logs
		 WHERE user_id = ?
		 ORDER BY measured_at DESC
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list weight logs: %w", err)
	}
	defer rows.Close()

	var logs []domain.WeightLog
	for rows.Next() {
		var l domain.WeightLog
		var dateStr string
		var note sql.NullString
		if err := rows.Scan(&l.ID, &l.UserID, &dateStr, &l.WeightKg, &note, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan weight log: %w", err)
		}
		l.MeasuredAt = parseWeightDate(dateStr)
		if note.Valid {
			l.Note = &note.String
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ForRange returns every weight log of the user in a date range (bounds
// included), sorted by ascending date.
func (w *WeightLogs) ForRange(ctx context.Context, userID int64, from, to time.Time) ([]domain.WeightLog, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT id, user_id, measured_at, weight_kg, note, created_at
		 FROM weight_logs
		 WHERE user_id = ? AND measured_at >= ? AND measured_at <= ?
		 ORDER BY measured_at ASC`,
		userID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("weight for range: %w", err)
	}
	defer rows.Close()
	var logs []domain.WeightLog
	for rows.Next() {
		var l domain.WeightLog
		var dateStr string
		var note sql.NullString
		if err := rows.Scan(&l.ID, &l.UserID, &dateStr, &l.WeightKg, &note, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan weight log: %w", err)
		}
		l.MeasuredAt = parseWeightDate(dateStr)
		if note.Valid {
			l.Note = &note.String
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (w *WeightLogs) DeleteWeightLog(ctx context.Context, id, userID int64) error {
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM weight_logs WHERE id = ? AND user_id = ?`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete weight log: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWeightLogNotFound
	}
	return nil
}

// parseWeightDate reads measured_at: the driver may return the DATE column
// as "2026-10-13" or as "2026-10-13T00:00:00Z".
func parseWeightDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", strings.Split(s, "T")[0])
	return t
}
