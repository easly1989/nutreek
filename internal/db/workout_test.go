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
	logs.CreateWorkoutLog(ctx, alice, gym.ID, d1, 60, nil)
	logs.CreateWorkoutLog(ctx, alice, run.ID, d1, 30, nil)
	logs.CreateWorkoutLog(ctx, alice, gym.ID, d2, 60, nil) // same pair again, newer
	logs.CreateWorkoutLog(ctx, bob, run.ID, d2, 99, nil)
	rec, err := logs.RecentWorkouts(ctx, alice, 3)
	if err != nil || len(rec) != 2 || rec[0].ActivityID != gym.ID || rec[0].Minutes != 60 || rec[1].ActivityID != run.ID {
		t.Fatalf("RecentWorkouts = %+v, %v", rec, err)
	}

	// Edit: the activity and the minutes change and the kcal are worked out again;
	// somebody else's workout is not found.
	id, _ := logs.CreateWorkoutLog(ctx, alice, gym.ID, d2, 60, nil)
	if err := logs.UpdateWorkoutLog(ctx, id, alice, run.ID, 120); err != nil {
		t.Fatal(err)
	}
	got, err := logs.GetWorkoutLog(ctx, id, alice)
	if err != nil || got.ActivityID != run.ID || got.DurationMinutes != 120 || got.KcalBurned != run.KcalPerHour*2 || !got.LoggedAt.Equal(d2) {
		t.Fatalf("after the edit: %+v, %v", got, err)
	}
	if err := logs.UpdateWorkoutLog(ctx, id, bob, gym.ID, 10); !errors.Is(err, ErrWorkoutLogNotFound) {
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
