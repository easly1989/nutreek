package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Notifications (#237): push subscriptions, reminders, quiet hours, the log of
// what was sent. The sending itself is in internal/notify.

// Reminder kinds. Water, gym and meals are built in (one of each per user, made
// on first use and off until the user turns them on); custom ones are many.
const (
	KindWater  = "water"
	KindGym    = "gym"
	KindMeals  = "meals"
	KindCustom = "custom"
)

// Reminder is one reminder of a user.
type Reminder struct {
	ID       int64
	UserID   int64
	Kind     string
	Title    string
	Enabled  bool
	Start    string // HH:MM: the first water slot, the gym or custom time
	End      string // HH:MM: the last water slot
	EveryMin int    // minutes between water slots
	Days     int    // bit 0 = Monday … bit 6 = Sunday
	LeadMin  int    // minutes before the time (gym, meals)
	SkipDone bool   // water: skip when the goal is reached or water was logged lately
}

// OnDay says whether the reminder is on for day (0 = Monday … 6 = Sunday).
func (r Reminder) OnDay(day int) bool { return r.Days&(1<<day) != 0 }

// PushSub is a device that allows notifications.
type PushSub struct {
	ID       int64
	UserID   int64
	Endpoint string
	P256dh   string
	Auth     string
}

// NotifySettings are a user's quiet hours and time zone.
type NotifySettings struct {
	QuietOn   bool
	QuietFrom string
	QuietTo   string
	TimeZone  string // an IANA name from the browser; "" until a device is added
}

// ErrInvalidReminder is returned for a reminder that cannot be saved.
var ErrInvalidReminder = errors.New("invalid reminder")

// ErrReminderNotFound is returned for a reminder that is not the user's.
var ErrReminderNotFound = errors.New("reminder not found")

// Notifications is the repository of the notification tables.
type Notifications struct {
	db *sql.DB
}

// NewNotifications creates a Notifications repository.
func NewNotifications(db *sql.DB) *Notifications { return &Notifications{db: db} }

var builtinDefaults = []Reminder{
	{Kind: KindWater, Start: "08:00", End: "20:00", EveryMin: 120, Days: 127, SkipDone: true},
	{Kind: KindGym, Start: "18:30", Days: 1<<0 | 1<<2 | 1<<4, LeadMin: 30},
	{Kind: KindMeals, LeadMin: 10, Days: 127},
}

// Reminders returns the user's reminders, built-in ones first, making the built-in
// ones when they are missing (all off).
func (n *Notifications) Reminders(ctx context.Context, userID int64) ([]Reminder, error) {
	for _, d := range builtinDefaults {
		if _, err := n.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO reminders (user_id, kind, enabled, start_time, end_time, every_min, days, lead_min, skip_done)
			VALUES (?, ?, 0, ?, ?, ?, ?, ?, ?)`,
			userID, d.Kind, d.Start, nonEmpty(d.End, "20:00"), nonZero(d.EveryMin, 120), d.Days, d.LeadMin, boolInt(d.SkipDone)); err != nil {
			return nil, fmt.Errorf("make built-in reminder: %w", err)
		}
	}
	return n.list(ctx, `WHERE user_id = ?`, userID)
}

// Enabled returns the reminders that are on, of every user that has a device.
func (n *Notifications) Enabled(ctx context.Context) ([]Reminder, error) {
	return n.list(ctx, `WHERE enabled = 1 AND user_id IN (SELECT user_id FROM push_subscriptions)`)
}

func (n *Notifications) list(ctx context.Context, where string, args ...any) ([]Reminder, error) {
	rows, err := n.db.QueryContext(ctx, `
		SELECT id, user_id, kind, title, enabled, start_time, end_time, every_min, days, lead_min, skip_done
		FROM reminders `+where+`
		ORDER BY CASE kind WHEN 'water' THEN 0 WHEN 'gym' THEN 1 WHEN 'meals' THEN 2 ELSE 3 END, start_time, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()
	var out []Reminder
	for rows.Next() {
		var r Reminder
		var en, skip int
		if err := rows.Scan(&r.ID, &r.UserID, &r.Kind, &r.Title, &en, &r.Start, &r.End, &r.EveryMin, &r.Days, &r.LeadMin, &skip); err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		r.Enabled, r.SkipDone = en == 1, skip == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func nonZero(n, def int) int {
	if n == 0 {
		return def
	}
	return n
}

// validReminder checks what a save may carry: times as HH:MM, a lead and a step in
// range, at least one day, and a title for a custom reminder.
func validReminder(r Reminder) bool {
	if ValidTimes(r.Start, "") != nil || r.Days <= 0 || r.Days > 127 || r.LeadMin < 0 || r.LeadMin > 240 || utf8.RuneCountInString(r.Title) > 200 {
		return false
	}
	switch r.Kind {
	case KindWater:
		return ValidTimes(r.Start, r.End) == nil && r.EveryMin >= 30 && r.EveryMin <= 720
	case KindCustom:
		return strings.TrimSpace(r.Title) != ""
	}
	return true
}

// SaveReminder saves a reminder of userID: a built-in one by its kind, a custom one
// by its id (a new one when ID is 0). ErrInvalidReminder for values out of range,
// ErrReminderNotFound for a custom id that is not theirs.
func (n *Notifications) SaveReminder(ctx context.Context, userID int64, r Reminder) error {
	r.Title = strings.TrimSpace(r.Title)
	if !validReminder(r) {
		return ErrInvalidReminder
	}
	if r.Kind == KindCustom {
		if r.ID == 0 {
			_, err := n.db.ExecContext(ctx, `
				INSERT INTO reminders (user_id, kind, title, enabled, start_time, days) VALUES (?, 'custom', ?, ?, ?, ?)`,
				userID, r.Title, boolInt(r.Enabled), r.Start, r.Days)
			return err
		}
		res, err := n.db.ExecContext(ctx, `
			UPDATE reminders SET title = ?, enabled = ?, start_time = ?, days = ? WHERE id = ? AND user_id = ? AND kind = 'custom'`,
			r.Title, boolInt(r.Enabled), r.Start, r.Days, r.ID, userID)
		if err != nil {
			return err
		}
		if c, _ := res.RowsAffected(); c == 0 {
			return ErrReminderNotFound
		}
		return nil
	}
	if _, err := n.Reminders(ctx, userID); err != nil { // makes the built-in rows
		return err
	}
	_, err := n.db.ExecContext(ctx, `
		UPDATE reminders SET enabled = ?, start_time = ?, end_time = ?, every_min = ?, days = ?, lead_min = ?, skip_done = ?
		WHERE user_id = ? AND kind = ?`,
		boolInt(r.Enabled), r.Start, nonEmpty(r.End, "20:00"), nonZero(r.EveryMin, 120), r.Days, r.LeadMin, boolInt(r.SkipDone), userID, r.Kind)
	return err
}

// DeleteReminder removes a custom reminder of userID.
func (n *Notifications) DeleteReminder(ctx context.Context, userID, id int64) error {
	res, err := n.db.ExecContext(ctx, `DELETE FROM reminders WHERE id = ? AND user_id = ? AND kind = 'custom'`, id, userID)
	if err != nil {
		return err
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return ErrReminderNotFound
	}
	return nil
}

// Settings returns the user's quiet hours and time zone (the defaults when never saved).
func (n *Notifications) Settings(ctx context.Context, userID int64) (NotifySettings, error) {
	s := NotifySettings{QuietOn: true, QuietFrom: "22:00", QuietTo: "07:00"}
	var on int
	err := n.db.QueryRowContext(ctx,
		`SELECT quiet_on, quiet_from, quiet_to, time_zone FROM notification_settings WHERE user_id = ?`, userID,
	).Scan(&on, &s.QuietFrom, &s.QuietTo, &s.TimeZone)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("notification settings: %w", err)
	}
	s.QuietOn = on == 1
	return s, nil
}

// SaveQuiet saves the quiet-hours band (two HH:MM times, from may be later than to).
func (n *Notifications) SaveQuiet(ctx context.Context, userID int64, on bool, from, to string) error {
	if ValidTimes(from, "") != nil || ValidTimes(to, "") != nil {
		return ErrInvalidReminder
	}
	_, err := n.db.ExecContext(ctx, `
		INSERT INTO notification_settings (user_id, quiet_on, quiet_from, quiet_to) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET quiet_on = excluded.quiet_on, quiet_from = excluded.quiet_from, quiet_to = excluded.quiet_to`,
		userID, boolInt(on), from, to)
	return err
}

// SetTimeZone keeps the browser's time zone (an IANA name); a name Go does not
// know is refused.
func (n *Notifications) SetTimeZone(ctx context.Context, userID int64, tz string) error {
	if _, err := time.LoadLocation(tz); err != nil || tz == "" || len(tz) > 64 {
		return ErrInvalidReminder
	}
	_, err := n.db.ExecContext(ctx, `
		INSERT INTO notification_settings (user_id, time_zone) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET time_zone = excluded.time_zone`, userID, tz)
	return err
}

// MealMutes returns the meal names the «Meals» reminder stays quiet for.
func (n *Notifications) MealMutes(ctx context.Context, userID int64) (map[string]bool, error) {
	rows, err := n.db.QueryContext(ctx, `SELECT name FROM notification_meal_mutes WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("meal mutes: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[strings.ToLower(name)] = true
	}
	return out, rows.Err()
}

// SetMealMutes replaces the muted meal names.
func (n *Notifications) SetMealMutes(ctx context.Context, userID int64, names []string) error {
	tx, err := n.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM notification_meal_mutes WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" && utf8.RuneCountInString(name) <= 200 {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO notification_meal_mutes (user_id, name) VALUES (?, ?)`, userID, name); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// AddSub keeps a device (by its endpoint: a device that comes back replaces its
// keys, and moves to the new user when someone else signs in on it).
func (n *Notifications) AddSub(ctx context.Context, userID int64, endpoint, p256dh, auth, userAgent string) error {
	if endpoint == "" || p256dh == "" || auth == "" || len(endpoint) > 2000 || !strings.HasPrefix(endpoint, "https://") {
		return ErrInvalidReminder
	}
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	_, err := n.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, user_agent) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth, user_agent = excluded.user_agent`,
		userID, endpoint, p256dh, auth, userAgent)
	return err
}

// Subs returns the devices of a user.
func (n *Notifications) Subs(ctx context.Context, userID int64) ([]PushSub, error) {
	rows, err := n.db.QueryContext(ctx,
		`SELECT id, user_id, endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()
	var out []PushSub
	for rows.Next() {
		var s PushSub
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256dh, &s.Auth); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteSub removes a device of userID by its endpoint (also used for a device the
// push service says is gone, with the user it belongs to).
func (n *Notifications) DeleteSub(ctx context.Context, userID int64, endpoint string) error {
	_, err := n.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint = ?`, userID, endpoint)
	return err
}

// MarkSent records that key was sent to userID; first is false when it already was,
// so a reminder goes out once. It also forgets what is older than two days.
func (n *Notifications) MarkSent(ctx context.Context, userID int64, key string) (first bool, err error) {
	res, err := n.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_log (user_id, key) VALUES (?, ?)`, userID, key)
	if err != nil {
		return false, err
	}
	c, _ := res.RowsAffected()
	return c == 1, nil
}

// ForgetOldSent drops the log entries older than two days.
func (n *Notifications) ForgetOldSent(ctx context.Context) error {
	_, err := n.db.ExecContext(ctx, `DELETE FROM notification_log WHERE sent_at < datetime('now', '-2 days')`)
	return err
}
