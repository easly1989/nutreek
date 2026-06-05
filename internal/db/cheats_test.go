package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheats(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	plans := NewWeeklyPlans(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	monday, _ := time.Parse("2006-01-02", "2026-04-27")
	plan, _ := plans.GetOrCreateForWeek(ctx, alice, monday)
	mealID, err := plans.AddMeal(ctx, plan.ID, alice, 0, "Pranzo")
	if err != nil {
		t.Fatal(err)
	}

	// Aggiungi un item ufficiale (non sgarro), poi 2 sgarri.
	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	if _, err := plans.AddItem(ctx, mealID, alice, nil, &pane, 50, "g"); err != nil {
		t.Fatal(err)
	}
	hamb, _ := ings.CreateLocal(ctx, "Hamburger", nil, "g", 250, 14, 5, 18, &alice, nil, nil, nil, nil, nil, nil)
	pat, _ := ings.CreateLocal(ctx, "Patatine", nil, "g", 312, 4, 41, 15, &alice, nil, nil, nil, nil, nil, nil)

	c1, err := plans.AddCheat(ctx, mealID, alice, nil, &hamb, 200)
	if err != nil {
		t.Fatalf("AddCheat hamb: %v", err)
	}
	c2, err := plans.AddCheat(ctx, mealID, alice, nil, &pat, 150)
	if err != nil {
		t.Fatalf("AddCheat pat: %v", err)
	}

	// Bob non puo' aggiungere sgarri al pasto di Alice.
	if _, err := plans.AddCheat(ctx, mealID, bob, nil, &pat, 100); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("Bob AddCheat: atteso ErrPlanNotFound, got %v", err)
	}

	// LoadFull include i cheats.
	full, err := plans.LoadFull(ctx, plan)
	if err != nil {
		t.Fatalf("LoadFull: %v", err)
	}
	if len(full.Days[0]) != 1 {
		t.Fatalf("attesi 1 meal, got %d", len(full.Days[0]))
	}
	mw := full.Days[0][0]
	if len(mw.Items) != 1 {
		t.Errorf("items = %d, atteso 1 (pane)", len(mw.Items))
	}
	if len(mw.Cheats) != 2 {
		t.Errorf("cheats = %d, attesi 2", len(mw.Cheats))
	}
	if !mw.HasCheats() {
		t.Error("HasCheats() dovrebbe essere true")
	}
	// Verifica che gli ingredienti siano risolti.
	if mw.Cheats[0].Ingredient == nil || mw.Cheats[0].Ingredient.Name != "Hamburger" {
		t.Errorf("primo cheat: ingrediente non risolto: %+v", mw.Cheats[0])
	}

	// Reorder cheats: scambia c1 e c2.
	if err := plans.ReorderCheats(ctx, mealID, alice, []int64{c2, c1}); err != nil {
		t.Fatalf("ReorderCheats: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	got := full.Days[0][0].Cheats
	if got[0].ID != c2 || got[1].ID != c1 {
		t.Errorf("ordine reorder = [%d %d], atteso [%d %d]", got[0].ID, got[1].ID, c2, c1)
	}

	// Rimuovi un cheat (Bob non puo').
	if err := plans.RemoveCheat(ctx, c1, bob); !errors.Is(err, ErrCheatNotFound) {
		t.Errorf("Bob remove: atteso ErrCheatNotFound, got %v", err)
	}
	if err := plans.RemoveCheat(ctx, c1, alice); err != nil {
		t.Fatalf("RemoveCheat: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	if len(full.Days[0][0].Cheats) != 1 {
		t.Errorf("cheats dopo remove = %d, atteso 1", len(full.Days[0][0].Cheats))
	}

	// ClearCheats rimuove i restanti.
	if err := plans.ClearCheats(ctx, mealID, alice); err != nil {
		t.Fatalf("ClearCheats: %v", err)
	}
	full, _ = plans.LoadFull(ctx, plan)
	if full.Days[0][0].HasCheats() {
		t.Error("dopo ClearCheats il pasto non dovrebbe avere sgarri")
	}
}
