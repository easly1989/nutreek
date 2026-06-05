package db

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
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

	// Ricetta: Pasta al pomodoro, 4 porzioni, 200g pasta + 100g pomodoro per ricetta intera.
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 360, 12, 75, 1, &alice, nil, nil, nil, nil, nil, nil)
	pom, _ := ings.CreateLocal(ctx, "Pomodoro", nil, "g", 18, 1, 4, 0, &alice, nil, nil, nil, nil, nil, nil)
	rid, _ := recipes.Create(ctx, alice, "Pasta al pomodoro", nil, 4, false)
	if _, err := recipes.AddItem(ctx, rid, alice, pasta, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := recipes.AddItem(ctx, rid, alice, pom, 100); err != nil {
		t.Fatal(err)
	}

	// Ingrediente diretto in altro pasto: Banana 120g.
	banana, _ := ings.CreateLocal(ctx, "Banana", nil, "g", 89, 1, 23, 0, &alice, nil, nil, nil, nil, nil, nil)

	// Pasto 1 lunedi': 1 porzione di pasta al pomodoro (=> 50g pasta + 25g pomodoro).
	m1, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	if _, err := plans.AddItem(ctx, m1, alice, &rid, nil, 1, "g"); err != nil {
		t.Fatal(err)
	}
	// Pasto 2 martedi': 2 porzioni di pasta al pomodoro (=> 100g pasta + 50g pomodoro).
	m2, _ := plans.AddMeal(ctx, plan.ID, alice, 1, "Pranzo")
	if _, err := plans.AddItem(ctx, m2, alice, &rid, nil, 2, "g"); err != nil {
		t.Fatal(err)
	}
	// Pasto 3 lunedi': 120g banana.
	m3, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Spuntino")
	if _, err := plans.AddItem(ctx, m3, alice, nil, &banana, 120, "g"); err != nil {
		t.Fatal(err)
	}
	// Pasto 4 mercoledi' SGARRATO: pasta normale, ma sgarro hamburger.
	m4, _ := plans.AddMeal(ctx, plan.ID, alice, 2, "Cena")
	if _, err := plans.AddItem(ctx, m4, alice, &rid, nil, 1, "g"); err != nil {
		t.Fatal(err)
	}
	hamb, _ := ings.CreateLocal(ctx, "Hamburger", nil, "g", 250, 14, 5, 18, &alice, nil, nil, nil, nil, nil, nil)
	if _, err := plans.AddCheat(ctx, m4, alice, nil, &hamb, 200); err != nil {
		t.Fatal(err)
	}

	// Crea lista e genera.
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

	// Atteso: pasta = 1+2 porzioni × 50g/porzione = 150g (NON include la
	// porzione di m4 perché m4 è sgarrato e va ignorato).
	// pomodoro = (1+2) × 25g = 75g.
	// banana = 120g.
	// Hamburger NON deve essere in lista (cheat).
	got := make(map[string]float64, len(full.Items))
	for _, it := range full.Items {
		got[it.Ingredient.Name] = it.TotalQuantity
	}
	if !approxEqual(got["Pasta"], 150) {
		t.Errorf("Pasta = %.2f, atteso 150", got["Pasta"])
	}
	if !approxEqual(got["Pomodoro"], 75) {
		t.Errorf("Pomodoro = %.2f, atteso 75", got["Pomodoro"])
	}
	if !approxEqual(got["Banana"], 120) {
		t.Errorf("Banana = %.2f, atteso 120", got["Banana"])
	}
	if _, ok := got["Hamburger"]; ok {
		t.Errorf("Hamburger non dovrebbe essere in lista (sgarro): %v", got)
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
	// Aggiungo voce manuale.
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

	// Rigenero: voce manuale persiste, checked sull'auto-item resta perche'
	// la quantita' non e' cambiata.
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	full, _ = shop.LoadFull(ctx, list.ID, alice)
	if len(full.UserItems) != 1 || full.UserItems[0].Description != "vino rosso" || !full.UserItems[0].Checked {
		t.Errorf("user item perso o checked perso: %+v", full.UserItems)
	}
	if len(full.Items) != 1 || !full.Items[0].Checked {
		t.Errorf("auto checked perso dopo regenerate (stessa qty): %+v", full.Items)
	}

	// Cambio quantita' del meal: rigenero, checked deve resettarsi (qty cambiata).
	if _, err := plans.AddItem(ctx, m1, alice, nil, &pasta, 50, "g"); err != nil {
		t.Fatal(err)
	}
	if err := shop.Generate(ctx, list.ID, alice); err != nil {
		t.Fatal(err)
	}
	full, _ = shop.LoadFull(ctx, list.ID, alice)
	if full.Items[0].Checked {
		t.Errorf("checked dovrebbe essere resettato dopo cambio quantita': qty=%.2f", full.Items[0].TotalQuantity)
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
		t.Errorf("Bob LoadFull: atteso ErrShoppingListNotFound, got %v", err)
	}
	if err := shop.Generate(ctx, listAlice.ID, bob); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob Generate: atteso ErrShoppingListNotFound, got %v", err)
	}
	if _, err := shop.AddUserItem(ctx, listAlice.ID, bob, "pane", ""); !errors.Is(err, ErrShoppingListNotFound) {
		t.Errorf("Bob AddUserItem: atteso ErrShoppingListNotFound, got %v", err)
	}
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}
