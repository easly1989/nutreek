package app

import (
	"context"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// With no text, the pickers list what the user used before first, while the
// ingredients page keeps its alphabetical list (#266).
func TestPickerListsUsedIngredientsFirst(t *testing.T) {
	a := newTestApp(t)
	user := mustCreateTestUser(t, a, "usati@esempio.test", "password-demo")
	ctx := contextWithUser(context.Background(), user)
	mk := func(name string) int64 {
		id, err := a.Ingredients.CreateLocal(ctx, name, nil, domain.UnitGram, 1, 1, 1, 1, &user.ID, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	mk("Aglio")
	mk("Basilico")
	used := mk("Zucchine")
	plans := db.NewWeeklyPlans(a.Store.DB)
	monday, _ := time.Parse("2006-01-02", "2026-11-16")
	plan, _ := plans.GetOrCreateForWeek(ctx, user.ID, monday)
	meal, _ := plans.AddMeal(ctx, plan.ID, user.ID, 0, "Pranzo")
	if _, err := plans.AddItem(ctx, meal, user.ID, nil, &used, 200, "g"); err != nil {
		t.Fatal(err)
	}

	first := func(rows []ingredientRow) string {
		if len(rows) == 0 || rows[0].Local == nil {
			t.Fatalf("no local rows: %+v", rows)
		}
		return rows[0].Local.Name
	}
	rows, _, err := a.searchPicker(ctx, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := first(rows); got != "Zucchine" {
		t.Errorf("picker with no text starts with %q, want the used one", got)
	}
	if len(rows) != 3 {
		t.Errorf("picker lists %d rows, want 3", len(rows))
	}
	rows, _, err = a.searchMixed(ctx, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := first(rows); got != "Aglio" {
		t.Errorf("ingredients page starts with %q, want the alphabetical first", got)
	}
}
