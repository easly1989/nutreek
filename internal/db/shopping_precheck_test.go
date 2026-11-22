package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The check before shopping sets rows aside, lowers their quantity or brings
// them back, in one go, for the user's own list only, and deletes nothing.
func TestApplyPrecheck(t *testing.T) {
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
	piece := 125.0
	yog, _ := ings.CreateLocal(ctx, "Yogurt", nil, "g", 57, 10, 3.8, 0.3, &alice, &piece, nil, nil, nil, nil, nil)
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 360, 12, 75, 1, &alice, nil, nil, nil, nil, nil, nil)
	riso, _ := ings.CreateLocal(ctx, "Riso", nil, "g", 350, 7, 78, 1, &alice, nil, nil, nil, nil, nil, nil)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &yog, 500, "g")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &pasta, 300, "g")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &riso, 200, "g")
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	_, _ = shop.AddManualItem(ctx, list.ID, alice, "Candele", 4, "pz")
	_, _ = shop.AddManualItem(ctx, list.ID, alice, "Pepe", 100, "g")
	if _, err := shop.AddUserItem(ctx, list.ID, alice, "Sale", "un sacchetto"); err != nil {
		t.Fatal(err)
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	id := map[string]int64{}
	for _, u := range full.UserItems {
		id[u.Description] = u.ID
	}
	autoRow := func(ing int64) (qty float64, ignored bool, found bool) {
		f, _ := shop.LoadFull(ctx, list.ID, alice)
		for _, it := range f.Items {
			if it.IngredientID == ing {
				return it.TotalQuantity, it.Ignored, true
			}
		}
		return 0, false, false
	}
	manualRow := func(desc string) (qty float64, ignored bool, found bool) {
		f, _ := shop.LoadFull(ctx, list.ID, alice)
		for _, u := range f.UserItems {
			if u.Description == desc {
				return u.Quantity, u.Ignored, true
			}
		}
		return 0, false, false
	}

	err := shop.ApplyPrecheck(ctx, list.ID, alice, []PrecheckChoice{
		{Key: RowKeyIngredient(pasta), State: PrecheckHave},
		{Key: RowKeyIngredient(yog), State: PrecheckSome, Have: 2, Unit: "pz"},   // 2 pieces of 125 g out of 500 g
		{Key: RowKeyIngredient(riso), State: PrecheckSome, Have: 250, Unit: "g"}, // more than needed: set aside
		{Key: RowKeyUserItem(id["Candele"]), State: PrecheckSome, Have: 1, Unit: "pz"},
		{Key: RowKeyUserItem(id["Pepe"]), State: PrecheckHave},
		{Key: RowKeyUserItem(id["Sale"]), State: PrecheckNeed},
		{Key: "i999999", State: PrecheckHave}, // a row that is gone is skipped
	})
	if err != nil {
		t.Fatal(err)
	}
	if q, ig, _ := autoRow(pasta); !ig || q != 300 {
		t.Errorf("pasta: ignored %v, qty %v, want set aside and untouched", ig, q)
	}
	if q, ig, _ := autoRow(yog); ig || q != 250 {
		t.Errorf("yogurt: ignored %v, qty %v, want 250 left", ig, q)
	}
	if _, ig, _ := autoRow(riso); !ig {
		t.Error("rice: having more than needed must set it aside")
	}
	if q, ig, _ := manualRow("Candele"); ig || q != 3 {
		t.Errorf("candles: ignored %v, qty %v, want 3 left", ig, q)
	}
	if _, ig, _ := manualRow("Pepe"); !ig {
		t.Error("pepper must be set aside")
	}
	if _, ig, found := manualRow("Sale"); !found || ig {
		t.Error("salt: «need it all» changes nothing")
	}

	// «Need it all» brings a row set aside back, with the quantity it has.
	if err := shop.ApplyPrecheck(ctx, list.ID, alice, []PrecheckChoice{{Key: RowKeyIngredient(pasta), State: PrecheckNeed}}); err != nil {
		t.Fatal(err)
	}
	if q, ig, _ := autoRow(pasta); ig || q != 300 {
		t.Errorf("pasta after «need»: ignored %v, qty %v", ig, q)
	}

	// An answer that cannot be applied changes nothing, even the ones before it.
	bad := []PrecheckChoice{
		{Key: RowKeyIngredient(pasta), State: PrecheckHave},
		{Key: RowKeyIngredient(yog), State: PrecheckSome, Have: 0, Unit: "g"},
	}
	if err := shop.ApplyPrecheck(ctx, list.ID, alice, bad); !errors.Is(err, ErrShoppingPrecheckInvalid) {
		t.Errorf("no quantity for «some»: %v", err)
	}
	if _, ig, _ := autoRow(pasta); ig {
		t.Error("a failed check must not apply the answers before the bad one")
	}
	for _, c := range []PrecheckChoice{
		{Key: RowKeyIngredient(pasta), State: PrecheckSome, Have: 1, Unit: "pz"},      // no piece weight
		{Key: RowKeyIngredient(pasta), State: PrecheckSome, Have: 1, Unit: "ml"},      // not its unit
		{Key: RowKeyUserItem(id["Sale"]), State: PrecheckSome, Have: 1, Unit: "g"},    // free-text quantity
		{Key: RowKeyUserItem(id["Candele"]), State: PrecheckSome, Have: 1, Unit: "g"}, // not its unit
		{Key: "x1", State: PrecheckHave},
		{Key: RowKeyIngredient(pasta), State: "maybe"},
	} {
		if err := shop.ApplyPrecheck(ctx, list.ID, alice, []PrecheckChoice{c}); !errors.Is(err, ErrShoppingPrecheckInvalid) {
			t.Errorf("%+v: %v, want invalid", c, err)
		}
	}
	if err := shop.ApplyPrecheck(ctx, list.ID, bob, []PrecheckChoice{{Key: RowKeyIngredient(pasta), State: PrecheckHave}}); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob checks Alice's list: %v", err)
	}
	if f, _ := shop.LoadFull(ctx, list.ID, alice); len(f.Items) != 3 || len(f.UserItems) != 3 {
		t.Errorf("rows were deleted: %d + %d", len(f.Items), len(f.UserItems))
	}
}
