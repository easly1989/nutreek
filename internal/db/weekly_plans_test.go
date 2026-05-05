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

func TestReorderMealItemsAndMeals(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)

	// 3 meals nel giorno 0.
	m1, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "A")
	m2, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "B")
	m3, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "C")

	// 3 items nel meal m1.
	ing1, _ := ings.CreateLocal(ctx, "X", nil, "g", 100, 1, 1, 1, &alice)
	ing2, _ := ings.CreateLocal(ctx, "Y", nil, "g", 100, 1, 1, 1, &alice)
	ing3, _ := ings.CreateLocal(ctx, "Z", nil, "g", 100, 1, 1, 1, &alice)
	i1, _ := plans.AddItem(ctx, m1, alice, nil, &ing1, 10)
	i2, _ := plans.AddItem(ctx, m1, alice, nil, &ing2, 10)
	i3, _ := plans.AddItem(ctx, m1, alice, nil, &ing3, 10)

	// Reorder meals: C, A, B.
	if err := plans.ReorderMeals(ctx, plan.ID, alice, 0, []int64{m3, m1, m2}); err != nil {
		t.Fatalf("ReorderMeals: %v", err)
	}
	full, _ := plans.LoadFull(ctx, plan)
	gotOrder := []int64{full.Days[0][0].ID, full.Days[0][1].ID, full.Days[0][2].ID}
	if gotOrder[0] != m3 || gotOrder[1] != m1 || gotOrder[2] != m2 {
		t.Errorf("ReorderMeals: ordine dopo = %v, atteso [%d %d %d]", gotOrder, m3, m1, m2)
	}

	// Reorder items: i3, i1, i2.
	if err := plans.ReorderMealItems(ctx, m1, alice, []int64{i3, i1, i2}); err != nil {
		t.Fatalf("ReorderMealItems: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	// Trova m1 nella lista (e' il secondo dopo il reorder).
	var m1Items []domain.MealItemFull
	for _, m := range full.Days[0] {
		if m.ID == m1 {
			m1Items = m.Items
		}
	}
	if len(m1Items) != 3 || m1Items[0].ID != i3 || m1Items[1].ID != i1 || m1Items[2].ID != i2 {
		t.Errorf("ReorderMealItems: ordine = %+v, atteso [%d %d %d]",
			[]int64{m1Items[0].ID, m1Items[1].ID, m1Items[2].ID}, i3, i1, i2)
	}

	// Mismatch: id estraneo -> ErrReorderMismatch, niente scrittura.
	if err := plans.ReorderMealItems(ctx, m1, alice, []int64{i1, i2, 9999}); !errors.Is(err, ErrReorderMismatch) {
		t.Errorf("ids estranei: atteso ErrReorderMismatch, got %v", err)
	}
	// Mancante: input piu' corto.
	if err := plans.ReorderMealItems(ctx, m1, alice, []int64{i1, i2}); !errors.Is(err, ErrReorderMismatch) {
		t.Errorf("ids parziali: atteso ErrReorderMismatch, got %v", err)
	}

	// Bob non puo' riordinare i meals di Alice.
	if err := plans.ReorderMeals(ctx, plan.ID, bob, 0, []int64{m1, m2, m3}); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("Bob reorder: atteso ErrPlanNotFound, got %v", err)
	}
}

func TestCopyFrom(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	recipes := NewRecipes(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")

	weekA, _ := time.Parse("2006-01-02", "2026-04-27")
	weekB, _ := time.Parse("2006-01-02", "2026-05-04") // settimana successiva

	planA, _ := plans.GetOrCreateForWeek(ctx, alice, weekA)
	planB, _ := plans.GetOrCreateForWeek(ctx, alice, weekB)

	// Popolo planA: 2 pasti con items.
	mA1, _ := plans.AddMeal(ctx, planA.ID, alice, 0, "Colazione")
	mA2, _ := plans.AddMeal(ctx, planA.ID, alice, 3, "Pranzo")

	ing1, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice)
	rec1, _ := recipes.Create(ctx, alice, "Pasta", nil, 2, false)
	if _, err := plans.AddItem(ctx, mA1, alice, nil, &ing1, 50); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddItem(ctx, mA2, alice, &rec1, nil, 1); err != nil {
		t.Fatal(err)
	}

	// Copia dst non vuoto -> ok.
	if err := plans.CopyFrom(ctx, planA.ID, planB.ID, alice); err != nil {
		t.Fatalf("CopyFrom: %v", err)
	}
	full, err := plans.LoadFull(ctx, planB)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	totalMeals := 0
	totalItems := 0
	for d := 0; d < 7; d++ {
		for _, m := range full.Days[d] {
			totalMeals++
			totalItems += len(m.Items)
		}
	}
	if totalMeals != 2 {
		t.Errorf("planB meals=%d, atteso 2", totalMeals)
	}
	if totalItems != 2 {
		t.Errorf("planB items=%d, atteso 2", totalItems)
	}

	// Re-copia su dst gia' popolato -> ErrPlanNotEmpty.
	if err := plans.CopyFrom(ctx, planA.ID, planB.ID, alice); !errors.Is(err, ErrPlanNotEmpty) {
		t.Errorf("re-copy: atteso ErrPlanNotEmpty, got %v", err)
	}

	// Bob non puo' copiare il piano di Alice.
	weekC, _ := time.Parse("2006-01-02", "2026-05-11")
	planBobC, _ := plans.GetOrCreateForWeek(ctx, bob, weekC)
	if err := plans.CopyFrom(ctx, planA.ID, planBobC.ID, bob); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("Bob copy from Alice: atteso ErrPlanNotFound, got %v", err)
	}

	// Sorgente vuota -> ErrSourcePlanEmpty.
	weekD, _ := time.Parse("2006-01-02", "2026-05-18")
	weekE, _ := time.Parse("2006-01-02", "2026-05-25")
	planEmpty, _ := plans.GetOrCreateForWeek(ctx, alice, weekD)
	planDst, _ := plans.GetOrCreateForWeek(ctx, alice, weekE)
	if err := plans.CopyFrom(ctx, planEmpty.ID, planDst.ID, alice); !errors.Is(err, ErrSourcePlanEmpty) {
		t.Errorf("src vuoto: atteso ErrSourcePlanEmpty, got %v", err)
	}
}
