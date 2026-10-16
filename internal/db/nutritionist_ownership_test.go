package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Variants and items are created only inside your own plan, and another
// user's private recipe cannot be added (it would reveal its name and
// ingredients) (#129).
func TestNutritionistOwnership(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	np := NewNutritionistPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	alicePlan, _ := np.GetOrCreateActivePlan(ctx, alice)
	bobPlan, _ := np.GetOrCreateActivePlan(ctx, bob)
	aliceMT, _ := np.CreateMealType(ctx, alicePlan.ID, "Pranzo")
	bobMT, _ := np.CreateMealType(ctx, bobPlan.ID, "Pranzo")

	// Variant: only in a meal type of your own plan.
	aliceVar, err := np.CreateVariation(ctx, alicePlan.ID, aliceMT, nil)
	if err != nil {
		t.Fatalf("CreateVariation in your own plan: %v", err)
	}
	if _, err := np.CreateVariation(ctx, alicePlan.ID, bobMT, nil); !errors.Is(err, ErrNutritionistItemNotFound) {
		t.Errorf("variant in Bob's plan: err = %v, want ErrNutritionistItemNotFound", err)
	}
	bobVar, _ := np.CreateVariation(ctx, bobPlan.ID, bobMT, nil)

	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)

	// Item: only in a variant of your own plan.
	if _, err := np.AddItem(ctx, alicePlan.ID, aliceVar, nil, &pane, 80, "g"); err != nil {
		t.Fatalf("AddItem in your own variant: %v", err)
	}
	if _, err := np.AddItem(ctx, alicePlan.ID, bobVar, nil, &pane, 80, "g"); !errors.Is(err, ErrNutritionistItemNotFound) {
		t.Errorf("item in Bob's variant: err = %v, want ErrNutritionistItemNotFound", err)
	}

	// Recipes: your own and public ones yes, other users' private ones no.
	mine, _ := recipes.Create(ctx, alice, "Mia", nil, 1, false)
	public, _ := recipes.Create(ctx, bob, "Pubblica di Bob", nil, 1, true)
	private, _ := recipes.Create(ctx, bob, "Privata di Bob", nil, 1, false)
	for _, id := range []int64{mine, public} {
		if _, err := np.AddItem(ctx, alicePlan.ID, aliceVar, &id, nil, 1, "g"); err != nil {
			t.Errorf("visible recipe %d: %v", id, err)
		}
	}
	if _, err := np.AddItem(ctx, alicePlan.ID, aliceVar, &private, nil, 1, "g"); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("Bob's private recipe: err = %v, want ErrRecipeNotFound", err)
	}
}

// The same holds for the week's meals and cheats.
func TestMealItemRecipeVisibility(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	recipes := NewRecipes(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")

	public, _ := recipes.Create(ctx, bob, "Pubblica di Bob", nil, 1, true)
	private, _ := recipes.Create(ctx, bob, "Privata di Bob", nil, 1, false)

	if _, err := plans.AddItem(ctx, meal, alice, &public, nil, 1, "g"); err != nil {
		t.Errorf("item with a public recipe: %v", err)
	}
	if _, err := plans.AddItem(ctx, meal, alice, &private, nil, 1, "g"); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("item with Bob's private recipe: err = %v, want ErrRecipeNotFound", err)
	}
	if _, err := plans.AddCheat(ctx, meal, alice, &private, nil, 1); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("cheat with Bob's private recipe: err = %v, want ErrRecipeNotFound", err)
	}
}
