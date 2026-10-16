package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheats(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	mealID, err := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	if err != nil {
		t.Fatal(err)
	}

	// Add an official item (not a cheat), then 2 cheats.
	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	if _, err := plans.AddItem(ctx, mealID, alice, nil, &pane, 50, "g"); err != nil {
		t.Fatal(err)
	}
	hamb, _ := ings.CreateLocal(ctx, "Hamburger", nil, "g", 250, 14, 5, 18, &alice, nil, nil, nil, nil, nil, nil)
	pat, _ := ings.CreateLocal(ctx, "Patatine", nil, "g", 312, 4, 41, 15, &alice, nil, nil, nil, nil, nil, nil)

	c1, err := plans.AddCheat(ctx, mealID, alice, nil, &hamb, 200)
	if err != nil {
		t.Fatalf("AddCheat hamb: %v", err)
	}
	c2, err := plans.AddCheat(ctx, mealID, alice, nil, &pat, 150)
	if err != nil {
		t.Fatalf("AddCheat pat: %v", err)
	}

	// Bob cannot add cheats to Alice's meal.
	if _, err := plans.AddCheat(ctx, mealID, bob, nil, &pat, 100); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("Bob AddCheat: want ErrPlanNotFound, got %v", err)
	}

	// LoadFull includes the cheats.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	if len(full.Days[0]) != 1 {
		t.Fatalf("want 1 meal, got %d", len(full.Days[0]))
	}
	mw := full.Days[0][0]
	if len(mw.Items) != 1 {
		t.Errorf("items = %d, want 1 (bread)", len(mw.Items))
	}
	if len(mw.Cheats) != 2 {
		t.Errorf("cheats = %d, want 2", len(mw.Cheats))
	}
	if !mw.HasCheats() {
		t.Error("HasCheats() should be true")
	}
	// Check that the ingredients are resolved.
	if mw.Cheats[0].Ingredient == nil || mw.Cheats[0].Ingredient.Name != "Hamburger" {
		t.Errorf("first cheat: ingredient not resolved: %+v", mw.Cheats[0])
	}

	// Reorder cheats: swap c1 and c2.
	if err := plans.ReorderCheats(ctx, mealID, alice, []int64{c2, c1}); err != nil {
		t.Fatalf("ReorderCheats: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	got := full.Days[0][0].Cheats
	if got[0].ID != c2 || got[1].ID != c1 {
		t.Errorf("reorder order = [%d %d], want [%d %d]", got[0].ID, got[1].ID, c2, c1)
	}

	// Remove a cheat (Bob cannot).
	if err := plans.RemoveCheat(ctx, c1, bob); !errors.Is(err, ErrCheatNotFound) {
		t.Errorf("Bob remove: want ErrCheatNotFound, got %v", err)
	}
	if err := plans.RemoveCheat(ctx, c1, alice); err != nil {
		t.Fatalf("RemoveCheat: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	if len(full.Days[0][0].Cheats) != 1 {
		t.Errorf("cheats after remove = %d, want 1", len(full.Days[0][0].Cheats))
	}

	// ClearCheats removes the rest.
	if err := plans.ClearCheats(ctx, mealID, alice); err != nil {
		t.Fatalf("ClearCheats: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	if full.Days[0][0].HasCheats() {
		t.Error("after ClearCheats the meal should have no cheats")
	}
}
