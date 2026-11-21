package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Merging two rows: ingredients of the meals (the plan is rewritten and the
// merge survives a regenerate), a manual item into an ingredient of the meals,
// two manual items; unlike units and a manual keep are refused; a pair can be
// kept apart.
func TestShoppingMergeRows(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	peso := 125.0
	yog, _ := ings.CreateLocal(ctx, "Yogurt greco 0%", nil, "g", 57, 10, 3.8, 0.3, &alice, &peso, nil, nil, nil, nil, nil)
	alto, _ := ings.CreateLocal(ctx, "Altoproteico yogurt greco", nil, "g", 60, 10.4, 3.6, 0.2, &alice, nil, nil, nil, nil, nil, nil)
	latte, _ := ings.CreateLocal(ctx, "Latte", nil, "ml", 46, 3.3, 4.8, 1.5, &alice, nil, nil, nil, nil, nil, nil)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Colazione")
	meal2, _ := plans.AddMeal(ctx, plan.ID, alice, 1, "Colazione")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &yog, 2, "pz") // 250 g
	_, _ = plans.AddItem(ctx, meal2, alice, nil, &alto, 150, "g")
	_, _ = plans.AddItem(ctx, meal2, alice, nil, &latte, 200, "ml")
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	qty := func(id int64) float64 {
		full, _ := shop.LoadFull(ctx, list.ID, alice)
		for _, it := range full.Items {
			if it.IngredientID == id {
				return it.TotalQuantity
			}
		}
		return -1
	}
	if qty(yog) != 250 || qty(alto) != 150 {
		t.Fatalf("start: yogurt %v, alto %v", qty(yog), qty(alto))
	}

	// Units that do not add up are refused, and so are rows that are not in the list.
	if err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), RowKeyIngredient(latte)); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("g with ml: %v", err)
	}
	if err := shop.MergeRows(ctx, list.ID, bob, RowKeyIngredient(yog), RowKeyIngredient(alto)); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob merges Alice's list: %v", err)
	}
	if err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), "u999"); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("missing manual row: %v", err)
	}

	// Two ingredients of the meals: Alto goes into Yogurt, in the plan and in the list.
	if err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), RowKeyIngredient(alto)); err != nil {
		t.Fatal(err)
	}
	if qty(yog) != 400 || qty(alto) != -1 {
		t.Errorf("after merge: yogurt %v, alto %v, want 400 and gone", qty(yog), qty(alto))
	}
	var onAlto int
	_ = store.DB.QueryRow(`SELECT COUNT(*) FROM meal_items WHERE ingredient_id = ?`, alto).Scan(&onAlto)
	if onAlto != 0 {
		t.Errorf("%d plan rows still use the dropped ingredient", onAlto)
	}
	// It stays merged: Alto added to the plan again counts as Yogurt.
	_, _ = plans.AddItem(ctx, meal2, alice, nil, &alto, 100, "g")
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if qty(yog) != 500 || qty(alto) != -1 {
		t.Errorf("after regenerate: yogurt %v, alto %v, want 500 and gone", qty(yog), qty(alto))
	}

	// A manual item (in pieces) into the ingredient of the meals: 2 pz x 125 g.
	if _, err := shop.AddManualItem(ctx, list.ID, alice, "Yogurt greco bianco", 2, "pz"); err != nil {
		t.Fatal(err)
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	m1 := RowKeyUserItem(full.UserItems[0].ID)
	if err := shop.MergeRows(ctx, list.ID, alice, m1, RowKeyIngredient(yog)); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("a manual row cannot absorb a row of the meals: %v", err)
	}
	if err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), m1); err != nil {
		t.Fatal(err)
	}
	if qty(yog) != 750 {
		t.Errorf("after the manual merge: %v, want 750", qty(yog))
	}
	if got, _ := shop.LoadFull(ctx, list.ID, alice); len(got.UserItems) != 0 {
		t.Errorf("the manual item must be gone: %+v", got.UserItems)
	}

	// Two manual items.
	_, _ = shop.AddManualItem(ctx, list.ID, alice, "Candele", 2, "pz")
	_, _ = shop.AddManualItem(ctx, list.ID, alice, "Candele profumate", 3, "pz")
	full, _ = shop.LoadFull(ctx, list.ID, alice)
	if err := shop.MergeRows(ctx, list.ID, alice, RowKeyUserItem(full.UserItems[0].ID), RowKeyUserItem(full.UserItems[1].ID)); err != nil {
		t.Fatal(err)
	}
	if got, _ := shop.LoadFull(ctx, list.ID, alice); len(got.UserItems) != 1 || got.UserItems[0].Quantity != 5 || got.UserItems[0].QuantityText != "5 pz" {
		t.Errorf("two manual items: %+v", got.UserItems)
	}

	// Keeping a pair apart is remembered, whatever the order.
	if err := shop.DismissSimilar(ctx, list.ID, alice, "u9", "i2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := shop.LoadFull(ctx, list.ID, alice); !got.Dismissed[PairKey("i2", "u9")] {
		t.Errorf("dismissed = %v", got.Dismissed)
	}
	if err := shop.DismissSimilar(ctx, list.ID, alice, "x1", "i2"); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("bad key: %v", err)
	}
}
