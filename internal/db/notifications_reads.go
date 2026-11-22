package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// What the notification ticker reads about a user's day (#237).

// MealSlot is a meal or a reminder of the day that has a time.
type MealSlot struct {
	ID    int64
	Name  string
	Start string // HH:MM
	Note  string // a reminder's note
}

// MealSlotsOn returns the meals and reminders of userID with a time, on the day of
// local (its calendar date), in order of time. Those with no time are left out.
func (r *WeeklyPlans) MealSlotsOn(ctx context.Context, userID int64, local time.Time) ([]MealSlot, error) {
	day := (int(local.Weekday()) + 6) % 7
	week := MondayOf(time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)).Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.name, m.start_time, m.note
		FROM meals m JOIN weekly_plans p ON p.id = m.weekly_plan_id
		WHERE p.user_id = ? AND substr(p.week_start, 1, 10) = ? AND m.day_of_week = ? AND m.start_time IS NOT NULL
		ORDER BY m.start_time, m.id`, userID, week, day)
	if err != nil {
		return nil, fmt.Errorf("meal slots: %w", err)
	}
	defer rows.Close()
	out := []MealSlot{}
	for rows.Next() {
		var s MealSlot
		if err := rows.Scan(&s.ID, &s.Name, &s.Start, &s.Note); err != nil {
			return nil, fmt.Errorf("scan meal slot: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// StateOn returns the water logged by userID on the day of local, in ml, and when the
// last of it was logged (zero when there is none).
func (w *WaterLogs) StateOn(ctx context.Context, userID int64, local time.Time) (int, time.Time, error) {
	var total int
	var last sql.NullString
	err := w.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount_ml), 0), MAX(created_at) FROM water_logs WHERE user_id = ? AND logged_at = ?`,
		userID, local.Format("2006-01-02")).Scan(&total, &last)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("water state: %w", err)
	}
	if !last.Valid {
		return total, time.Time{}, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z"} {
		if t, err := time.ParseInLocation(layout, last.String, time.UTC); err == nil {
			return total, t, nil
		}
	}
	return total, time.Time{}, nil
}
