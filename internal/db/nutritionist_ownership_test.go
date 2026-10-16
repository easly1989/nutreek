package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Varianti e voci si creano solo dentro il proprio piano, e una ricetta
// privata di un altro utente non si puo' aggiungere (ne mostrerebbe nome e
// ingredienti) (#129).
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

	// Variante: solo nel tipo pasto del proprio piano.
	aliceVar, err := np.CreateVariation(ctx, alicePlan.ID, aliceMT, nil)
	if err != nil {
		t.Fatalf("CreateVariation nel proprio piano: %v", err)
	}
	if _, err := np.CreateVariation(ctx, alicePlan.ID, bobMT, nil); !errors.Is(err, ErrNutritionistItemNotFound) {
		t.Errorf("variante nel piano di Bob: err = %v, atteso ErrNutritionistItemNotFound", err)
	}
	bobVar, _ := np.CreateVariation(ctx, bobPlan.ID, bobMT, nil)

	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)

	// Voce: solo nella variante del proprio piano.
	if _, err := np.AddItem(ctx, alicePlan.ID, aliceVar, nil, &pane, 80, "g"); err != nil {
		t.Fatalf("AddItem nella propria variante: %v", err)
	}
	if _, err := np.AddItem(ctx, alicePlan.ID, bobVar, nil, &pane, 80, "g"); !errors.Is(err, ErrNutritionistItemNotFound) {
		t.Errorf("voce nella variante di Bob: err = %v, atteso ErrNutritionistItemNotFound", err)
	}

	// Ricette: le proprie e le pubbliche si', le private altrui no.
	mine, _ := recipes.Create(ctx, alice, "Mia", nil, 1, false)
	public, _ := recipes.Create(ctx, bob, "Pubblica di Bob", nil, 1, true)
	private, _ := recipes.Create(ctx, bob, "Privata di Bob", nil, 1, false)
	for _, id := range []int64{mine, public} {
		if _, err := np.AddItem(ctx, alicePlan.ID, aliceVar, &id, nil, 1, "g"); err != nil {
			t.Errorf("ricetta visibile %d: %v", id, err)
		}
	}
	if _, err := np.AddItem(ctx, alicePlan.ID, aliceVar, &private, nil, 1, "g"); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("ricetta privata di Bob: err = %v, atteso ErrRecipeNotFound", err)
	}
}

// Lo stesso vale per pasti e sgarri della settimana.
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
		t.Errorf("voce con ricetta pubblica: %v", err)
	}
	if _, err := plans.AddItem(ctx, meal, alice, &private, nil, 1, "g"); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("voce con ricetta privata di Bob: err = %v, atteso ErrRecipeNotFound", err)
	}
	if _, err := plans.AddCheat(ctx, meal, alice, &private, nil, 1); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("sgarro con ricetta privata di Bob: err = %v, atteso ErrRecipeNotFound", err)
	}
}
