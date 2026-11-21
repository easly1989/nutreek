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
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), RowKeyIngredient(latte)); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("g with ml: %v", err)
	}
	if _, err := shop.MergeRows(ctx, list.ID, bob, RowKeyIngredient(yog), RowKeyIngredient(alto)); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob merges Alice's list: %v", err)
	}
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), "u999"); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("missing manual row: %v", err)
	}

	// Two ingredients of the meals: Alto goes into Yogurt, in the plan and in the list.
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), RowKeyIngredient(alto)); err != nil {
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
	if _, err := shop.MergeRows(ctx, list.ID, alice, m1, RowKeyIngredient(yog)); !errors.Is(err, ErrShoppingMergeInvalid) {
		t.Errorf("a manual row cannot absorb a row of the meals: %v", err)
	}
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(yog), m1); err != nil {
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
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyUserItem(full.UserItems[0].ID), RowKeyUserItem(full.UserItems[1].ID)); err != nil {
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

// A row key has one spelling: «i05» is not a second name for «i5», so a row
// cannot be merged into itself by writing its id another way.
func TestParseRowKeyIsCanonical(t *testing.T) {
	for _, k := range []string{"i05", "i+5", "u03", "i0", "i-1", "i", "x5", "i5x", ""} {
		if _, _, ok := ParseRowKey(k); ok {
			t.Errorf("ParseRowKey(%q) accepted", k)
		}
	}
	if kind, id, ok := ParseRowKey("u42"); !ok || kind != 'u' || id != 42 {
		t.Errorf("ParseRowKey(u42) = %c %d %v", kind, id, ok)
	}
}

// Merges follow a chain to its end, leave the other users' plans and the meals
// with a cheat alone, and convert pieces by weight.
func TestShoppingMergeChainPlanScopeAndPieces(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	pa, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	pb, _ := plans.GetOrCreateForWeek(ctx, bob, monday)
	w100, w50 := 100.0, 50.0
	a, _ := ings.CreateLocal(ctx, "Biscotti A", nil, "g", 400, 6, 70, 10, &alice, &w100, nil, nil, nil, nil, nil)
	b, _ := ings.CreateLocal(ctx, "Biscotti B", nil, "g", 400, 6, 70, 10, &alice, &w50, nil, nil, nil, nil, nil)
	c, _ := ings.CreateLocal(ctx, "Biscotti C", nil, "g", 400, 6, 70, 10, &alice, nil, nil, nil, nil, nil, nil)
	meal, _ := plans.AddMeal(ctx, pa.ID, alice, 0, "Colazione")
	cheatMeal, _ := plans.AddMeal(ctx, pa.ID, alice, 1, "Cena")
	bobMeal, _ := plans.AddMeal(ctx, pb.ID, bob, 0, "Colazione")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &a, 2, "pz") // 200 g of A
	_, _ = plans.AddItem(ctx, meal, alice, nil, &b, 3, "pz") // 150 g of B
	_, _ = plans.AddItem(ctx, cheatMeal, alice, nil, &b, 40, "g")
	_, _ = plans.AddItem(ctx, bobMeal, bob, nil, &b, 70, "g")
	if _, err := store.DB.Exec(`INSERT INTO meal_cheat_items (meal_id, ingredient_id, quantity) VALUES (?, ?, 10)`, cheatMeal, a); err != nil {
		t.Fatal(err)
	}
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}

	// B into A: the 3 pieces of 50 g become pieces of A (100 g): 1.5 pz.
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(a), RowKeyIngredient(b)); err != nil {
		t.Fatal(err)
	}
	var qty float64
	var unit string
	if err := store.DB.QueryRow(`SELECT quantity, quantity_unit FROM meal_items WHERE meal_id = ? AND ingredient_id = ? ORDER BY quantity LIMIT 1`, meal, a).Scan(&qty, &unit); err != nil {
		t.Fatal(err)
	}
	var onB int
	_ = store.DB.QueryRow(`SELECT COUNT(*) FROM meal_items WHERE meal_id = ? AND ingredient_id = ?`, meal, b).Scan(&onB)
	if onB != 0 {
		t.Errorf("%d rows of B left in the merged week", onB)
	}
	var got150 float64
	_ = store.DB.QueryRow(`SELECT quantity FROM meal_items WHERE meal_id = ? AND ingredient_id = ? AND quantity <> 2`, meal, a).Scan(&got150)
	if got150 != 1.5 {
		t.Errorf("3 pieces of 50 g became %v pieces of 100 g, want 1.5", got150)
	}
	// A meal with a cheat and another user's plan are not touched.
	var cheatB, bobB int
	_ = store.DB.QueryRow(`SELECT COUNT(*) FROM meal_items WHERE meal_id = ? AND ingredient_id = ?`, cheatMeal, b).Scan(&cheatB)
	_ = store.DB.QueryRow(`SELECT COUNT(*) FROM meal_items WHERE meal_id = ? AND ingredient_id = ?`, bobMeal, b).Scan(&bobB)
	if cheatB != 1 || bobB != 1 {
		t.Errorf("rows of B in the cheat meal %d and in Bob's plan %d, want 1 and 1", cheatB, bobB)
	}

	// A chain: A into C; B (still in the cheat meal, planned again) follows to C.
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(c), RowKeyIngredient(a)); err == nil {
		t.Fatal("C is not in the list, the merge must be refused")
	}
	_, _ = plans.AddItem(ctx, meal, alice, nil, &c, 100, "g")
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := shop.MergeRows(ctx, list.ID, alice, RowKeyIngredient(c), RowKeyIngredient(a)); err != nil {
		t.Fatal(err)
	}
	_, _ = plans.AddItem(ctx, meal, alice, nil, &b, 10, "g") // B again, after B -> A -> C
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	if len(full.Items) != 1 || full.Items[0].IngredientID != c {
		t.Fatalf("after the chain: %d rows, want only C", len(full.Items))
	}
	_ = qty
	_ = unit
}
