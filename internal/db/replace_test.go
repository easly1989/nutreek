package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Replacing the ingredient of a row (#227) keeps the row where it is, checks the
// owner, refuses a recipe row, and stores the quantity in a unit the new
// ingredient can count.
func TestReplaceIngredient(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	plans := NewWeeklyPlans(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	weight := 125.0
	greek, _ := ings.CreateLocal(ctx, "Greek yogurt", nil, "g", 58, 10, 4, 0.3, &alice, nil, nil, nil, nil, nil, nil)
	skyr, _ := ings.CreateLocal(ctx, "Skyr", nil, "g", 62, 11, 3.5, 0.2, &alice, &weight, nil, nil, nil, nil, nil)
	apple, _ := ings.CreateLocal(ctx, "Apple", nil, "g", 52, 0.3, 14, 0.2, &alice, nil, nil, nil, nil, nil, nil)

	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Breakfast")
	first, _ := plans.AddItem(ctx, meal, alice, nil, &apple, 100, "g")
	row, err := plans.AddItem(ctx, meal, alice, nil, &greek, 150, "g")
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := recipes.Create(ctx, alice, "Bowl", nil, 1, false)
	recipeRow := mustAddRecipeItem(t, recipes, rec, alice, greek, 150)
	ownRecipe, _ := recipes.Create(ctx, alice, "Plain", nil, 1, false)
	if _, err := plans.AddItem(ctx, meal, alice, &ownRecipe, nil, 1, ""); err != nil {
		t.Fatal(err)
	}

	// The row, with its meal and day.
	it, err := plans.IngredientItem(ctx, row, alice)
	if err != nil || it.MealName != "Breakfast" || it.DayOfWeek != 0 || *it.IngredientID != greek {
		t.Fatalf("IngredientItem = %+v, %v", it, err)
	}
	if _, err := plans.IngredientItem(ctx, row, bob); !errors.Is(err, ErrMealItemNotFound) {
		t.Errorf("another user's row: %v, want ErrMealItemNotFound", err)
	}

	// Another user cannot replace it; a recipe row is not an ingredient row.
	if err := plans.ReplaceItemIngredient(ctx, row, bob, skyr, 150, "g"); !errors.Is(err, ErrMealItemNotFound) {
		t.Errorf("replace by another user: %v, want ErrMealItemNotFound", err)
	}
	var recipeItem int64
	if err := store.DB.QueryRowContext(ctx, `SELECT id FROM meal_items WHERE recipe_id = ?`, ownRecipe).Scan(&recipeItem); err != nil {
		t.Fatal(err)
	}
	if err := plans.ReplaceItemIngredient(ctx, recipeItem, alice, skyr, 150, "g"); !errors.Is(err, ErrMealItemNotFound) {
		t.Errorf("replace on a recipe row: %v, want ErrMealItemNotFound", err)
	}
	if err := plans.ReplaceItemIngredient(ctx, row, alice, skyr, 0, "g"); err == nil {
		t.Error("a zero quantity must be refused")
	}

	// Same row id, new ingredient, quantity kept; «pz» holds because Skyr has a
	// unit weight, and falls back to grams for an ingredient that has none.
	if err := plans.ReplaceItemIngredient(ctx, row, alice, skyr, 2, "pz"); err != nil {
		t.Fatal(err)
	}
	var ingID int64
	var qty float64
	var unit string
	var order int
	read := func() {
		if err := store.DB.QueryRowContext(ctx, `SELECT ingredient_id, quantity, quantity_unit, sort_order FROM meal_items WHERE id = ?`, row).Scan(&ingID, &qty, &unit, &order); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if ingID != skyr || qty != 2 || unit != "pz" || order != 1 {
		t.Errorf("row = ingredient %d, %v %s, order %d; want Skyr, 2 pz, order 1", ingID, qty, unit, order)
	}
	if err := plans.ReplaceItemIngredient(ctx, row, alice, apple, 250, "pz"); err != nil {
		t.Fatal(err)
	}
	read()
	if ingID != apple || unit != "g" {
		t.Errorf("row = ingredient %d in %s, want Apple in g", ingID, unit)
	}
	if err := plans.ReplaceItemIngredient(ctx, row, alice, 999999, 150, "g"); !errors.Is(err, ErrIngredientNotFound) {
		t.Errorf("an unknown ingredient: %v, want ErrIngredientNotFound", err)
	}
	if untouched, _ := plans.IngredientItem(ctx, first, alice); untouched == nil || *untouched.IngredientID != apple || untouched.Quantity != 100 {
		t.Error("the other rows of the meal must not change")
	}

	// A recipe row: owner and recipe checked, order kept.
	if err := recipes.ReplaceItem(ctx, rec, recipeRow, bob, skyr, 150, "g"); err == nil {
		t.Error("another user must not replace a recipe row")
	}
	if err := recipes.ReplaceItem(ctx, ownRecipe, recipeRow, alice, skyr, 150, "g"); !errors.Is(err, ErrRecipeItemNotFound) {
		t.Errorf("a row of another recipe: %v, want ErrRecipeItemNotFound", err)
	}
	if err := recipes.ReplaceItem(ctx, rec, recipeRow, alice, skyr, 150, "g"); err != nil {
		t.Fatal(err)
	}
	full, _ := recipes.GetWithItems(ctx, rec, alice)
	if len(full.Items) != 1 || full.Items[0].ID != recipeRow || full.Items[0].IngredientID != skyr || full.Items[0].Quantity != 150 {
		t.Errorf("recipe items = %+v, want the same row with Skyr, 150", full.Items)
	}

	// Same category: only what this user put in the category of the ingredient.
	cat, _ := cats.CreateCategory(ctx, alice, "Dairy")
	for _, id := range []int64{greek, skyr} {
		if err := cats.SetIngredientCategory(ctx, alice, id, &cat); err != nil {
			t.Fatal(err)
		}
	}
	same, err := cats.SameCategoryIDs(ctx, alice, greek)
	if err != nil || len(same) != 2 || !same[greek] || !same[skyr] || same[apple] {
		t.Errorf("same category = %v, %v; want Greek yogurt and Skyr", same, err)
	}
	if none, _ := cats.SameCategoryIDs(ctx, bob, greek); len(none) != 0 {
		t.Errorf("another user's categories counted: %v", none)
	}
	if none, _ := cats.SameCategoryIDs(ctx, alice, apple); len(none) != 0 {
		t.Errorf("an ingredient with no category has no neighbours: %v", none)
	}

	// An ingredient the reconciler replaced while a dialog was open is refused.
	if err := ings.MarkSuperseded(ctx, apple, greek); err != nil {
		t.Fatal(err)
	}
	if err := recipes.ReplaceItem(ctx, rec, recipeRow, alice, apple, 150, "g"); !errors.Is(err, ErrIngredientNotFound) {
		t.Errorf("a superseded ingredient: %v, want ErrIngredientNotFound", err)
	}
}

func mustAddRecipeItem(t *testing.T, recipes *Recipes, recipeID, userID, ingredientID int64, qty float64) int64 {
	t.Helper()
	id, err := recipes.AddItem(context.Background(), recipeID, userID, ingredientID, qty, "g")
	if err != nil {
		t.Fatal(err)
	}
	return id
}
