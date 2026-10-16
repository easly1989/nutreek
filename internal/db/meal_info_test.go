package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
