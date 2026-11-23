package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

var (
	ErrWorkoutLogNotFound = errors.New("allenamento non trovato")
	ErrActivityNotFound   = errors.New("attività non trovata")
	ErrActivityExists     = errors.New("attività già presente")
)

type WorkoutLogs struct {
	db *sql.DB
}

func NewWorkoutLogs(db *sql.DB) *WorkoutLogs {
	return &WorkoutLogs{db: db}
}

// ListActivities returns the built-in activities, shared by everyone.
func (w *WorkoutLogs) ListActivities(ctx context.Context) ([]domain.WorkoutActivity, error) {
	return w.ListActivitiesFor(ctx, 0)
}

// ListActivitiesFor returns the built-in activities and the ones userID added,
// those last.
func (w *WorkoutLogs) ListActivitiesFor(ctx context.Context, userID int64) ([]domain.WorkoutActivity, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT id, COALESCE(code, ''), name, kcal_per_hour, sort_order, user_id IS NOT NULL
		 FROM workout_activities WHERE user_id IS NULL OR user_id = ?
		 ORDER BY user_id IS NOT NULL, sort_order, name`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workout activities: %w", err)
	}
	defer rows.Close()

	var activities []domain.WorkoutActivity
	for rows.Next() {
		var a domain.WorkoutActivity
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.KcalPerHour, &a.SortOrder, &a.Custom); err != nil {
			return nil, fmt.Errorf("scan workout activity: %w", err)
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

// kcalFor works out the kcal of minutes of an activity for userID, from the
// weight in the profile; ErrActivityNotFound when the activity is neither a
// built-in one nor userID's.
func (w *WorkoutLogs) kcalFor(ctx context.Context, userID, activityID int64, minutes int) (float64, error) {
	var perHour float64
	var weight sql.NullFloat64
	err := w.db.QueryRowContext(ctx,
		`SELECT wa.kcal_per_hour, u.weight_kg FROM workout_activities wa, users u
		 WHERE wa.id = ? AND u.id = ? AND (wa.user_id IS NULL OR wa.user_id = u.id)`,
		activityID, userID).Scan(&perHour, &weight)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrActivityNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("lookup activity kcal: %w", err)
	}
	return domain.WorkoutKcal(perHour, minutes, weight.Float64), nil
}

// nullTime is the stored form of a time of day: NULL when none was given.
func nullTime(atTime string) any {
	if atTime == "" {
		return nil
	}
	return atTime
}

// CreateWorkoutLog logs a workout; atTime is "HH:MM" or "". The kcal come from the
// activity and the weight in the profile.
func (w *WorkoutLogs) CreateWorkoutLog(ctx context.Context, userID, activityID int64, date time.Time, minutes int, atTime string, note *string) (int64, error) {
	kcalBurned, err := w.kcalFor(ctx, userID, activityID, minutes)
	if err != nil {
		return 0, err
	}
	res, err := w.db.ExecContext(ctx,
		`INSERT INTO workout_logs (user_id, activity_id, logged_at, duration_minutes, kcal_burned, note, at_time)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, activityID, date.Format("2006-01-02"), minutes, kcalBurned, note, nullTime(atTime),
	)
	if err != nil {
		return 0, fmt.Errorf("insert workout log: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// CreateActivity adds an activity of userID's own; ErrActivityExists when they
// already have one with that name.
func (w *WorkoutLogs) CreateActivity(ctx context.Context, userID int64, name string, kcalPerHour float64) (int64, error) {
	res, err := w.db.ExecContext(ctx,
		`INSERT INTO workout_activities (name, kcal_per_hour, sort_order, user_id) VALUES (?, ?, 0, ?)`,
		name, kcalPerHour, userID)
	if isUniqueViolation(err) {
		return 0, ErrActivityExists
	}
	if err != nil {
		return 0, fmt.Errorf("insert activity: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (w *WorkoutLogs) ListWorkoutLogs(ctx context.Context, userID int64, from, to time.Time) ([]domain.WorkoutLog, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT wl.id, wl.user_id, wl.activity_id, COALESCE(wa.code, ''), wa.name, wl.logged_at,
		        wl.duration_minutes, wl.kcal_burned, wl.note, wl.created_at, COALESCE(wl.at_time, '')
		 FROM workout_logs wl
		 JOIN workout_activities wa ON wa.id = wl.activity_id
		 WHERE wl.user_id = ? AND wl.logged_at BETWEEN ? AND ?
		 ORDER BY wl.logged_at DESC, COALESCE(wl.at_time, '24:00') DESC, wl.id DESC`,
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
			&dateStr, &l.DurationMinutes, &l.KcalBurned, &note, &l.CreatedAt, &l.AtTime); err != nil {
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

// WorkoutRecent is an activity with a duration the user logged lately: a shortcut
// for the next entry.
type WorkoutRecent struct {
	ActivityID int64
	Code, Name string
	Minutes    int
}

// RecentWorkouts returns the last distinct activity and duration pairs logged by
// userID, most recent first.
func (w *WorkoutLogs) RecentWorkouts(ctx context.Context, userID int64, limit int) ([]WorkoutRecent, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT wl.activity_id, COALESCE(wa.code, ''), wa.name, wl.duration_minutes
		 FROM workout_logs wl
		 JOIN workout_activities wa ON wa.id = wl.activity_id
		 WHERE wl.user_id = ?
		 GROUP BY wl.activity_id, wl.duration_minutes
		 ORDER BY MAX(wl.logged_at) DESC, MAX(wl.id) DESC
		 LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("recent workouts: %w", err)
	}
	defer rows.Close()
	var out []WorkoutRecent
	for rows.Next() {
		var r WorkoutRecent
		if err := rows.Scan(&r.ActivityID, &r.Code, &r.Name, &r.Minutes); err != nil {
			return nil, fmt.Errorf("scan recent workout: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetWorkoutLog returns one workout of userID, ErrWorkoutLogNotFound otherwise.
func (w *WorkoutLogs) GetWorkoutLog(ctx context.Context, id, userID int64) (domain.WorkoutLog, error) {
	var l domain.WorkoutLog
	var dateStr string
	err := w.db.QueryRowContext(ctx,
		`SELECT wl.id, wl.activity_id, COALESCE(wa.code, ''), wa.name, wl.logged_at, wl.duration_minutes, wl.kcal_burned, COALESCE(wl.at_time, '')
		 FROM workout_logs wl JOIN workout_activities wa ON wa.id = wl.activity_id
		 WHERE wl.id = ? AND wl.user_id = ?`, id, userID,
	).Scan(&l.ID, &l.ActivityID, &l.ActivityCode, &l.ActivityName, &dateStr, &l.DurationMinutes, &l.KcalBurned, &l.AtTime)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrWorkoutLogNotFound
	}
	if err != nil {
		return l, fmt.Errorf("get workout log: %w", err)
	}
	l.UserID = userID
	// A DATE column can come back as "2026-10-12T00:00:00Z".
	if len(dateStr) >= 10 {
		l.LoggedAt, _ = time.Parse("2006-01-02", dateStr[:10])
	}
	return l, nil
}

// UpdateWorkoutLog changes the activity, the minutes and the time of a workout
// and works the kcal out again; ErrWorkoutLogNotFound when it is not userID's.
func (w *WorkoutLogs) UpdateWorkoutLog(ctx context.Context, id, userID, activityID int64, minutes int, atTime string) error {
	kcal, err := w.kcalFor(ctx, userID, activityID, minutes)
	if err != nil {
		return err
	}
	res, err := w.db.ExecContext(ctx,
		`UPDATE workout_logs SET activity_id = ?, duration_minutes = ?, kcal_burned = ?, at_time = ? WHERE id = ? AND user_id = ?`,
		activityID, minutes, kcal, nullTime(atTime), id, userID)
	if err != nil {
		return fmt.Errorf("update workout log: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWorkoutLogNotFound
	}
	return nil
}

// CopyWorkoutLogs repeats the workouts of userID on the day from onto the day
// to and returns how many it copied.
func (w *WorkoutLogs) CopyWorkoutLogs(ctx context.Context, userID int64, from, to time.Time) (int, error) {
	res, err := w.db.ExecContext(ctx,
		`INSERT INTO workout_logs (user_id, activity_id, logged_at, duration_minutes, kcal_burned, note, at_time)
		 SELECT user_id, activity_id, ?, duration_minutes, kcal_burned, note, at_time FROM workout_logs
		 WHERE user_id = ? AND logged_at = ?`,
		to.Format("2006-01-02"), userID, from.Format("2006-01-02"))
	if err != nil {
		return 0, fmt.Errorf("copy workout logs: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
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
