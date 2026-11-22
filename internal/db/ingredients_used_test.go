package db

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// The search puts first what the viewer used before: in their plans and
// recipes, the most used first; then the ingredients they made, then the
// OpenFoodFacts ones. Someone else's use does not count (#225).
func TestSearchActiveUsedFirst(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	recipes := NewRecipes(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	mkLocal := func(name string) int64 {
		id, err := ings.CreateLocal(ctx, name, nil, domain.UnitGram, 100, 5, 20, 1, &alice, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	mkOFF := func(code, name string) int64 {
		id, err := ings.CreateOFF(ctx, code, name, nil, domain.UnitGram, 100, 5, 20, 1, &alice, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	unused := mkLocal("Pasta A")
	heavy := mkLocal("Pasta B")
	imported := mkOFF("8000000000011", "Pasta C")
	offUnused := mkOFF("8000000000012", "Pasta D")
	inRecipe := mkLocal("Pasta E")

	monday, _ := time.Parse("2006-01-02", "2026-11-09")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	meal, _ := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	for i := 0; i < 3; i++ {
		if _, err := plans.AddItem(ctx, meal, alice, nil, &heavy, 100, "g"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := plans.AddItem(ctx, meal, alice, nil, &imported, 100, "g"); err != nil {
		t.Fatal(err)
	}
	rec, err := recipes.Create(ctx, alice, "Sugo", nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := recipes.AddItem(ctx, rec, alice, inRecipe, 100, "g"); err != nil {
			t.Fatal(err)
		}
	}
	// Bob uses the unused one a lot: it does not move for Alice.
	bobPlan, _ := plans.GetOrCreateForWeek(ctx, bob, monday)
	bobMeal, _ := plans.AddMeal(ctx, bobPlan.ID, bob, 0, "Pranzo")
	for i := 0; i < 5; i++ {
		_, _ = plans.AddItem(ctx, bobMeal, bob, nil, &unused, 100, "g")
	}

	got, err := ings.SearchActive(ctx, alice, "pasta", 20)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Pasta B", "Pasta E", "Pasta C", "Pasta A", "Pasta D"}
	if gotNames := names(got); !slices.Equal(gotNames, want) {
		t.Errorf("order = %v, want %v (used most first, then made by hand, then imported)", gotNames, want)
	}
	_ = offUnused

	// Used as much, the more recent first.
	older := mkLocal("Riso vecchio")
	newer := mkLocal("Riso nuovo")
	for _, c := range []struct {
		id   int64
		week string
	}{{older, "2026-01-05"}, {newer, "2026-11-02"}} {
		wk, _ := time.Parse("2006-01-02", c.week)
		p, _ := plans.GetOrCreateForWeek(ctx, alice, wk)
		m, _ := plans.AddMeal(ctx, p.ID, alice, 0, "Pranzo")
		if _, err := plans.AddItem(ctx, m, alice, nil, &c.id, 100, "g"); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := ings.SearchActive(ctx, alice, "riso", 20); !slices.Equal(names(got), []string{"Riso nuovo", "Riso vecchio"}) {
		t.Errorf("same uses: %v, want the more recent first", names(got))
	}

	// An exact name still comes first, used or not.
	if got, _ := ings.SearchActive(ctx, alice, "Pasta D", 20); len(got) == 0 || got[0].Name != "Pasta D" {
		t.Errorf("exact name: %v", names(got))
	}
}

// CatalogBarcodes says which products the viewer already has as ingredients, so
// the search shows a product and its ingredient once.
func TestCatalogBarcodes(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	if _, err := ings.CreateOFF(ctx, "8000000000021", "Fage", nil, domain.UnitGram, 97, 9, 4, 5, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	local, _ := ings.CreateLocal(ctx, "Mio", nil, domain.UnitGram, 1, 1, 1, 1, nil, nil, nil, nil, nil, nil, nil)
	if err := ings.SetLocalBarcode(ctx, local, "8000000000022"); err != nil {
		t.Fatal(err)
	}
	got, err := ings.CatalogBarcodes(ctx, 0, []string{"8000000000021", "8000000000022", "8000000000023"})
	if err != nil {
		t.Fatal(err)
	}
	if !got["8000000000021"] || !got["8000000000022"] || got["8000000000023"] || len(got) != 2 {
		t.Errorf("barcodes = %v, want the import and the linked local one", got)
	}
	if got, _ := ings.CatalogBarcodes(ctx, 0, nil); len(got) != 0 {
		t.Errorf("no codes: %v", got)
	}
}

// off_aligned is on for an import and goes off once an edit changes what
// OpenFoodFacts gave; saving the same values keeps it, and a hand-made
// ingredient is never aligned.
func TestOFFAlignedFlag(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	brand := "Fage"
	id, err := ings.CreateOFF(ctx, "8000000000031", "Yogurt greco", &brand, domain.UnitGram, 97.0, 9.0, 4.0, 5.0, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	aligned := func(id int64) bool {
		t.Helper()
		ing, err := ings.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return ing.OFFAligned
	}
	if !aligned(id) {
		t.Fatal("an import must start aligned")
	}
	same := func() error {
		return ings.Update(ctx, id, "Yogurt greco", &brand, domain.UnitGram, 97.004, 9, 4, 5, nil, nil, nil, nil, nil, nil)
	}
	if err := same(); err != nil {
		t.Fatal(err)
	}
	if !aligned(id) {
		t.Error("saving the same values (within rounding) must keep it aligned")
	}
	if err := ings.Update(ctx, id, "Yogurt greco", &brand, domain.UnitGram, 97, 9, 4, 5, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := ings.Update(ctx, id, "Yogurt greco", &brand, domain.UnitGram, 120, 9, 4, 5, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if aligned(id) {
		t.Error("changing the kcal must clear it")
	}
	if err := same(); err != nil {
		t.Fatal(err)
	}
	if aligned(id) {
		t.Error("going back to the old values must not set it again: an edit was made")
	}

	local, _ := ings.CreateLocal(ctx, "Mio", nil, domain.UnitGram, 1, 1, 1, 1, nil, nil, nil, nil, nil, nil, nil)
	if aligned(local) {
		t.Error("a hand-made ingredient is never aligned")
	}
	// A new weight per piece or a micronutrient is also a change.
	id2, _ := ings.CreateOFF(ctx, "8000000000032", "Pane", nil, domain.UnitGram, 250, 8, 50, 2, nil, nil, nil, nil, nil, nil, nil)
	w := 40.0
	if err := ings.Update(ctx, id2, "Pane", nil, domain.UnitGram, 250, 8, 50, 2, &w, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if aligned(id2) {
		t.Error("a weight per piece added by hand must clear it")
	}
}
