package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

var ErrWorkoutLogNotFound = errors.New("allenamento non trovato")

type WorkoutLogs struct {
	db *sql.DB
}

func NewWorkoutLogs(db *sql.DB) *WorkoutLogs {
	return &WorkoutLogs{db: db}
}

func (w *WorkoutLogs) ListActivities(ctx context.Context) ([]domain.WorkoutActivity, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT id, COALESCE(code, ''), name, kcal_per_hour, sort_order FROM workout_activities ORDER BY sort_order`,
	)
	if err != nil {
		return nil, fmt.Errorf("list workout activities: %w", err)
	}
	defer rows.Close()

	var activities []domain.WorkoutActivity
	for rows.Next() {
		var a domain.WorkoutActivity
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.KcalPerHour, &a.SortOrder); err != nil {
			return nil, fmt.Errorf("scan workout activity: %w", err)
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

func (w *WorkoutLogs) CreateWorkoutLog(ctx context.Context, userID, activityID int64, date time.Time, minutes int, note *string) (int64, error) {
	var kcalPerHour float64
	err := w.db.QueryRowContext(ctx,
		`SELECT kcal_per_hour FROM workout_activities WHERE id = ?`, activityID,
	).Scan(&kcalPerHour)
	if err != nil {
		return 0, fmt.Errorf("lookup activity kcal: %w", err)
	}

	kcalBurned := math.Round((kcalPerHour/60)*float64(minutes)*10) / 10

	res, err := w.db.ExecContext(ctx,
		`INSERT INTO workout_logs (user_id, activity_id, logged_at, duration_minutes, kcal_burned, note)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, activityID, date.Format("2006-01-02"), minutes, kcalBurned, note,
	)
	if err != nil {
		return 0, fmt.Errorf("insert workout log: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (w *WorkoutLogs) ListWorkoutLogs(ctx context.Context, userID int64, from, to time.Time) ([]domain.WorkoutLog, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT wl.id, wl.user_id, wl.activity_id, COALESCE(wa.code, ''), wa.name, wl.logged_at,
		        wl.duration_minutes, wl.kcal_burned, wl.note, wl.created_at
		 FROM workout_logs wl
		 JOIN workout_activities wa ON wa.id = wl.activity_id
		 WHERE wl.user_id = ? AND wl.logged_at BETWEEN ? AND ?
		 ORDER BY wl.logged_at DESC, wl.id DESC`,
		userID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("list workout logs: %w", err)
	}
	defer rows.Close()

	var logs []domain.WorkoutLog
	for rows.Next() {
		var l domain.WorkoutLog
		var dateStr string
		var note sql.NullString
		if err := rows.Scan(&l.ID, &l.UserID, &l.ActivityID, &l.ActivityCode, &l.ActivityName,
			&dateStr, &l.DurationMinutes, &l.KcalBurned, &note, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workout log: %w", err)
		}
		l.LoggedAt, _ = time.Parse("2006-01-02", dateStr)
		if note.Valid {
			l.Note = &note.String
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (w *WorkoutLogs) DeleteWorkoutLog(ctx context.Context, id, userID int64) error {
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM workout_logs WHERE id = ? AND user_id = ?`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete workout log: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrWorkoutLogNotFound
	}
	return nil
}

// SumKcalForWeek returns the total kcal burned in a date range.
func (w *WorkoutLogs) SumKcalForWeek(ctx context.Context, userID int64, from, to time.Time) (float64, error) {
	var sum float64
	err := w.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(kcal_burned), 0) FROM workout_logs
		 WHERE user_id = ? AND logged_at BETWEEN ? AND ?`,
		userID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum workout kcal for week: %w", err)
	}
	return sum, nil
}

// WorkoutKcalForDay returns the total kcal burned on a day.
func (w *WorkoutLogs) WorkoutKcalForDay(ctx context.Context, userID int64, date time.Time) (float64, error) {
	var total sql.NullFloat64
	err := w.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(kcal_burned), 0) FROM workout_logs
		 WHERE user_id = ? AND logged_at = ?`,
		userID, date.Format("2006-01-02"),
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum workout kcal for day: %w", err)
	}
	return total.Float64, nil
}
