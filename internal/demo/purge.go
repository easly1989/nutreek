package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// PurgeHourUTC is when every demo group is deleted, every night.
const PurgeHourUTC = 3

// sweepEvery is how often groups past their idle time are looked for.
const sweepEvery = 5 * time.Minute

// seenEvery is how often a user's requests move their group's last_seen_at:
// one write a minute is plenty for an idle time of hours.
const seenEvery = time.Minute

// sqliteTime is how CURRENT_TIMESTAMP writes a DATETIME, so cutoffs written
// the same way compare as text.
const sqliteTime = "2006-01-02 15:04:05"

// Seen records a request of a signed-in user, moving their group's
// last_seen_at at most once every seenEvery. A user without a group is no
// error.
func (s *Service) Seen(ctx context.Context, userID int64) error {
	now := time.Now()
	s.mu.Lock()
	last := s.seen[userID]
	s.mu.Unlock()
	if now.Sub(last) < seenEvery {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE demo_groups SET last_seen_at = ?
		WHERE id = (SELECT group_id FROM demo_members WHERE user_id = ?)`,
		now.UTC().Format(sqliteTime), userID); err != nil {
		return err
	}
	s.mu.Lock()
	s.seen[userID] = now
	s.mu.Unlock()
	return nil
}

// Purge deletes the groups unused past the idle time, or every group and
// every demo user when all is set, with all their data. It returns the
// number of groups deleted.
func (s *Service) Purge(ctx context.Context, all bool) (int, error) {
	s.mu.Lock()
	for id, at := range s.seen {
		if time.Since(at) >= seenEvery {
			delete(s.seen, id)
		}
	}
	s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	groupsQ, usersQ := `SELECT id FROM demo_groups`, `SELECT id FROM users WHERE email LIKE ?`
	groupArgs, userArgs := []any{}, []any{"%" + emailDomain}
	if !all {
		// last_seen_at may trail the last request by seenEvery: wait that much
		// longer, so no group goes before its session has expired.
		cutoff := time.Now().UTC().Add(-s.cfg.Idle - seenEvery).Format(sqliteTime)
		groupsQ += ` WHERE last_seen_at < ?`
		usersQ = `SELECT m.user_id FROM demo_members m JOIN demo_groups g ON g.id = m.group_id WHERE g.last_seen_at < ?`
		groupArgs, userArgs = []any{cutoff}, []any{cutoff}
	}
	groups, err := ids(ctx, tx, groupsQ, groupArgs...)
	if err != nil {
		return 0, err
	}
	users, err := ids(ctx, tx, usersQ, userArgs...)
	if err != nil {
		return 0, err
	}
	if len(users) > 0 {
		in := placeholders(len(users))
		// ingredients.created_by does not cascade, and another group may use
		// what these users made: forget the author first, then delete what
		// nobody uses any more once the users are gone.
		made, err := ids(ctx, tx, `SELECT id FROM ingredients WHERE created_by IN (`+in+`)`, users...)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE ingredients SET created_by = NULL WHERE created_by IN (`+in+`)`, users...); err != nil {
			return 0, fmt.Errorf("demo purge: ingredients: %w", err)
		}
		// One statement, so rows that point at each other across the cascade
		// (a week and its template) go together.
		if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id IN (`+in+`)`, users...); err != nil {
			return 0, fmt.Errorf("demo purge: users: %w", err)
		}
		if err := deleteUnused(ctx, tx, made); err != nil {
			return 0, err
		}
	}
	if len(groups) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM demo_groups WHERE id IN (`+placeholders(len(groups))+`)`, groups...); err != nil {
			return 0, fmt.Errorf("demo purge: groups: %w", err)
		}
	}
	if all {
		// Every user is gone: no session is worth keeping.
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
			return 0, fmt.Errorf("demo purge: sessions: %w", err)
		}
	}
	return len(groups), tx.Commit()
}

// deleteUnused deletes the ingredients nothing refers to any more. The
// database says which: a delete a foreign key refuses is one still in use.
// A nutrition plan's reference is ON DELETE SET NULL and refuses nothing, so
// it is checked by hand. Passes repeat while they make progress, for ingredients that only the ones
// deleted in a pass referred to (superseded_by).
func deleteUnused(ctx context.Context, tx *sql.Tx, ingredients []any) error {
	for progress := true; progress; {
		progress = false
		kept := ingredients[:0]
		for _, id := range ingredients {
			res, err := tx.ExecContext(ctx, `DELETE FROM ingredients WHERE id = ?
				AND NOT EXISTS (SELECT 1 FROM nutritionist_meal_items WHERE ingredient_id = ?)`, id, id)
			var sqlErr *sqlite.Error
			switch {
			case err == nil:
				if n, _ := res.RowsAffected(); n == 0 {
					kept = append(kept, id)
					continue
				}
				progress = true
			case errors.As(err, &sqlErr) && sqlErr.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
				kept = append(kept, id)
			default:
				return fmt.Errorf("demo purge: ingredient %v: %w", id, err)
			}
		}
		ingredients = kept
	}
	return nil
}

func ids(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]any, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("demo purge: %w", err)
	}
	defer rows.Close()
	var out []any
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// placeholders returns n comma-separated "?".
func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

// NextNightly is the next nightly purge after now.
func NextNightly(now time.Time) time.Time {
	now = now.UTC()
	t := time.Date(now.Year(), now.Month(), now.Day(), PurgeHourUTC, 0, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// Run deletes idle groups every few minutes and every group each night at
// PurgeHourUTC, until ctx ends.
func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	purge := func(all bool) {
		n, err := s.Purge(ctx, all)
		switch {
		case err != nil && ctx.Err() == nil:
			logger.Error("demo: purge failed", "all", all, "err", err)
		case err == nil && (n > 0 || all):
			logger.Info("demo: groups deleted", "count", n, "all", all)
		}
	}
	purge(false)
	sweep := time.NewTicker(sweepEvery)
	defer sweep.Stop()
	night := time.NewTimer(time.Until(NextNightly(time.Now())))
	defer night.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			purge(false)
		case <-night.C:
			purge(true)
			night.Reset(time.Until(NextNightly(time.Now())))
		}
	}
}
