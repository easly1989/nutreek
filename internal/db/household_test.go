package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// The shopping list sums what everyone eats and keeps who needs how much: the owner's
// quantity, each person's scaled one, a hand-set quantity, and a skipped meal.
func TestShoppingWithHousehold(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	plans := NewWeeklyPlans(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	h := NewHousehold(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	if _, err := store.DB.Exec(`UPDATE users SET target_kcal = 2400 WHERE id = ?`, alice); err != nil {
		t.Fatal(err)
	}
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 350, 12, 72, 1.5, &alice, nil, nil, nil, nil, nil, nil)
	oil, _ := ings.CreateLocal(ctx, "Oil", nil, "g", 900, 0, 0, 100, &alice, nil, nil, nil, nil, nil, nil)
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	mon, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Lunch")
	tue, _ := plans.AddMeal(ctx, plan.ID, alice, 1, "Lunch")
	_, _ = plans.AddItem(ctx, mon, alice, nil, &pasta, 100, "g")
	_, _ = plans.AddItem(ctx, tue, alice, nil, &pasta, 100, "g")
	oilRow, _ := plans.AddItem(ctx, mon, alice, nil, &oil, 10, "g")

	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if s, _ := shop.Shares(ctx, list.ID, alice); len(s) != 0 {
		t.Errorf("a household of one keeps no shares: %v", s)
	}

	giulia, _ := h.Add(ctx, alice, "Giulia", 1800) // 0.75
	leo, _ := h.Add(ctx, alice, "Leo", 1200)       // 0.5
	_ = h.SetSkip(ctx, alice, giulia, tue, true)   // Giulia is out on Tuesday
	_ = h.SetQuantity(ctx, alice, leo, oilRow, 0)  // Leo has no oil
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	got := map[int64]float64{}
	for _, it := range full.Items {
		got[it.IngredientID] = it.TotalQuantity
	}
	// Pasta: the owner 200, Giulia 75 on Monday only, Leo 50 + 50. Oil: the owner 10, Giulia 7.5 -> 10, Leo none.
	if got[pasta] != 375 || got[oil] != 20 {
		t.Errorf("totals = %v, want pasta 375 and oil 20", got)
	}
	shares, _ := shop.Shares(ctx, list.ID, alice)
	if s := shares[pasta]; s[0] != 200 || s[giulia] != 75 || s[leo] != 100 {
		t.Errorf("pasta shares = %v", s)
	}
	if _, ok := shares[oil][leo]; ok {
		t.Errorf("Leo needs no oil: %v", shares[oil])
	}
	if other, _ := shop.Shares(ctx, list.ID, alice+1); len(other) != 0 {
		t.Error("another account reads the shares")
	}
}

// The household tables (#238): people with a name and a goal (up to six), what they
// changed in a week, and everything scoped to the account that owns it.
func TestHousehold(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	plans := NewWeeklyPlans(store.DB)
	h := NewHousehold(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 350, 12, 72, 1.5, &alice, nil, nil, nil, nil, nil, nil)
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	lunch, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Lunch")
	row, _ := plans.AddItem(ctx, lunch, alice, nil, &pasta, 100, "g")
	bobPlan, _ := plans.GetOrCreateForWeek(ctx, bob, monday)
	bobMeal, _ := plans.AddMeal(ctx, bobPlan.ID, bob, 0, "Lunch")

	// People: validated, in the order added, scoped to the account.
	giulia, err := h.Add(ctx, alice, "  Giulia ", 1800)
	if err != nil {
		t.Fatal(err)
	}
	leo, _ := h.Add(ctx, alice, "Leo", 1400)
	for _, bad := range []struct {
		name string
		kcal float64
	}{{"", 1800}, {strings.Repeat("x", 61), 1800}, {"Z", 100}, {"Z", 9000}} {
		if _, err := h.Add(ctx, alice, bad.name, bad.kcal); !errors.Is(err, ErrInvalidPerson) {
			t.Errorf("Add(%q, %v) = %v, want ErrInvalidPerson", bad.name, bad.kcal, err)
		}
	}
	list, _ := h.List(ctx, alice)
	if len(list) != 2 || list[0].Name != "Giulia" || list[1].ID != leo || list[0].TargetKcal != 1800 {
		t.Fatalf("List = %+v", list)
	}
	if other, _ := h.List(ctx, bob); len(other) != 0 {
		t.Errorf("another account sees %d people", len(other))
	}
	if err := h.Update(ctx, bob, giulia, "Mine now", 2000); !errors.Is(err, ErrPersonNotFound) {
		t.Errorf("Update of someone else's person = %v", err)
	}
	if err := h.Update(ctx, alice, giulia, "Giulia R.", 1850); err != nil {
		t.Fatal(err)
	}
	for i := len(list); i < MaxPeople; i++ {
		if _, err := h.Add(ctx, alice, "P", 2000); err != nil {
			t.Fatalf("person %d: %v", i, err)
		}
	}
	if _, err := h.Add(ctx, alice, "One too many", 2000); !errors.Is(err, ErrInvalidPerson) {
		t.Errorf("a seventh person = %v, want ErrInvalidPerson", err)
	}

	// What a person changed: by hand per item, skipped per meal; scoped and cleared.
	if err := h.SetQuantity(ctx, alice, giulia, row, 90); err != nil {
		t.Fatal(err)
	}
	if err := h.SetQuantity(ctx, alice, giulia, row, 0); err != nil { // an update, 0 = not eaten
		t.Fatal(err)
	}
	if err := h.SetQuantity(ctx, alice, giulia, row, -1); !errors.Is(err, ErrInvalidPerson) {
		t.Errorf("a negative quantity = %v", err)
	}
	if err := h.SetSkip(ctx, alice, leo, lunch, true); err != nil {
		t.Fatal(err)
	}
	if err := h.SetSkip(ctx, alice, leo, lunch, true); err != nil { // twice is fine
		t.Fatal(err)
	}
	if err := h.SetQuantity(ctx, bob, giulia, row, 50); !errors.Is(err, ErrPersonNotFound) {
		t.Errorf("Bob changing Alice's person and item = %v", err)
	}
	if err := h.SetSkip(ctx, alice, giulia, bobMeal, true); !errors.Is(err, ErrPersonNotFound) {
		t.Errorf("skipping a meal of another account = %v", err)
	}
	w, err := h.Week(ctx, alice, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q, ok := w.Quantities[giulia][row]; !ok || q != 0 || !w.Skips[leo][lunch] {
		t.Errorf("Week = %+v", w)
	}
	if other, _ := h.Week(ctx, bob, plan.ID); len(other.Quantities)+len(other.Skips) != 0 {
		t.Error("another account reads the week's changes")
	}
	if err := h.SetSkip(ctx, alice, leo, lunch, false); err != nil {
		t.Fatal(err)
	}
	if err := h.ClearQuantity(ctx, alice, giulia, row); err != nil {
		t.Fatal(err)
	}
	if w, _ := h.Week(ctx, alice, plan.ID); len(w.Quantities)+len(w.Skips) != 0 {
		t.Errorf("after clearing: %+v", w)
	}

	// A quantity set by hand follows the row's unit and ingredient: the owner changing
	// the amount keeps it, changing the unit or replacing the ingredient drops it.
	rice, _ := ings.CreateLocal(ctx, "Rice", nil, "g", 350, 7, 78, 1, &alice, nil, nil, nil, nil, nil, nil)
	count := func() int {
		var n int
		_ = store.DB.QueryRow(`SELECT COUNT(*) FROM household_item_quantities WHERE meal_item_id = ?`, row).Scan(&n)
		return n
	}
	_ = h.SetQuantity(ctx, alice, leo, row, 40)
	if err := plans.UpdateItem(ctx, row, alice, 120, "g"); err != nil || count() != 1 {
		t.Errorf("a new amount in the same unit keeps the hand-set quantity: %v, %d", err, count())
	}
	if err := plans.ReplaceItemIngredient(ctx, row, alice, rice, 120, "g"); err != nil || count() != 0 {
		t.Errorf("replacing the ingredient drops it: %v, %d", err, count())
	}

	// Deleting an item, a meal or a person takes what hangs on it along.
	_ = h.SetQuantity(ctx, alice, giulia, row, 80)
	_ = h.SetSkip(ctx, alice, giulia, lunch, true)
	if err := h.Delete(ctx, alice, giulia); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = store.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM household_item_quantities) + (SELECT COUNT(*) FROM household_meal_skips)`).Scan(&n)
	if n != 0 {
		t.Errorf("%d rows left after deleting the person", n)
	}
	if err := h.Delete(ctx, alice, giulia); !errors.Is(err, ErrPersonNotFound) {
		t.Errorf("deleting twice = %v", err)
	}
}
