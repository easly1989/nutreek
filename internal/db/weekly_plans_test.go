package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestMondayOf(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2026-04-26", "2026-04-20"}, // domenica → lunedi' precedente
		{"2026-04-27", "2026-04-27"}, // lunedi' identita'
		{"2026-04-28", "2026-04-27"}, // martedi'
		{"2026-05-03", "2026-04-27"}, // domenica successiva
	}
	for _, tc := range cases {
		in, _ := time.Parse("2006-01-02", tc.in)
		got := MondayOf(in).Format("2006-01-02")
		if got != tc.want {
			t.Errorf("MondayOf(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestWeeklyPlansApplyTemplate(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	tpls := NewMealTemplates(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")

	// Template con 3 slot ogni-giorno + 1 slot solo lunedi'.
	tplID, _ := tpls.Create(ctx, alice, "Test")
	if _, err := tpls.AddSlot(ctx, tplID, alice, domain.DayOfWeekEvery, "Colazione"); err != nil {
		t.Fatal(err)
	}
	if _, err := tpls.AddSlot(ctx, tplID, alice, domain.DayOfWeekEvery, "Pranzo"); err != nil {
		t.Fatal(err)
	}
	if _, err := tpls.AddSlot(ctx, tplID, alice, domain.DayOfWeekEvery, "Cena"); err != nil {
		t.Fatal(err)
	}
	if _, err := tpls.AddSlot(ctx, tplID, alice, 0, "Allenamento (lun)"); err != nil {
		t.Fatal(err)
	}

	// Apply a una settimana.
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, err := plans.GetOrCreateForWeek(ctx, alice, monday)
	if err != nil {
		t.Fatalf("GetOrCreateForWeek: %v", err)
	}
	if plan.WeekStart.Format("2006-01-02") != "2026-04-27" {
		t.Errorf("week start: %v", plan.WeekStart)
	}

	if err := plans.ApplyTemplate(ctx, plan.ID, alice, tplID); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	// 3 ogni-giorno × 7 + 1 solo lunedi' = 22 meals.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	totalMeals := 0
	for d := 0; d < 7; d++ {
		totalMeals += len(full.Days[d])
	}
	if totalMeals != 22 {
		t.Errorf("totale meals = %d, attesi 22", totalMeals)
	}
	// Lunedi' ha 4 pasti, gli altri giorni 3.
	if len(full.Days[0]) != 4 {
		t.Errorf("lun: %d pasti, attesi 4", len(full.Days[0]))
	}
	if len(full.Days[3]) != 3 {
		t.Errorf("gio: %d pasti, attesi 3", len(full.Days[3]))
	}

	// GetOrCreateForWeek e' idempotente: stesso piano.
	again, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	if again.ID != plan.ID {
		t.Error("GetOrCreateForWeek non idempotente")
	}
}

func TestWeeklyPlansItemsXOR(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	mealID, err := plans.AddMeal(ctx, plan.ID, alice, 0, "Colazione")
	if err != nil {
		t.Fatal(err)
	}

	ingID, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice)
	recID, _ := recipes.Create(ctx, alice, "Bruschetta", nil, 2, false)

	// Item come ingrediente.
	if _, err := plans.AddItem(ctx, mealID, alice, nil, &ingID, 50); err != nil {
		t.Fatalf("add ingredient item: %v", err)
	}
	// Item come ricetta.
	if _, err := plans.AddItem(ctx, mealID, alice, &recID, nil, 1); err != nil {
		t.Fatalf("add recipe item: %v", err)
	}
	// Errore: entrambi nil.
	if _, err := plans.AddItem(ctx, mealID, alice, nil, nil, 1); err == nil {
		t.Errorf("entrambi nil dovrebbe errore")
	}
	// Errore: entrambi non-nil.
	if _, err := plans.AddItem(ctx, mealID, alice, &recID, &ingID, 1); err == nil {
		t.Errorf("entrambi non-nil dovrebbe errore")
	}

	// LoadFull popola Recipe vs Ingredient su ciascun item.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	if len(full.Days[0]) != 1 || len(full.Days[0][0].Items) != 2 {
		t.Fatalf("attesi 1 meal con 2 items, got %v", full.Days[0])
	}
	var sawRecipe, sawIngredient bool
	for _, it := range full.Days[0][0].Items {
		if it.Recipe != nil && it.Recipe.Name == "Bruschetta" {
			sawRecipe = true
		}
		if it.Ingredient != nil && it.Ingredient.Name == "Pane" {
			sawIngredient = true
		}
	}
	if !sawRecipe || !sawIngredient {
		t.Errorf("non tutti gli items risolti: rec=%v ing=%v", sawRecipe, sawIngredient)
	}

	// RemoveItem da utente sbagliato fallisce.
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	if err := plans.RemoveItem(ctx, full.Days[0][0].Items[0].ID, bob); !errors.Is(err, ErrMealItemNotFound) {
		t.Errorf("Bob remove item dovrebbe fallire, got %v", err)
	}
}
