package db

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
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
	if _, err := recipes.AddItem(ctx, rid, alice, pasta, 200, "g"); err != nil {
		t.Fatal(err)
	}
	if _, err := recipes.AddItem(ctx, rid, alice, pom, 100, "g"); err != nil {
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

func TestShoppingEditQuantityAndManualMerge(t *testing.T) {
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
	peso := 150.0
	mela, _ := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, &peso, nil, nil, nil, nil, nil)
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 360, 12, 75, 1, &alice, nil, nil, nil, nil, nil, nil)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	if _, err := plans.AddItem(ctx, meal, alice, nil, &mela, 100, "g"); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddItem(ctx, meal, alice, nil, &pasta, 100, "g"); err != nil {
		t.Fatal(err)
	}
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	qty := func(name string) float64 {
		full, _ := shop.LoadFull(ctx, list.ID, alice)
		for _, it := range full.Items {
			if it.Ingredient.Name == name {
				return it.TotalQuantity
			}
		}
		return -1
	}

	// Manual «mela» in pieces merges: 2 pz × 150 g onto the 100 g.
	if merged, err := shop.AddManualItem(ctx, list.ID, alice, "mela", 2, "pz"); err != nil || !merged {
		t.Fatalf("pz merge: merged=%v err=%v", merged, err)
	}
	if got := qty("Mela"); !approxEqual(got, 400) {
		t.Errorf("Mela = %v, want 400", got)
	}
	// Grams merge too; pieces of an ingredient without unit weight do not.
	if merged, _ := shop.AddManualItem(ctx, list.ID, alice, "Pasta", 50, "g"); !merged {
		t.Error("grams of a listed ingredient should merge")
	}
	if merged, _ := shop.AddManualItem(ctx, list.ID, alice, "Pasta", 1, "pz"); merged {
		t.Error("pieces without a unit weight must not merge")
	}
	// An unknown item stays a manual row, with its quantity text.
	if merged, _ := shop.AddManualItem(ctx, list.ID, alice, "Candele", 3, "pz"); merged {
		t.Error("unknown item must not merge")
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	if len(full.UserItems) != 2 || full.UserItems[1].QuantityText != "3 pz" {
		t.Errorf("manual items = %+v", full.UserItems)
	}

	// Edit the quantity of an auto item; ownership and bounds are enforced.
	var melaID int64
	for _, it := range full.Items {
		if it.Ingredient.Name == "Mela" {
			melaID = it.ID
		}
	}
	if err := shop.UpdateItemQuantity(ctx, melaID, alice, 300, "g"); err != nil || !approxEqual(qty("Mela"), 300) {
		t.Errorf("edit: err=%v qty=%v", err, qty("Mela"))
	}
	if err := shop.UpdateItemQuantity(ctx, melaID, bob, 10, "g"); !errors.Is(err, ErrShoppingItemNotFound) {
		t.Errorf("Bob edit: %v", err)
	}
	if err := shop.UpdateItemQuantity(ctx, melaID, alice, 0, "g"); !errors.Is(err, ErrShoppingQuantityInvalid) {
		t.Errorf("zero qty: %v", err)
	}

	// Pieces become grams with the weight per piece (3 × 150 g); an ingredient
	// without one refuses pieces.
	if err := shop.UpdateItemQuantity(ctx, melaID, alice, 3, "pz"); err != nil || !approxEqual(qty("Mela"), 450) {
		t.Errorf("pieces: err=%v qty=%v, want 450", err, qty("Mela"))
	}
	var pastaID int64
	for _, it := range full.Items {
		if it.Ingredient.Name == "Pasta" {
			pastaID = it.ID
		}
	}
	if err := shop.UpdateItemQuantity(ctx, pastaID, alice, 2, "pz"); !errors.Is(err, ErrShoppingQuantityInvalid) {
		t.Errorf("pieces without weight: %v", err)
	}

	// Edit a manual item: name, number and unit; then switch it to free text.
	candeleID := full.UserItems[1].ID
	if err := shop.UpdateUserItem(ctx, candeleID, alice, "Candele grandi", 5, "pz", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := shop.LoadFull(ctx, list.ID, alice)
	if u := got.UserItems[1]; u.Description != "Candele grandi" || u.QuantityText != "5 pz" || u.Quantity != 5 || u.Unit != "pz" {
		t.Errorf("manual after edit = %+v", u)
	}
	if err := shop.UpdateUserItem(ctx, candeleID, alice, "Candele grandi", 0, "g", ""); err != nil {
		t.Fatal(err)
	}
	if u := mustUserItem(t, shop, ctx, list.ID, alice, 1); u.QuantityText != "" || u.Unit != "" {
		t.Errorf("quantity removed = %+v", u)
	}
	if err := shop.UpdateUserItem(ctx, candeleID, alice, "Candele grandi", 0, "", "una confezione"); err != nil {
		t.Fatal(err)
	}
	if u := mustUserItem(t, shop, ctx, list.ID, alice, 1); u.QuantityText != "una confezione" || u.Unit != "" {
		t.Errorf("free text = %+v", u)
	}
	if err := shop.UpdateUserItem(ctx, candeleID, bob, "x", 1, "g", ""); !errors.Is(err, ErrShoppingItemNotFound) {
		t.Errorf("Bob edits Alice's item: %v", err)
	}
	if err := shop.UpdateUserItem(ctx, candeleID, alice, " ", 1, "g", ""); !errors.Is(err, ErrShoppingDescriptionRequired) {
		t.Errorf("empty name: %v", err)
	}
	if err := shop.UpdateUserItem(ctx, candeleID, alice, "x", 1, "kg", ""); !errors.Is(err, ErrShoppingQuantityInvalid) {
		t.Errorf("bad unit: %v", err)
	}
}

func mustUserItem(t *testing.T, shop *ShoppingLists, ctx context.Context, listID, userID int64, i int) domain.ShoppingUserItem {
	t.Helper()
	full, err := shop.LoadFull(ctx, listID, userID)
	if err != nil || len(full.UserItems) <= i {
		t.Fatalf("load: %v, %d items", err, len(full.UserItems))
	}
	return full.UserItems[i]
}

// The grouping is saved per user, starts as «none» and refuses unknown values.
func TestShoppingGroupingSetting(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()
	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")

	if g, err := users.ShoppingGrouping(ctx, alice); err != nil || g != ShoppingGroupNone {
		t.Fatalf("default = %q, %v, want none", g, err)
	}
	if err := users.SetShoppingGrouping(ctx, alice, ShoppingGroupCategory); err != nil {
		t.Fatal(err)
	}
	if g, _ := users.ShoppingGrouping(ctx, alice); g != ShoppingGroupCategory {
		t.Errorf("after set = %q", g)
	}
	if g, _ := users.ShoppingGrouping(ctx, bob); g != ShoppingGroupNone {
		t.Errorf("Bob = %q, want none: the setting is per user", g)
	}
	if err := users.SetShoppingGrouping(ctx, alice, ShoppingGroupCustom); err != nil {
		t.Errorf("custom: %v", err)
	}
	if err := users.SetShoppingGrouping(ctx, alice, "alphabetical"); !errors.Is(err, ErrShoppingGroupInvalid) {
		t.Errorf("unknown value: %v", err)
	}
}

// SaveOrder stores the dragged order, ignores keys that are not rows of the
// list, keeps the rows placed before after the ones it names, and belongs to
// the list's owner.
func TestShoppingSaveOrder(t *testing.T) {
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
	mela, _ := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, nil, nil, nil, nil, nil, nil)
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 360, 12, 75, 1, &alice, nil, nil, nil, nil, nil, nil)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &mela, 100, "g")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &pasta, 100, "g")
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := shop.AddManualItem(ctx, list.ID, alice, "Candele", 2, "pz"); err != nil {
		t.Fatal(err)
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	candele := RowKeyUserItem(full.UserItems[0].ID)
	kMela, kPasta := RowKeyIngredient(mela), RowKeyIngredient(pasta)

	// Pasta first, an unknown key, Candele; Mela is not named.
	if err := shop.SaveOrder(ctx, list.ID, alice, []string{kPasta, "i999999", "x1", candele}); err != nil {
		t.Fatal(err)
	}
	got, _ := shop.LoadFull(ctx, list.ID, alice)
	if len(got.Order) != 2 || got.Order[kPasta] != 0 || got.Order[candele] != 1 {
		t.Errorf("order = %v, want Pasta 0 and Candele 1; Mela, never placed, has no position", got.Order)
	}
	if _, ok := got.Order["i999999"]; ok {
		t.Error("an unknown key must be ignored")
	}

	// A second save names only Mela: it goes first, the others keep their order.
	if err := shop.SaveOrder(ctx, list.ID, alice, []string{kMela}); err != nil {
		t.Fatal(err)
	}
	got, _ = shop.LoadFull(ctx, list.ID, alice)
	if got.Order[kMela] != 0 || got.Order[kPasta] != 1 || got.Order[candele] != 2 {
		t.Errorf("after the second save = %v", got.Order)
	}

	// The order survives a regenerate: it is keyed by ingredient, not by row.
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if again, _ := shop.LoadFull(ctx, list.ID, alice); again.Order[kPasta] != 1 {
		t.Errorf("after regenerate = %v", again.Order)
	}
	if err := shop.SaveOrder(ctx, list.ID, bob, []string{kMela}); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob saves Alice's list: %v", err)
	}
}

// Ignoring flips for auto and manual items, survives a regenerate, is per
// owner; an auto item can be deleted and regenerating brings it back.
func TestShoppingIgnoreAndDelete(t *testing.T) {
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
	mela, _ := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, nil, nil, nil, nil, nil, nil)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	_, _ = plans.AddItem(ctx, meal, alice, nil, &mela, 100, "g")
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := shop.AddManualItem(ctx, list.ID, alice, "Candele", 2, "pz"); err != nil {
		t.Fatal(err)
	}
	full, _ := shop.LoadFull(ctx, list.ID, alice)
	autoID, userID := full.Items[0].ID, full.UserItems[0].ID

	if err := shop.ToggleItemIgnored(ctx, autoID, alice); err != nil {
		t.Fatal(err)
	}
	if err := shop.ToggleUserItemIgnored(ctx, userID, alice); err != nil {
		t.Fatal(err)
	}
	got, _ := shop.LoadFull(ctx, list.ID, alice)
	if !got.Items[0].Ignored || !got.UserItems[0].Ignored {
		t.Fatalf("after toggle: auto %v, manual %v, want both ignored", got.Items[0].Ignored, got.UserItems[0].Ignored)
	}
	if err := shop.ToggleItemIgnored(ctx, autoID, bob); !errors.Is(err, ErrShoppingItemNotFound) {
		t.Errorf("Bob ignores Alice's item: %v", err)
	}

	// A regenerate keeps the item ignored.
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if again, _ := shop.LoadFull(ctx, list.ID, alice); !again.Items[0].Ignored {
		t.Error("an ignored item must stay ignored after a regenerate")
	}
	// Toggling again restores it.
	again, _ := shop.LoadFull(ctx, list.ID, alice)
	if err := shop.ToggleItemIgnored(ctx, again.Items[0].ID, alice); err != nil {
		t.Fatal(err)
	}
	if got, _ := shop.LoadFull(ctx, list.ID, alice); got.Items[0].Ignored {
		t.Error("the second toggle must restore the item")
	}

	// Delete: owner only; a regenerate brings the item back.
	cur, _ := shop.LoadFull(ctx, list.ID, alice)
	if err := shop.RemoveItem(ctx, cur.Items[0].ID, bob); !errors.Is(err, ErrShoppingItemNotFound) {
		t.Errorf("Bob deletes Alice's item: %v", err)
	}
	if err := shop.RemoveItem(ctx, cur.Items[0].ID, alice); err != nil {
		t.Fatal(err)
	}
	if got, _ := shop.LoadFull(ctx, list.ID, alice); len(got.Items) != 0 {
		t.Errorf("after delete: %d items, want 0", len(got.Items))
	}
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	if got, _ := shop.LoadFull(ctx, list.ID, alice); len(got.Items) != 1 {
		t.Errorf("after regenerate: %d items, want it back", len(got.Items))
	}
}
