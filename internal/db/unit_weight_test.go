package db

import (
	"context"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/macros"
)

// Le voci in pezzi («pz») valgono quantita' × peso unitario: i loader
// devono quindi portarsi dietro unit_weight_g dell'ingrediente (#126).

func TestLoadFullPezziPortaPesoUnitario(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	peso := 150.0
	mela, err := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, &peso, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Spuntino")
	if _, err := plans.AddItem(ctx, meal, alice, nil, &mela, 1, "pz"); err != nil {
		t.Fatal(err)
	}

	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	it := full.Days[0][0].Items[0]
	if it.Ingredient == nil || it.Ingredient.UnitWeightG == nil || *it.Ingredient.UnitWeightG != 150 {
		t.Fatalf("UnitWeightG non caricato: %+v", it.Ingredient)
	}
	if kcal := macros.ForMealItem(it).Kcal; !approxEqual(kcal, 78) {
		t.Errorf("kcal di 1 pz di mela = %.2f, atteso 78 (150 g)", kcal)
	}
}

func TestShoppingPezziInGrammi(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	shop := NewShoppingLists(store.DB, plans)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	peso := 150.0
	mela, _ := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, &peso, nil, nil, nil, nil, nil)
	// Senza peso unitario noto la quantita' resta quella inserita.
	kiwi, _ := ings.CreateLocal(ctx, "Kiwi", nil, "g", 61, 1.1, 14.7, 0.5, &alice, nil, nil, nil, nil, nil, nil)

	m1, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Spuntino")
	if _, err := plans.AddItem(ctx, m1, alice, nil, &mela, 2, "pz"); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddItem(ctx, m1, alice, nil, &kiwi, 1, "pz"); err != nil {
		t.Fatal(err)
	}
	m2, _ := plans.AddMeal(ctx, plan.ID, alice, 1, "Spuntino")
	if _, err := plans.AddItem(ctx, m2, alice, nil, &mela, 100, "g"); err != nil {
		t.Fatal(err)
	}

	list, err := shop.GetOrCreateList(ctx, alice, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	full, err := shop.LoadFull(ctx, list.ID, alice)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, it := range full.Items {
		got[it.Ingredient.Name] = it.TotalQuantity
	}
	if !approxEqual(got["Mela"], 400) { // 2 pz × 150 g + 100 g
		t.Errorf("Mela = %.2f, atteso 400", got["Mela"])
	}
	if !approxEqual(got["Kiwi"], 1) {
		t.Errorf("Kiwi = %.2f, atteso 1 (nessun peso unitario)", got["Kiwi"])
	}
}

func TestNutritionistPezziPortaPesoUnitario(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	np := NewNutritionistPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	plan, _ := np.GetOrCreateActivePlan(ctx, alice)
	mtID, _ := np.CreateMealType(ctx, plan.ID, "Spuntino")
	varID, _ := np.CreateVariation(ctx, mtID, nil)
	peso := 150.0
	mela, _ := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, &peso, nil, nil, nil, nil, nil)
	if _, err := np.AddItem(ctx, varID, nil, &mela, 1, "pz"); err != nil {
		t.Fatal(err)
	}

	full, err := np.LoadPlanFull(ctx, alice)
	if err != nil {
		t.Fatalf("LoadPlanFull: %v", err)
	}
	items := full.MealTypes[0].Variations[0].Items
	ing := items[0].Ingredient
	if ing == nil || ing.UnitWeightG == nil || *ing.UnitWeightG != 150 {
		t.Fatalf("UnitWeightG non caricato: %+v", ing)
	}
	if kcal := macros.ForNutritionistVariation(items, nil).Kcal; !approxEqual(kcal, 78) {
		t.Errorf("kcal variante con 1 pz di mela = %.2f, atteso 78", kcal)
	}
}
