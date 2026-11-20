package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The empty-week cards say how many meals last week has, without creating
// that week's plan.
func TestMealCountForWeek(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")

	if n, err := plans.MealCountForWeek(ctx, alice, monday); err != nil || n != 0 {
		t.Fatalf("no plan: n = %d, err = %v, want 0", n, err)
	}
	var count int
	_ = store.DB.QueryRow(`SELECT COUNT(*) FROM weekly_plans`).Scan(&count)
	if count != 0 {
		t.Errorf("MealCountForWeek created %d plans, want none", count)
	}

	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	for _, name := range []string{"Colazione", "Pranzo"} {
		if _, err := plans.AddMeal(ctx, plan.ID, alice, 0, name); err != nil {
			t.Fatal(err)
		}
	}
	// Any day of the week finds the same plan.
	if n, _ := plans.MealCountForWeek(ctx, alice, monday.AddDate(0, 0, 3)); n != 2 {
		t.Errorf("alice: n = %d, want 2", n)
	}
	if n, _ := plans.MealCountForWeek(ctx, bob, monday); n != 0 {
		t.Errorf("bob: n = %d, want 0", n)
	}
}

// The picker dialog shows «Aggiungi a Pranzo · Giovedì 15 ottobre»:
// MealInfo reads the meal's name and date, for the owner only (#122).
func TestMealInfo(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	meal, err := plans.AddMeal(ctx, plan.ID, alice, 3, "Pranzo")
	if err != nil {
		t.Fatal(err)
	}

	name, date, err := plans.MealInfo(ctx, meal, alice)
	if err != nil {
		t.Fatalf("MealInfo: %v", err)
	}
	if name != "Pranzo" || date.Format("2006-01-02") != "2026-10-15" {
		t.Errorf("MealInfo = %q %s, want Pranzo 2026-10-15", name, date.Format("2006-01-02"))
	}
	if _, _, err := plans.MealInfo(ctx, meal, bob); !errors.Is(err, ErrMealNotFound) {
		t.Errorf("another user's meal: err = %v, want ErrMealNotFound", err)
	}
	if _, _, err := plans.MealInfo(ctx, 9999, alice); !errors.Is(err, ErrMealNotFound) {
		t.Errorf("missing meal: err = %v, want ErrMealNotFound", err)
	}
}
