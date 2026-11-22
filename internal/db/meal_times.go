package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ErrTooLong is returned for a meal name or a reminder's note over 200 characters.
var ErrTooLong = errors.New("too long")

// Meal times and reminders (#236). Times are wall-clock «HH:MM» with no time zone.

// ErrInvalidTime is returned for a time that is not «HH:MM» (00:00 to 23:59), an
// end that is not after the start, or a start that is missing where it is required.
var ErrInvalidTime = errors.New("invalid time")

var hhmm = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// ValidTimes checks a start (required) and an optional end ("" for none).
func ValidTimes(start, end string) error {
	if !hhmm.MatchString(start) {
		return ErrInvalidTime
	}
	if end != "" && (!hhmm.MatchString(end) || end <= start) {
		return ErrInvalidTime
	}
	return nil
}

// nullIfEmpty stores "" as NULL.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// MealTimeDefault is the time a user set for a meal name.
type MealTimeDefault struct {
	Name       string
	Start, End string
}

// MealTimes is a user's defaults, found by meal name in any case.
type MealTimes struct {
	byName map[string]MealTimeDefault
	List   []MealTimeDefault // by start time, then name
}

// For returns the start and end (NULL-able, for a query) of a meal name.
func (t MealTimes) For(name string) (start, end any) {
	d, ok := t.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, nil
	}
	return d.Start, nullIfEmpty(d.End)
}

// Label is the time of a meal name as shown («12:30», «12:30–13:30»), "" with none.
func (t MealTimes) Label(name string) string {
	d, ok := t.byName[strings.ToLower(strings.TrimSpace(name))]
	switch {
	case !ok:
		return ""
	case d.End == "":
		return d.Start
	}
	return d.Start + "–" + d.End
}

// Lookup is For with strings.
func (t MealTimes) Lookup(name string) (MealTimeDefault, bool) {
	d, ok := t.byName[strings.ToLower(strings.TrimSpace(name))]
	return d, ok
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func mealTimeDefaults(ctx context.Context, q queryer, userID int64) (MealTimes, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT name, start_time, COALESCE(end_time, '') FROM meal_time_defaults
		WHERE user_id = ? ORDER BY start_time, name`, userID)
	if err != nil {
		return MealTimes{}, fmt.Errorf("meal time defaults: %w", err)
	}
	defer rows.Close()
	out := MealTimes{byName: map[string]MealTimeDefault{}}
	for rows.Next() {
		var d MealTimeDefault
		if err := rows.Scan(&d.Name, &d.Start, &d.End); err != nil {
			return MealTimes{}, fmt.Errorf("scan meal time default: %w", err)
		}
		out.byName[strings.ToLower(d.Name)] = d
		out.List = append(out.List, d)
	}
	return out, rows.Err()
}

// MealTimeDefaults returns the times userID set per meal name.
func (r *WeeklyPlans) MealTimeDefaults(ctx context.Context, userID int64) (MealTimes, error) {
	return mealTimeDefaults(ctx, r.db, userID)
}

// SetMealTimeDefault sets (or changes) the time of a meal name for userID;
// ErrInvalidTime for a bad time, an empty or too long name.
func (r *WeeklyPlans) SetMealTimeDefault(ctx context.Context, userID int64, name, start, end string) error {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 200 {
		return ErrTooLong
	}
	if name == "" || ValidTimes(start, end) != nil {
		return ErrInvalidTime
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO meal_time_defaults (user_id, name, start_time, end_time) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, name) DO UPDATE SET name = excluded.name, start_time = excluded.start_time, end_time = excluded.end_time`,
		userID, name, start, nullIfEmpty(end)); err != nil {
		return fmt.Errorf("set meal time default: %w", err)
	}
	return nil
}

// DeleteMealTimeDefault removes the time of a meal name; meals already created
// keep theirs.
func (r *WeeklyPlans) DeleteMealTimeDefault(ctx context.Context, userID int64, name string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM meal_time_defaults WHERE user_id = ? AND name = ?`, userID, strings.TrimSpace(name)); err != nil {
		return fmt.Errorf("delete meal time default: %w", err)
	}
	return nil
}

// AddMealAt adds a meal at a time (start required, end optional). With an empty
// start it takes the user's time for that name, and with none it has no time, like
// the meals from before.
func (r *WeeklyPlans) AddMealAt(ctx context.Context, planID, userID int64, day int, name, start, end string) (int64, error) {
	if start == "" {
		defaults, err := mealTimeDefaults(ctx, r.db, userID)
		if err != nil {
			return 0, err
		}
		if d, ok := defaults.Lookup(name); ok {
			start, end = d.Start, d.End
		}
	}
	if start != "" {
		if err := ValidTimes(start, end); err != nil {
			return 0, err
		}
	}
	id, err := r.AddMeal(ctx, planID, userID, day, name)
	if err != nil {
		return 0, err
	}
	if start != "" {
		if _, err := r.db.ExecContext(ctx, `UPDATE meals SET start_time = ?, end_time = ? WHERE id = ?`, start, nullIfEmpty(end), id); err != nil {
			return 0, fmt.Errorf("set meal time: %w", err)
		}
	}
	return id, nil
}

// AddReminder adds a meal with no food (a supplement, a medicine) at a time.
func (r *WeeklyPlans) AddReminder(ctx context.Context, planID, userID int64, day int, name, note, start, end string) (int64, error) {
	if err := ValidTimes(start, end); err != nil {
		return 0, err
	}
	if utf8.RuneCountInString(note) > 200 {
		return 0, ErrTooLong
	}
	id, err := r.AddMeal(ctx, planID, userID, day, name)
	if err != nil {
		return 0, err
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE meals SET start_time = ?, end_time = ?, is_reminder = 1, note = ? WHERE id = ?`,
		start, nullIfEmpty(end), strings.TrimSpace(note), id); err != nil {
		return 0, fmt.Errorf("set reminder: %w", err)
	}
	return id, nil
}

// SetMealTime changes the time of one of userID's meals (the day re-sorts by it);
// ErrMealNotFound for a meal that is not theirs, ErrInvalidTime for a bad time.
func (r *WeeklyPlans) SetMealTime(ctx context.Context, mealID, userID int64, start, end string) (day int, err error) {
	if err := ValidTimes(start, end); err != nil {
		return 0, err
	}
	err = r.db.QueryRowContext(ctx, `
		UPDATE meals SET start_time = ?, end_time = ?
		WHERE id = ? AND weekly_plan_id IN (SELECT id FROM weekly_plans WHERE user_id = ?)
		RETURNING day_of_week`, start, nullIfEmpty(end), mealID, userID).Scan(&day)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrMealNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("set meal time: %w", err)
	}
	return day, nil
}
