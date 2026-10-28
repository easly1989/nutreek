package db

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// Recipe rows keep the unit they were entered in (#206): a row in pieces
// counts quantity × unit weight in every total, rows from before stay grams.
func TestRecipeRowsInPieces(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	plans := NewWeeklyPlans(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	weight := 60.0
	egg, _ := ings.CreateLocal(ctx, "Egg", nil, "g", 143, 12.6, 0.7, 9.5, &alice, &weight, nil, nil, nil, nil, nil)
	oats, _ := ings.CreateLocal(ctx, "Oats", nil, "g", 380, 13, 60, 7, &alice, nil, nil, nil, nil, nil, nil)
	rec, _ := recipes.Create(ctx, alice, "Omelette", nil, 2, false)

	eggRow, err := recipes.AddItem(ctx, rec, alice, egg, 2, "pz") // 120 g
	if err != nil {
		t.Fatal(err)
	}
	// A row as written before the migration: no unit, so grams.
	if _, err := store.DB.ExecContext(ctx,
		`INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, sort_order) VALUES (?, ?, 50, 1)`, rec, oats); err != nil {
		t.Fatal(err)
	}
	// Pieces of an ingredient without a unit weight cannot be counted: grams.
	if _, err := recipes.AddItem(ctx, rec, alice, oats, 3, "pz"); err != nil {
		t.Fatal(err)
	}

	full, err := recipes.GetWithItems(ctx, rec, alice)
	if err != nil {
		t.Fatal(err)
	}
	units := map[string]string{}
	for _, it := range full.Items {
		units[it.Ingredient.Name+"/"+strconv.FormatFloat(it.Quantity, 'f', -1, 64)] = it.QuantityUnit
	}
	if units["Egg/2"] != "pz" || units["Oats/50"] != "g" || units["Oats/3"] != "g" {
		t.Fatalf("units = %v, want Egg/2 pz, Oats/50 g, Oats/3 g", units)
	}
	// 120 g egg = 171.6 kcal, 53 g oats = 201.4 kcal.
	const total = 171.6 + 201.4
	if got := macros.ForRecipeItems(full.Items).Kcal; !approxEqual(got, total) {
		t.Errorf("recipe total = %.2f, want %.2f", got, total)
	}
	per, err := recipes.PerServingByRecipe(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(per[rec].Kcal, total/2) {
		t.Errorf("per serving (SQL) = %.2f, want %.2f", per[rec].Kcal, total/2)
	}

	// The weekly plan and the shopping list: one serving of two.
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Breakfast")
	if _, err := plans.AddItem(ctx, meal, alice, &rec, nil, 1, ""); err != nil {
		t.Fatal(err)
	}
	week, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := macros.ForMealItem(week.Days[0][0].Items[0]).Kcal; !approxEqual(got, total/2) {
		t.Errorf("plan item = %.2f kcal, want %.2f", got, total/2)
	}
	list, _ := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	sl, _ := shop.LoadFull(ctx, list.ID, alice)
	got := map[string]float64{}
	for _, it := range sl.Items {
		got[it.Ingredient.Name] = it.TotalQuantity
	}
	if !approxEqual(got["Egg"], 60) || !approxEqual(got["Oats"], 26.5) {
		t.Errorf("shopping = %v, want Egg 60, Oats 26.5", got)
	}

	// The copy keeps the pieces, and is identical content to the source.
	dup, err := recipes.Duplicate(ctx, rec, alice, "Omelette 2")
	if err != nil {
		t.Fatal(err)
	}
	dupFull, _ := recipes.GetWithItems(ctx, dup, alice)
	if got := macros.ForRecipeItems(dupFull.Items).Kcal; !approxEqual(got, total) {
		t.Errorf("copy total = %.2f, want %.2f", got, total)
	}
	same := make([]domain.RecipeItem, 0, len(dupFull.Items))
	for _, it := range dupFull.Items {
		same = append(same, it.RecipeItem)
	}
	if other, _ := recipes.IdenticalContentRecipe(ctx, alice, same, dup); other != "Omelette" {
		t.Errorf("identical content = %q, want Omelette", other)
	}
	for i := range same {
		if same[i].QuantityUnit == "pz" {
			same[i].QuantityUnit = "g" // 2 g of egg is another recipe
		}
	}
	if other, _ := recipes.IdenticalContentRecipe(ctx, alice, same, dup); other != "" {
		t.Errorf("2 g matched 2 pz: %q", other)
	}

	// UpdateItem: owner and recipe checked, quantity and unit stored.
	if err := recipes.UpdateItem(ctx, rec, eggRow, bob, 1, "pz"); !errors.Is(err, ErrRecipeItemNotFound) {
		t.Errorf("Bob edits Alice's row: %v, want ErrRecipeItemNotFound", err)
	}
	if err := recipes.UpdateItem(ctx, dup, eggRow, alice, 1, "pz"); !errors.Is(err, ErrRecipeItemNotFound) {
		t.Errorf("row of another recipe: %v, want ErrRecipeItemNotFound", err)
	}
	if err := recipes.UpdateItem(ctx, rec, eggRow, alice, 0, "pz"); err == nil {
		t.Error("quantity 0 accepted")
	}
	if err := recipes.UpdateItem(ctx, rec, eggRow, alice, 180, "g"); err != nil {
		t.Fatal(err)
	}
	full, _ = recipes.GetWithItems(ctx, rec, alice)
	if it := full.Items[0]; it.Quantity != 180 || it.QuantityUnit != "g" {
		t.Errorf("after edit: %v %s, want 180 g", it.Quantity, it.QuantityUnit)
	}
}

// When an ingredient loses its unit weight, its rows in pieces become grams
// with the old weight: their totals do not change, nothing turns into 2 g.
func TestUnitWeightRemovedKeepsPieces(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	weight := 60.0
	egg, _ := ings.CreateLocal(ctx, "Egg", nil, "g", 143, 12.6, 0.7, 9.5, &alice, &weight, nil, nil, nil, nil, nil)
	rec, _ := recipes.Create(ctx, alice, "Eggs", nil, 1, false)
	if _, err := recipes.AddItem(ctx, rec, alice, egg, 2, "pz"); err != nil {
		t.Fatal(err)
	}
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Lunch")
	if _, err := plans.AddItem(ctx, meal, alice, nil, &egg, 3, "pz"); err != nil {
		t.Fatal(err)
	}

	// A new weight is just used: pieces stay pieces.
	weight = 50
	if err := ings.Update(ctx, egg, "Egg", nil, "g", 143, 12.6, 0.7, 9.5, &weight, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	full, _ := recipes.GetWithItems(ctx, rec, alice)
	if it := full.Items[0]; it.QuantityUnit != "pz" || macros.RecipeItemGrams(it) != 100 {
		t.Errorf("new weight: %v %s = %v g, want 2 pz = 100 g", it.Quantity, it.QuantityUnit, macros.RecipeItemGrams(it))
	}

	// No weight any more: grams, with the weight the rows were counted with.
	if err := ings.Update(ctx, egg, "Egg", nil, "g", 143, 12.6, 0.7, 9.5, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	full, _ = recipes.GetWithItems(ctx, rec, alice)
	if it := full.Items[0]; it.Quantity != 100 || it.QuantityUnit != "g" {
		t.Errorf("recipe row: %v %s, want 100 g", it.Quantity, it.QuantityUnit)
	}
	week, _ := plans.LoadFull(ctx, plan)
	if it := week.Days[0][0].Items[0]; it.Quantity != 150 || it.QuantityUnit != "g" {
		t.Errorf("plan item: %v %s, want 150 g", it.Quantity, it.QuantityUnit)
	}
}
