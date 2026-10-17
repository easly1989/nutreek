package db

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// On a scoped database a user sees the hand-made ingredients and public
// recipes of their peers only, plus every OpenFoodFacts product, and cannot
// use the others by id; an unscoped one shares everything (#185).
func TestCatalogScope(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	d := store.DB
	alice := newTestUser(t, store, "alice@x.test")
	ann := newTestUser(t, store, "ann@x.test")
	bob := newTestUser(t, store, "bob@x.test")

	ings, recipes, plans := NewIngredients(d), NewRecipes(d), NewWeeklyPlans(d)
	paste, _ := ings.CreateLocal(ctx, "Zesty paste", nil, "g", 100, 1, 1, 1, &alice, nil, nil, nil, nil, nil, nil)
	bar, _ := ings.CreateOFF(ctx, "8000000000001", "Zesty bar", nil, "g", 400, 5, 60, 15, &alice, nil, nil, nil, nil, nil, nil)
	bowl, _ := recipes.Create(ctx, alice, "Zesty bowl", nil, 1, true)

	names := func(viewer int64) (ingNames, recNames []string) {
		t.Helper()
		found, err := ings.SearchActive(ctx, viewer, "zesty", 20)
		if err != nil {
			t.Fatal(err)
		}
		listed, err := ings.ListActive(ctx, viewer, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != len(listed) {
			t.Errorf("viewer %d: search %d, list %d", viewer, len(found), len(listed))
		}
		for _, i := range found {
			ingNames = append(ingNames, i.Name)
		}
		visible, err := recipes.ListVisible(ctx, viewer)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range visible {
			recNames = append(recNames, r.Name)
		}
		return ingNames, recNames
	}

	// Unscoped, as a normal install: Bob sees and uses everything.
	if i, r := names(bob); len(i) != 2 || len(r) != 1 {
		t.Errorf("unscoped: Bob sees %v and %v", i, r)
	}

	// Alice and Ann are peers; Bob is alone.
	if _, err := d.Exec(`CREATE TABLE peers (user_id INTEGER, grp INTEGER); INSERT INTO peers VALUES (?, 1), (?, 1), (?, 2)`, alice, ann, bob); err != nil {
		t.Fatal(err)
	}
	ScopeCatalog(d, `SELECT user_id FROM peers WHERE grp = (SELECT grp FROM peers WHERE user_id = ?)`)
	t.Cleanup(func() { catalogScopes.Delete(d) })

	if i, r := names(ann); !slices.Equal(i, []string{"Zesty bar", "Zesty paste"}) || !slices.Equal(r, []string{"Zesty bowl"}) {
		t.Errorf("Ann sees %v and %v", i, r)
	}
	if i, r := names(bob); !slices.Equal(i, []string{"Zesty bar"}) || len(r) != 0 {
		t.Errorf("Bob sees %v and %v", i, r)
	}
	if _, err := ings.GetVisible(ctx, paste, bob); !errors.Is(err, ErrIngredientNotFound) {
		t.Errorf("Bob gets the paste by id: %v", err)
	}
	if _, err := ings.GetVisible(ctx, bar, bob); err != nil {
		t.Errorf("Bob does not get the OpenFoodFacts bar: %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, bowl, bob); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("Bob opens the bowl: %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, bowl, ann); err != nil {
		t.Errorf("Ann does not open the bowl: %v", err)
	}
	if m, _ := recipes.PerServingByRecipe(ctx, bob); len(m) != 0 {
		t.Errorf("Bob gets macros of %d recipes", len(m))
	}

	// Nor in a meal, by id.
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, bob, monday)
	meal, err := plans.AddMeal(ctx, plan.ID, bob, 0, "Lunch")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plans.AddItem(ctx, meal, bob, nil, &paste, 50, "g"); !errors.Is(err, ErrIngredientNotFound) {
		t.Errorf("Bob adds the paste to a meal: %v", err)
	}
	if _, err := plans.AddCheat(ctx, meal, bob, &bowl, nil, 1); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("Bob adds the bowl as a cheat: %v", err)
	}
	if _, err := plans.AddItem(ctx, meal, bob, nil, &bar, 50, "g"); err != nil {
		t.Errorf("Bob cannot add the OpenFoodFacts bar: %v", err)
	}
}
