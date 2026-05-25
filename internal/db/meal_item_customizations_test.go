package db

import (
	"context"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// TestMealItemCustomizationsPipeline verifica il ciclo completo:
// salva variazioni → LoadFull → ForMealItem usa le variazioni.
func TestMealItemCustomizationsPipeline(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	plans := NewWeeklyPlans(store.DB)
	customs := NewMealItemCustomizations(store.DB)

	// Setup: utente, due ingredienti con valori nutrizionali distinti.
	uid, _ := users.Create(ctx, "test@ex.com", "h", "Test")

	// Müesli: 480 kcal/100g, Latte: 48 kcal/100ml (ratio 10:1).
	muesliID, err := ings.CreateLocal(ctx, "Müesli test", nil, domain.UnitGram, 480, 10, 60, 14, &uid)
	if err != nil {
		t.Fatalf("CreateLocal muesli: %v", err)
	}
	latteID, err := ings.CreateLocal(ctx, "Latte test", nil, domain.UnitMilliliter, 48, 3.5, 5, 1.6, &uid)
	if err != nil {
		t.Fatalf("CreateLocal latte: %v", err)
	}

	// Ricetta: 100g müesli + 300ml latte → 480+144 = 624 kcal (1 porzione).
	recID, _ := recipes.Create(ctx, uid, "Colazione test", nil, 1, false)
	if _, err := recipes.AddItem(ctx, recID, uid, muesliID, 100); err != nil {
		t.Fatalf("AddItem muesli: %v", err)
	}
	if _, err := recipes.AddItem(ctx, recID, uid, latteID, 300); err != nil {
		t.Fatalf("AddItem latte: %v", err)
	}

	// Piano: aggiungi 1 porzione della ricetta al lunedì.
	monday, _ := time.Parse("2006-01-02", "2026-05-26")
	plan, _ := plans.GetOrCreateForWeek(ctx, uid, monday)
	mealID, _ := plans.AddMeal(ctx, plan.ID, uid, 0, "Colazione")
	itemID, err := plans.AddItem(ctx, mealID, uid, &recID, nil, 1)
	if err != nil {
		t.Fatalf("AddItem piano: %v", err)
	}

	// Verifica macro SENZA variazioni: 624 kcal.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	item := findItem(t, full, itemID)
	if item.IsCustomized() {
		t.Error("non dovrebbe avere variazioni prima di Replace")
	}
	baseKcal := macros.ForMealItem(*item).Kcal
	if baseKcal < 623.9 || baseKcal > 624.1 {
		t.Errorf("kcal base: got %.2f want ~624", baseKcal)
	}

	// Salva variazioni: 45g müesli + 300ml latte → 360 kcal.
	muesliIng, _ := ings.GetByID(ctx, muesliID)
	latteIng, _ := ings.GetByID(ctx, latteID)
	err = customs.Replace(ctx, itemID, uid, []CustomizationInput{
		{IngredientID: muesliIng.ID, Quantity: 45},
		{IngredientID: latteIng.ID, Quantity: 300},
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// Ricarica il piano e verifica che le variazioni siano usate.
	full, err = plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull dopo variazioni: %v", err)
	}
	item = findItem(t, full, itemID)
	if !item.IsCustomized() {
		t.Fatal("item deve essere customizzato dopo Replace")
	}
	if len(item.Customizations) != 2 {
		t.Fatalf("variazioni: got %d want 2", len(item.Customizations))
	}
	for _, c := range item.Customizations {
		if c.Ingredient == nil {
			t.Fatal("Ingredient deve essere popolato nelle variazioni")
		}
	}

	customKcal := macros.ForMealItem(*item).Kcal
	// 45*4.80 + 300*0.48 = 216+144 = 360
	if customKcal < 359.9 || customKcal > 360.1 {
		t.Errorf("kcal variazioni 45g+300ml: got %.2f want ~360", customKcal)
	}
	if customKcal >= baseKcal-0.1 && customKcal <= baseKcal+0.1 {
		t.Error("le variazioni devono cambiare i macro rispetto alla ricetta base")
	}

	// Cambia variazioni: 50g müesli + 250ml latte → ancora 360 kcal (coincidenza M=10L).
	// Le proteine però cambiano: 45g→4.5g vs 50g→5g (müesli), 300ml→10.5g vs 250ml→8.75g (latte).
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
	// kcal: 50*4.80 + 250*0.48 = 240+120 = 360 (stesso, coincidenza numerica)
	if m50.Kcal < 359.9 || m50.Kcal > 360.1 {
		t.Errorf("kcal variazioni 50g+250ml: got %.2f want ~360", m50.Kcal)
	}
	// proteine: 50*0.1 + 250*0.035 = 5+8.75 = 13.75 (diverso da 45g+300ml=15g)
	if m50.Protein > 13.8 || m50.Protein < 13.7 {
		t.Errorf("proteine 50g+250ml: got %.4f want ~13.75", m50.Protein)
	}

	// Reset: rimuove tutte le variazioni.
	if err := customs.Replace(ctx, itemID, uid, nil); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	item = findItem(t, full, itemID)
	if item.IsCustomized() {
		t.Error("dopo reset non ci devono essere variazioni")
	}
	resetKcal := macros.ForMealItem(*item).Kcal
	if resetKcal < 623.9 || resetKcal > 624.1 {
		t.Errorf("kcal dopo reset: got %.2f want ~624 (ricetta base)", resetKcal)
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
	t.Fatalf("item %d non trovato nel piano", itemID)
	return nil
}
