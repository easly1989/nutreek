package db

import (
	"context"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// TestMealItemCustomizationsPipeline checks the full cycle:
// save variations → LoadFull → ForMealItem uses the variations.
func TestMealItemCustomizationsPipeline(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	plans := NewWeeklyPlans(store.DB)
	customs := NewMealItemCustomizations(store.DB)

	// Setup: a user, two ingredients with different nutrition values.
	uid, _ := users.Create(ctx, "test@ex.com", "h", "Test")

	// Muesli: 480 kcal/100g, milk: 48 kcal/100ml (ratio 10:1).
	muesliID, err := ings.CreateLocal(ctx, "Müesli test", nil, domain.UnitGram, 480, 10, 60, 14, &uid, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateLocal muesli: %v", err)
	}
	latteID, err := ings.CreateLocal(ctx, "Latte test", nil, domain.UnitMilliliter, 48, 3.5, 5, 1.6, &uid, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateLocal latte: %v", err)
	}

	// Recipe: 100g muesli + 300ml milk → 480+144 = 624 kcal (1 serving).
	recID, _ := recipes.Create(ctx, uid, "Colazione test", nil, 1, false)
	if _, err := recipes.AddItem(ctx, recID, uid, muesliID, 100); err != nil {
		t.Fatalf("AddItem muesli: %v", err)
	}
	if _, err := recipes.AddItem(ctx, recID, uid, latteID, 300); err != nil {
		t.Fatalf("AddItem latte: %v", err)
	}

	// Plan: add 1 serving of the recipe to Monday.
	monday, _ := time.Parse("2006-01-02", "2026-05-26")
	plan, _ := plans.GetOrCreateForWeek(ctx, uid, monday)
	mealID, _ := plans.AddMeal(ctx, plan.ID, uid, 0, "Colazione")
	itemID, err := plans.AddItem(ctx, mealID, uid, &recID, nil, 1, "g")
	if err != nil {
		t.Fatalf("AddItem plan: %v", err)
	}

	// Check the macros WITHOUT variations: 624 kcal.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	item := findItem(t, full, itemID)
	if item.IsCustomized() {
		t.Error("there should be no variations before Replace")
	}
	baseKcal := macros.ForMealItem(*item).Kcal
	if baseKcal < 623.9 || baseKcal > 624.1 {
		t.Errorf("kcal base: got %.2f want ~624", baseKcal)
	}

	// Save variations: 45g muesli + 300ml milk → 360 kcal.
	muesliIng, _ := ings.GetByID(ctx, muesliID)
	latteIng, _ := ings.GetByID(ctx, latteID)
	err = customs.Replace(ctx, itemID, uid, []CustomizationInput{
		{IngredientID: muesliIng.ID, Quantity: 45},
		{IngredientID: latteIng.ID, Quantity: 300},
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// Reload the plan and check the variations are used.
	full, err = plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull after variations: %v", err)
	}
	item = findItem(t, full, itemID)
	if !item.IsCustomized() {
		t.Fatal("the item must be customized after Replace")
	}
	if len(item.Customizations) != 2 {
		t.Fatalf("variazioni: got %d want 2", len(item.Customizations))
	}
	for _, c := range item.Customizations {
		if c.Ingredient == nil {
			t.Fatal("Ingredient must be filled in the variations")
		}
	}

	customKcal := macros.ForMealItem(*item).Kcal
	// 45*4.80 + 300*0.48 = 216+144 = 360
	if customKcal < 359.9 || customKcal > 360.1 {
		t.Errorf("kcal variazioni 45g+300ml: got %.2f want ~360", customKcal)
	}
	if customKcal >= baseKcal-0.1 && customKcal <= baseKcal+0.1 {
		t.Error("the variations must change the macros against the base recipe")
	}

	// Change the variations: 50g muesli + 250ml milk → still 360 kcal (coincidence M=10L).
	// Protein changes though: 45g→4.5g vs 50g→5g (muesli), 300ml→10.5g vs 250ml→8.75g (milk).
	err = customs.Replace(ctx, itemID, uid, []CustomizationInput{
		{IngredientID: muesliIng.ID, Quantity: 50},
		{IngredientID: latteIng.ID, Quantity: 250},
	})
	if err != nil {
		t.Fatalf("Replace secondo: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	item = findItem(t, full, itemID)
	m50 := macros.ForMealItem(*item)
	// kcal: 50*4.80 + 250*0.48 = 240+120 = 360 (the same, a numeric coincidence)
	if m50.Kcal < 359.9 || m50.Kcal > 360.1 {
		t.Errorf("kcal variazioni 50g+250ml: got %.2f want ~360", m50.Kcal)
	}
	// protein: 50*0.1 + 250*0.035 = 5+8.75 = 13.75 (unlike 45g+300ml=15g)
	if m50.Protein > 13.8 || m50.Protein < 13.7 {
		t.Errorf("proteine 50g+250ml: got %.4f want ~13.75", m50.Protein)
	}

	// Reset: remove every variation.
	if err := customs.Replace(ctx, itemID, uid, nil); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	item = findItem(t, full, itemID)
	if item.IsCustomized() {
		t.Error("after reset there must be no variations")
	}
	resetKcal := macros.ForMealItem(*item).Kcal
	if resetKcal < 623.9 || resetKcal > 624.1 {
		t.Errorf("kcal after reset: got %.2f want ~624 (base recipe)", resetKcal)
	}
}

func findItem(t *testing.T, full *domain.WeeklyPlanFull, itemID int64) *domain.MealItemFull {
	t.Helper()
	for _, dayMeals := range full.Days {
		for _, m := range dayMeals {
			for i := range m.Items {
				if m.Items[i].ID == itemID {
					return &m.Items[i]
				}
			}
		}
	}
	t.Fatalf("item %d not found in the plan", itemID)
	return nil
}
