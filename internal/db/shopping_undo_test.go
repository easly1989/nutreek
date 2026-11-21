package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A merge can be undone: the plan rows, the list quantities and the items that
// went are put back, in the three kinds of merge, and only by the list's owner.
func TestShoppingUndoMerge(t *testing.T) {
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
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Colazione")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &yog, 2, "pz")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &alto, 150, "g")
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

	// Two ingredients of the plan: merge, then undo.
	undo, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), RowKeyIngredient(alto))
	if err != nil || undo == nil {
		t.Fatalf("merge: %v", err)
	}
	if qty(alto) != -1 {
		t.Fatal("the merge did not merge")
	}
	if err := shop.UndoMerge(ctx, list.ID, bob, undo); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob undoes Alice's merge: %v", err)
	}
	if err := shop.UndoMerge(ctx, list.ID+99, alice, undo); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("another list: %v", err)
	}
	if err := shop.UndoMerge(ctx, list.ID, alice, undo); err != nil {
		t.Fatal(err)
	}
	if qty(yog) != 250 || qty(alto) != 150 {
		t.Errorf("after undo: yogurt %v, alto %v, want 250 and 150", qty(yog), qty(alto))
	}
	var onAlto int
	_ = store.DB.QueryRow(`SELECT COUNT(*) FROM meal_items WHERE ingredient_id = ?`, alto).Scan(&onAlto)
	if onAlto != 1 {
		t.Errorf("%d plan rows use Alto after the undo, want 1", onAlto)
	}
	var merges int
	_ = store.DB.QueryRow(`SELECT COUNT(*) FROM shopping_list_merges`).Scan(&merges)
	if merges != 0 {
		t.Errorf("%d merge records after the undo, want none", merges)
	}

	// A manual item into the ingredient of the meals.
	if _, err := shop.AddManualItem(ctx, list.ID, alice, "Yogurt greco bianco", 200, "g"); err != nil {
		t.Fatal(err)
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	manual := full.UserItems[0]
	undo, err = shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), RowKeyUserItem(manual.ID))
	if err != nil {
		t.Fatal(err)
	}
	if qty(yog) != 450 {
		t.Fatalf("after the manual merge: %v", qty(yog))
	}
	if err := shop.UndoMerge(ctx, list.ID, alice, undo); err != nil {
		t.Fatal(err)
	}
	got, _ := shop.LoadFull(ctx, list.ID, alice)
	if qty(yog) != 250 || len(got.UserItems) != 1 || got.UserItems[0].Quantity != 200 || got.UserItems[0].Description != manual.Description {
		t.Errorf("after undo: yogurt %v, manual %+v", qty(yog), got.UserItems)
	}

	// Two manual items.
	_, _ = shop.AddManualItem(ctx, list.ID, alice, "Sale grosso", 2, "pz")
	_, _ = shop.AddManualItem(ctx, list.ID, alice, "Sale grosso marino", 3, "pz")
	full, _ = shop.LoadFull(ctx, list.ID, alice)
	var a1, b1 int64
	for _, u := range full.UserItems {
		switch u.Description {
		case "Sale grosso":
			a1 = u.ID
		case "Sale grosso marino":
			b1 = u.ID
		}
	}
	undo, err = shop.MergeRows(ctx, list.ID, alice, RowKeyUserItem(a1), RowKeyUserItem(b1))
	if err != nil {
		t.Fatal(err)
	}
	if err := shop.UndoMerge(ctx, list.ID, alice, undo); err != nil {
		t.Fatal(err)
	}
	got, _ = shop.LoadFull(ctx, list.ID, alice)
	by := map[int64]float64{}
	for _, u := range got.UserItems {
		by[u.ID] = u.Quantity
	}
	if by[a1] != 2 || by[b1] != 3 {
		t.Errorf("after undoing two manual items: %v, want 2 and 3", by)
	}
}

// The undo survives what happens in between: a manual item added after the
// merge must not take the place of the one that went, and a regenerate (which
// gives the rows from the meals new ids) must not send the quantity to another row.
func TestShoppingUndoMergeSurvivesChanges(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	yog, _ := ings.CreateLocal(ctx, "Yogurt greco 0%", nil, "g", 57, 10, 3.8, 0.3, &alice, nil, nil, nil, nil, nil, nil)
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 360, 12, 75, 1, &alice, nil, nil, nil, nil, nil, nil)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &yog, 250, "g")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &pasta, 100, "g")
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Sale grosso", "Sale grosso marino"} {
		if _, err := shop.AddManualItem(ctx, list.ID, alice, n, 2, "pz"); err != nil {
			t.Fatal(err)
		}
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	var a1, b1 int64
	for _, u := range full.UserItems {
		if u.Description == "Sale grosso" {
			a1 = u.ID
		} else {
			b1 = u.ID
		}
	}

	// Two manual items merged, a new one added (it takes the freed id), undone.
	undo, err := shop.MergeRows(ctx, list.ID, alice, RowKeyUserItem(a1), RowKeyUserItem(b1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shop.AddManualItem(ctx, list.ID, alice, "Pepe", 1, "pz"); err != nil {
		t.Fatal(err)
	}
	if err := shop.UndoMerge(ctx, list.ID, alice, undo); err != nil {
		t.Fatal(err)
	}
	got, _ := shop.LoadFull(ctx, list.ID, alice)
	names := map[string]float64{}
	for _, u := range got.UserItems {
		names[u.Description] = u.Quantity
	}
	if len(got.UserItems) != 3 || names["Sale grosso"] != 2 || names["Sale grosso marino"] != 2 || names["Pepe"] != 1 {
		t.Errorf("after the undo: %v, want the two salts and Pepe", names)
	}

	// A manual item merged into the yogurt, the list regenerated, then undone:
	// the yogurt goes back to 250 g and Pasta is untouched.
	if _, err := shop.AddManualItem(ctx, list.ID, alice, "Yogurt greco bianco", 100, "g"); err != nil {
		t.Fatal(err)
	}
	full, _ = shop.LoadFull(ctx, list.ID, alice)
	var yogManual int64
	for _, u := range full.UserItems {
		if u.Description == "Yogurt greco bianco" {
			yogManual = u.ID
		}
	}
	undo, err = shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), RowKeyUserItem(yogManual))
	if err != nil {
		t.Fatal(err)
	}
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if err := shop.UndoMerge(ctx, list.ID, alice, undo); err != nil {
		t.Fatal(err)
	}
	got, _ = shop.LoadFull(ctx, list.ID, alice)
	for _, it := range got.Items {
		switch it.IngredientID {
		case yog:
			if it.TotalQuantity != 250 {
				t.Errorf("yogurt = %v, want 250", it.TotalQuantity)
			}
		case pasta:
			if it.TotalQuantity != 100 {
				t.Errorf("pasta = %v, want 100 (untouched)", it.TotalQuantity)
			}
		}
	}
}
