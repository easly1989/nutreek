package db

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestShoppingGenerateAndAggregation(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)

	// Recipe: pasta with tomato, 4 servings, 200g pasta + 100g tomato for the whole recipe.
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 360, 12, 75, 1, &alice, nil, nil, nil, nil, nil, nil)
	pom, _ := ings.CreateLocal(ctx, "Pomodoro", nil, "g", 18, 1, 4, 0, &alice, nil, nil, nil, nil, nil, nil)
	rid, _ := recipes.Create(ctx, alice, "Pasta al pomodoro", nil, 4, false)
	if _, err := recipes.AddItem(ctx, rid, alice, pasta, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := recipes.AddItem(ctx, rid, alice, pom, 100); err != nil {
		t.Fatal(err)
	}

	// A direct ingredient in another meal: banana 120g.
	banana, _ := ings.CreateLocal(ctx, "Banana", nil, "g", 89, 1, 23, 0, &alice, nil, nil, nil, nil, nil, nil)

	// Meal 1 Monday: 1 serving of pasta with tomato (=> 50g pasta + 25g tomato).
	m1, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	if _, err := plans.AddItem(ctx, m1, alice, &rid, nil, 1, "g"); err != nil {
		t.Fatal(err)
	}
	// Meal 2 Tuesday: 2 servings of pasta with tomato (=> 100g pasta + 50g tomato).
	m2, _ := plans.AddMeal(ctx, plan.ID, alice, 1, "Pranzo")
	if _, err := plans.AddItem(ctx, m2, alice, &rid, nil, 2, "g"); err != nil {
		t.Fatal(err)
	}
	// Meal 3 Monday: 120g banana.
	m3, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Spuntino")
	if _, err := plans.AddItem(ctx, m3, alice, nil, &banana, 120, "g"); err != nil {
		t.Fatal(err)
	}
	// Meal 4 Wednesday WITH A CHEAT: regular pasta, but a hamburger cheat.
	m4, _ := plans.AddMeal(ctx, plan.ID, alice, 2, "Cena")
	if _, err := plans.AddItem(ctx, m4, alice, &rid, nil, 1, "g"); err != nil {
		t.Fatal(err)
	}
	hamb, _ := ings.CreateLocal(ctx, "Hamburger", nil, "g", 250, 14, 5, 18, &alice, nil, nil, nil, nil, nil, nil)
	if _, err := plans.AddCheat(ctx, m4, alice, nil, &hamb, 200); err != nil {
		t.Fatal(err)
	}

	// Create the list and generate it.
	list, err := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err != nil {
		t.Fatalf("GetOrCreateList: %v", err)
	}
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	full, err := shop.LoadFull(ctx, list.ID, alice)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}

	// Expected: pasta = 1+2 servings × 50g/serving = 150g (WITHOUT m4's
	// serving, because m4 has a cheat and is ignored).
	// tomato = (1+2) × 25g = 75g.
	// banana = 120g.
	// The hamburger must NOT be in the list (cheat).
	got := make(map[string]float64, len(full.Items))
	for _, it := range full.Items {
		got[it.Ingredient.Name] = it.TotalQuantity
	}
	if !approxEqual(got["Pasta"], 150) {
		t.Errorf("Pasta = %.2f, want 150", got["Pasta"])
	}
	if !approxEqual(got["Pomodoro"], 75) {
		t.Errorf("Pomodoro = %.2f, want 75", got["Pomodoro"])
	}
	if !approxEqual(got["Banana"], 120) {
		t.Errorf("Banana = %.2f, want 120", got["Banana"])
	}
	if _, ok := got["Hamburger"]; ok {
		t.Errorf("the hamburger should not be in the list (cheat): %v", got)
	}
}

func TestShoppingUserItemsPersistAcrossGenerate(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)

	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 360, 12, 75, 1, &alice, nil, nil, nil, nil, nil, nil)
	m1, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	if _, err := plans.AddItem(ctx, m1, alice, nil, &pasta, 100, "g"); err != nil {
		t.Fatal(err)
	}

	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	// Add a manual item.
	uid, err := shop.AddUserItem(ctx, list.ID, alice, "vino rosso", "1 bottiglia")
	if err != nil {
		t.Fatalf("AddUserItem: %v", err)
	}
	// Spunta entrambe.
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	if err := shop.ToggleItem(ctx, full.Items[0].ID, alice); err != nil {
		t.Fatal(err)
	}
	if err := shop.ToggleUserItem(ctx, uid, alice); err != nil {
		t.Fatal(err)
	}

	// Regenerate: the manual item persists, and the auto item stays checked
	// because its quantity did not change.
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	full, _ = shop.LoadFull(ctx, list.ID, alice)
	if len(full.UserItems) != 1 || full.UserItems[0].Description != "vino rosso" || !full.UserItems[0].Checked {
		t.Errorf("user item lost or checked lost: %+v", full.UserItems)
	}
	if len(full.Items) != 1 || !full.Items[0].Checked {
		t.Errorf("auto checked lost after regenerate (same qty): %+v", full.Items)
	}

	// Change the meal's quantity: regenerate, checked must reset (qty changed).
	if _, err := plans.AddItem(ctx, m1, alice, nil, &pasta, 50, "g"); err != nil {
		t.Fatal(err)
	}
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	full, _ = shop.LoadFull(ctx, list.ID, alice)
	if full.Items[0].Checked {
		t.Errorf("checked should reset after a quantity change: qty=%.2f", full.Items[0].TotalQuantity)
	}
}

func TestShoppingOwnership(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")

	listAlice, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if _, err := shop.LoadFull(ctx, listAlice.ID, bob); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob LoadFull: want ErrShoppingListNotFound, got %v", err)
	}
	if err := shop.Generate(ctx, listAlice.ID, bob); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob Generate: want ErrShoppingListNotFound, got %v", err)
	}
	if _, err := shop.AddUserItem(ctx, listAlice.ID, bob, "pane", ""); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob AddUserItem: want ErrShoppingListNotFound, got %v", err)
	}
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}
