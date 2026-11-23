package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Workouts of the day (#251): the recent ones, editing, and «As yesterday».
func TestWorkoutShortcutsAndEdit(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	logs := NewWorkoutLogs(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	acts, err := logs.ListActivities(ctx)
	if err != nil || len(acts) < 2 {
		t.Fatalf("activities: %v", err)
	}
	gym, run := acts[0], acts[1]
	d1 := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	d2 := d1.AddDate(0, 0, 1)

	// Recent: distinct pairs, newest first, only the user's own.
	logs.CreateWorkoutLog(ctx, alice, gym.ID, d1, 60, "", nil)
	logs.CreateWorkoutLog(ctx, alice, run.ID, d1, 30, "", nil)
	logs.CreateWorkoutLog(ctx, alice, gym.ID, d2, 60, "", nil) // same pair again, newer
	logs.CreateWorkoutLog(ctx, bob, run.ID, d2, 99, "", nil)
	rec, err := logs.RecentWorkouts(ctx, alice, 3)
	if err != nil || len(rec) != 2 || rec[0].ActivityID != gym.ID || rec[0].Minutes != 60 || rec[1].ActivityID != run.ID {
		t.Fatalf("RecentWorkouts = %+v, %v", rec, err)
	}

	// Edit: the activity and the minutes change and the kcal are worked out again;
	// somebody else's workout is not found.
	id, _ := logs.CreateWorkoutLog(ctx, alice, gym.ID, d2, 60, "", nil)
	if err := logs.UpdateWorkoutLog(ctx, id, alice, run.ID, 120, ""); err != nil {
		t.Fatal(err)
	}
	got, err := logs.GetWorkoutLog(ctx, id, alice)
	if err != nil || got.ActivityID != run.ID || got.DurationMinutes != 120 || got.KcalBurned != run.KcalPerHour*2 || !got.LoggedAt.Equal(d2) {
		t.Fatalf("after the edit: %+v, %v", got, err)
	}
	if err := logs.UpdateWorkoutLog(ctx, id, bob, gym.ID, 10, ""); !errors.Is(err, ErrWorkoutLogNotFound) {
		t.Errorf("editing another user's workout = %v, want ErrWorkoutLogNotFound", err)
	}
	if _, err := logs.GetWorkoutLog(ctx, id, bob); !errors.Is(err, ErrWorkoutLogNotFound) {
		t.Errorf("reading another user's workout = %v, want ErrWorkoutLogNotFound", err)
	}

	// As yesterday: the day before's workouts are repeated, with their kcal, and
	// nothing else moves.
	d3 := d2.AddDate(0, 0, 1)
	n, err := logs.CopyWorkoutLogs(ctx, alice, d2, d3)
	if err != nil || n != 2 {
		t.Fatalf("CopyWorkoutLogs = %d, %v, want 2", n, err)
	}
	if kcal, _ := logs.WorkoutKcalForDay(ctx, alice, d3); kcal != run.KcalPerHour*2+gym.KcalPerHour {
		t.Errorf("kcal of the copied day = %v", kcal)
	}
	if n, _ := logs.CopyWorkoutLogs(ctx, alice, d3.AddDate(0, 0, 5), d3); n != 0 {
		t.Errorf("copying an empty day copied %d", n)
	}
}

// Workouts, part 2 (#251): activities of one's own, a time of day, kcal from the
// weight in the profile, and the goal setting.
func TestWorkoutCustomActivitiesTimeAndWeight(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	logs := NewWorkoutLogs(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

	// An activity of one's own: only its owner sees it, two people can both have
	// «Padel», one person cannot have it twice, and nobody can log somebody else's.
	builtin, _ := logs.ListActivitiesFor(ctx, alice)
	padel, err := logs.CreateActivity(ctx, alice, "Padel", 360)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logs.CreateActivity(ctx, alice, "Padel", 100); !errors.Is(err, ErrActivityExists) {
		t.Errorf("same name twice = %v, want ErrActivityExists", err)
	}
	if _, err := logs.CreateActivity(ctx, bob, "Padel", 100); err != nil {
		t.Errorf("the same name for another user = %v", err)
	}
	mine, _ := logs.ListActivitiesFor(ctx, alice)
	if len(mine) != len(builtin)+1 || !mine[len(mine)-1].Custom || mine[len(mine)-1].ID != padel {
		t.Errorf("alice's list = %d activities, last %+v", len(mine), mine[len(mine)-1])
	}
	if all, _ := logs.ListActivities(ctx); len(all) != len(builtin) {
		t.Errorf("the built-in list grew to %d", len(all))
	}
	if _, err := logs.CreateWorkoutLog(ctx, bob, padel, day, 60, "", nil); !errors.Is(err, ErrActivityNotFound) {
		t.Errorf("logging another user's activity = %v, want ErrActivityNotFound", err)
	}

	// The kcal follow the weight in the profile (the list is made for 70 kg); a
	// user with no weight gets the list's figure.
	id, err := logs.CreateWorkoutLog(ctx, alice, padel, day, 60, "18:30", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := logs.GetWorkoutLog(ctx, id, alice)
	if got.KcalBurned != 360 || got.AtTime != "18:30" {
		t.Errorf("no weight: %v kcal at %q, want 360 at 18:30", got.KcalBurned, got.AtTime)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE users SET weight_kg = 91 WHERE id = ?`, alice); err != nil {
		t.Fatal(err)
	}
	if err := logs.UpdateWorkoutLog(ctx, id, alice, padel, 60, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = logs.GetWorkoutLog(ctx, id, alice)
	if got.KcalBurned != 468 || got.AtTime != "" {
		t.Errorf("91 kg: %v kcal at %q, want 468 and no time", got.KcalBurned, got.AtTime)
	}

	// The goal setting is off for a new user and is saved.
	if u, _ := users.GetByID(ctx, alice); u.WorkoutsInGoal {
		t.Error("a new user has workouts in the goal")
	}
	if err := users.SetWorkoutsInGoal(ctx, alice, true); err != nil {
		t.Fatal(err)
	}
	if u, _ := users.GetByID(ctx, alice); !u.WorkoutsInGoal {
		t.Error("the setting was not saved")
	}
}
