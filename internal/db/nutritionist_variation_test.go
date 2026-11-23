package db

import (
	"context"
	"errors"
	"testing"
)

// A variant is renamed and copied only inside your own plan (#251); the copy
// has the same items and the original keeps its own.
func TestNutritionistRenameAndDuplicateVariation(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	np := NewNutritionistPlans(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	alicePlan, _ := np.GetOrCreateActivePlan(ctx, alice)
	bobPlan, _ := np.GetOrCreateActivePlan(ctx, bob)
	mt, _ := np.CreateMealType(ctx, alicePlan.ID, "Pranzo")
	bobMT, _ := np.CreateMealType(ctx, bobPlan.ID, "Pranzo")
	v, _ := np.CreateVariation(ctx, alicePlan.ID, mt, nil)
	bobV, _ := np.CreateVariation(ctx, bobPlan.ID, bobMT, nil)
	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	if _, err := np.AddItem(ctx, alicePlan.ID, v, nil, &pane, 80, "g"); err != nil {
		t.Fatal(err)
	}

	name := "Light"
	if err := np.RenameVariation(ctx, v, alicePlan.ID, &name); err != nil {
		t.Fatal(err)
	}
	if err := np.RenameVariation(ctx, bobV, alicePlan.ID, &name); !errors.Is(err, ErrNutritionistItemNotFound) {
		t.Errorf("renaming Bob's variant = %v, want ErrNutritionistItemNotFound", err)
	}

	copyName := "Light (copy)"
	nv, err := np.DuplicateVariation(ctx, v, alicePlan.ID, &copyName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := np.DuplicateVariation(ctx, bobV, alicePlan.ID, &copyName); !errors.Is(err, ErrNutritionistItemNotFound) {
		t.Errorf("copying Bob's variant = %v, want ErrNutritionistItemNotFound", err)
	}

	full, err := np.LoadPlanFull(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	items := map[int64]int{}
	for _, m := range full.MealTypes {
		for _, vv := range m.Variations {
			if vv.Name != nil {
				got = append(got, *vv.Name)
			}
			items[vv.ID] = len(vv.Items)
		}
	}
	if len(got) != 2 || got[0] != "Light" || got[1] != "Light (copy)" {
		t.Errorf("variants = %v, want [Light, Light (copy)]", got)
	}
	if items[v] != 1 || items[nv] != 1 {
		t.Errorf("items: original %d, copy %d, want 1 and 1", items[v], items[nv])
	}
}
