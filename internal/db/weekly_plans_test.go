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
		{"2026-04-26", "2026-04-20"}, // Sunday → the previous Monday
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

	// A template with 3 every-day slots + 1 Monday-only slot.
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

	// Apply it to a week.
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

	// 3 every-day × 7 + 1 Monday-only = 22 meals.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	totalMeals := 0
	for d := 0; d < 7; d++ {
		totalMeals += len(full.Days[d])
	}
	if totalMeals != 22 {
		t.Errorf("total meals = %d, want 22", totalMeals)
	}
	// Monday has 4 meals, the other days 3.
	if len(full.Days[0]) != 4 {
		t.Errorf("Mon: %d meals, want 4", len(full.Days[0]))
	}
	if len(full.Days[3]) != 3 {
		t.Errorf("Thu: %d meals, want 3", len(full.Days[3]))
	}

	// GetOrCreateForWeek is idempotent: the same plan.
	again, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	if again.ID != plan.ID {
		t.Error("GetOrCreateForWeek not idempotent")
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

	ingID, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	recID, _ := recipes.Create(ctx, alice, "Bruschetta", nil, 2, false)

	// An item as an ingredient.
	if _, err := plans.AddItem(ctx, mealID, alice, nil, &ingID, 50, "g"); err != nil {
		t.Fatalf("add ingredient item: %v", err)
	}
	// An item as a recipe.
	if _, err := plans.AddItem(ctx, mealID, alice, &recID, nil, 1, "g"); err != nil {
		t.Fatalf("add recipe item: %v", err)
	}
	// Error: both nil.
	if _, err := plans.AddItem(ctx, mealID, alice, nil, nil, 1, "g"); err == nil {
		t.Errorf("both nil should fail")
	}
	// Error: both non-nil.
	if _, err := plans.AddItem(ctx, mealID, alice, &recID, &ingID, 1, "g"); err == nil {
		t.Errorf("both non-nil should fail")
	}

	// LoadFull fills Recipe or Ingredient on each item.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	if len(full.Days[0]) != 1 || len(full.Days[0][0].Items) != 2 {
		t.Fatalf("want 1 meal with 2 items, got %v", full.Days[0])
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
		t.Errorf("not every item resolved: rec=%v ing=%v", sawRecipe, sawIngredient)
	}

	// RemoveItem from the wrong user fails.
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

	// 3 meals on day 0.
	m1, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "A")
	m2, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "B")
	m3, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "C")

	// 3 items in meal m1.
	ing1, _ := ings.CreateLocal(ctx, "X", nil, "g", 100, 1, 1, 1, &alice, nil, nil, nil, nil, nil, nil)
	ing2, _ := ings.CreateLocal(ctx, "Y", nil, "g", 100, 1, 1, 1, &alice, nil, nil, nil, nil, nil, nil)
	ing3, _ := ings.CreateLocal(ctx, "Z", nil, "g", 100, 1, 1, 1, &alice, nil, nil, nil, nil, nil, nil)
	i1, _ := plans.AddItem(ctx, m1, alice, nil, &ing1, 10, "g")
	i2, _ := plans.AddItem(ctx, m1, alice, nil, &ing2, 10, "g")
	i3, _ := plans.AddItem(ctx, m1, alice, nil, &ing3, 10, "g")

	// Reorder meals: C, A, B.
	if err := plans.ReorderMeals(ctx, plan.ID, alice, 0, []int64{m3, m1, m2}); err != nil {
		t.Fatalf("ReorderMeals: %v", err)
	}
	full, _ := plans.LoadFull(ctx, plan)
	gotOrder := []int64{full.Days[0][0].ID, full.Days[0][1].ID, full.Days[0][2].ID}
	if gotOrder[0] != m3 || gotOrder[1] != m1 || gotOrder[2] != m2 {
		t.Errorf("ReorderMeals: order after = %v, want [%d %d %d]", gotOrder, m3, m1, m2)
	}

	// Reorder items: i3, i1, i2.
	if err := plans.ReorderMealItems(ctx, m1, alice, []int64{i3, i1, i2}); err != nil {
		t.Fatalf("ReorderMealItems: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	// Find m1 in the list (it is second after the reorder).
	var m1Items []domain.MealItemFull
	for _, m := range full.Days[0] {
		if m.ID == m1 {
			m1Items = m.Items
		}
	}
	if len(m1Items) != 3 || m1Items[0].ID != i3 || m1Items[1].ID != i1 || m1Items[2].ID != i2 {
		t.Errorf("ReorderMealItems: order = %+v, want [%d %d %d]",
			[]int64{m1Items[0].ID, m1Items[1].ID, m1Items[2].ID}, i3, i1, i2)
	}

	// Mismatch: a foreign id -> ErrReorderMismatch, nothing written.
	if err := plans.ReorderMealItems(ctx, m1, alice, []int64{i1, i2, 9999}); !errors.Is(err, ErrReorderMismatch) {
		t.Errorf("foreign ids: want ErrReorderMismatch, got %v", err)
	}
	// Missing: a shorter input.
	if err := plans.ReorderMealItems(ctx, m1, alice, []int64{i1, i2}); !errors.Is(err, ErrReorderMismatch) {
		t.Errorf("partial ids: want ErrReorderMismatch, got %v", err)
	}

	// Bob cannot reorder Alice's meals.
	if err := plans.ReorderMeals(ctx, plan.ID, bob, 0, []int64{m1, m2, m3}); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("Bob reorder: want ErrPlanNotFound, got %v", err)
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
	weekB, _ := time.Parse("2006-01-02", "2026-05-04") // the following week

	planA, _ := plans.GetOrCreateForWeek(ctx, alice, weekA)
	planB, _ := plans.GetOrCreateForWeek(ctx, alice, weekB)

	// Fill planA: 2 meals with items.
	mA1, _ := plans.AddMeal(ctx, planA.ID, alice, 0, "Colazione")
	mA2, _ := plans.AddMeal(ctx, planA.ID, alice, 3, "Pranzo")

	ing1, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	rec1, _ := recipes.Create(ctx, alice, "Pasta", nil, 2, false)
	if _, err := plans.AddItem(ctx, mA1, alice, nil, &ing1, 50, "g"); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddItem(ctx, mA2, alice, &rec1, nil, 1, "g"); err != nil {
		t.Fatal(err)
	}

	// Copy into an empty dst -> ok.
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
		t.Errorf("planB meals=%d, want 2", totalMeals)
	}
	if totalItems != 2 {
		t.Errorf("planB items=%d, want 2", totalItems)
	}

	// Copy again onto the filled dst -> ErrPlanNotEmpty.
	if err := plans.CopyFrom(ctx, planA.ID, planB.ID, alice); !errors.Is(err, ErrPlanNotEmpty) {
		t.Errorf("re-copy: want ErrPlanNotEmpty, got %v", err)
	}

	// Bob cannot copy Alice's plan.
	weekC, _ := time.Parse("2006-01-02", "2026-05-11")
	planBobC, _ := plans.GetOrCreateForWeek(ctx, bob, weekC)
	if err := plans.CopyFrom(ctx, planA.ID, planBobC.ID, bob); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("Bob copy from Alice: want ErrPlanNotFound, got %v", err)
	}

	// Empty source -> ErrSourcePlanEmpty.
	weekD, _ := time.Parse("2006-01-02", "2026-05-18")
	weekE, _ := time.Parse("2006-01-02", "2026-05-25")
	planEmpty, _ := plans.GetOrCreateForWeek(ctx, alice, weekD)
	planDst, _ := plans.GetOrCreateForWeek(ctx, alice, weekE)
	if err := plans.CopyFrom(ctx, planEmpty.ID, planDst.ID, alice); !errors.Is(err, ErrSourcePlanEmpty) {
		t.Errorf("empty src: want ErrSourcePlanEmpty, got %v", err)
	}
}
