package db

import (
	"context"
	"testing"
	"time"
)

// TestWeeklyPlansCreateMeals verifica che CreateMeals inserisca pasti e voci
// nel piano corretto, in transazione, con ownership verificata.
func TestWeeklyPlansCreateMeals(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")

	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	riso, _ := ings.CreateLocal(ctx, "Riso", nil, "g", 350, 7, 78, 1, &alice, nil, nil, nil, nil, nil, nil)

	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, err := plans.GetOrCreateForWeek(ctx, alice, monday)
	if err != nil {
		t.Fatalf("GetOrCreateForWeek: %v", err)
	}

	meals := []GeneratedMeal{
		{DayOfWeek: 0, Name: "Colazione", SortOrder: 0, Items: []GeneratedMealItem{
			{IngredientID: &pane, Quantity: 80, QuantityUnit: "g"},
		}},
		{DayOfWeek: 0, Name: "Pranzo", SortOrder: 1, Items: []GeneratedMealItem{
			{IngredientID: &riso, Quantity: 100, QuantityUnit: "g"},
			{IngredientID: &pane, Quantity: 50, QuantityUnit: "g"},
		}},
		{DayOfWeek: 3, Name: "Cena", SortOrder: 0, Items: []GeneratedMealItem{
			{IngredientID: &riso, Quantity: 120, QuantityUnit: "g"},
		}},
	}

	// Ownership: bob non puo' scrivere sul piano di alice.
	if err := plans.CreateMeals(ctx, plan.ID, bob, meals); err == nil {
		t.Fatal("CreateMeals doveva fallire per ownership errata")
	}

	if err := plans.CreateMeals(ctx, plan.ID, alice, meals); err != nil {
		t.Fatalf("CreateMeals: %v", err)
	}

	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	if got := len(full.Days[0]); got != 2 {
		t.Errorf("giorno 0: %d pasti, attesi 2", got)
	}
	if got := len(full.Days[3]); got != 1 {
		t.Errorf("giorno 3: %d pasti, attesi 1", got)
	}
	// Pranzo (giorno 0) ha 2 voci.
	var pranzoItems int
	for _, m := range full.Days[0] {
		if m.Name == "Pranzo" {
			pranzoItems = len(m.Items)
		}
	}
	if pranzoItems != 2 {
		t.Errorf("Pranzo: %d voci, attese 2", pranzoItems)
	}
}

// TestNutritionistHasVariations verifica il flag minimo richiesto per la
// generazione automatica: almeno una variante con almeno un item.
func TestNutritionistHasVariations(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	np := NewNutritionistPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")

	// Nessun item ancora: false.
	has, err := np.HasVariations(ctx, alice)
	if err != nil {
		t.Fatalf("HasVariations: %v", err)
	}
	if has {
		t.Error("atteso false senza varianti/item")
	}

	plan, _ := np.GetOrCreateActivePlan(ctx, alice)
	mtID, _ := np.CreateMealType(ctx, plan.ID, "Colazione")
	varID, _ := np.CreateVariation(ctx, plan.ID, mtID, nil)

	// Variante senza item: ancora false.
	has, _ = np.HasVariations(ctx, alice)
	if has {
		t.Error("atteso false con variante vuota")
	}

	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	if _, err := np.AddItem(ctx, plan.ID, varID, nil, &pane, 80, "g"); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	// Ora c'e' un item: true.
	has, _ = np.HasVariations(ctx, alice)
	if !has {
		t.Error("atteso true con almeno un item")
	}
}
