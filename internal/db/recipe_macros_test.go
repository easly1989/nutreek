package db

import (
	"context"
	"testing"
)

// L'elenco ricette mostra kcal e macro per porzione di ogni ricetta (#122):
// PerServingByRecipe li calcola in un colpo per tutte le ricette visibili.
func TestPerServingByRecipe(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	recs := NewRecipes(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 350, 12, 72, 1.5, &alice, nil, nil, nil, nil, nil, nil)
	olio, _ := ings.CreateLocal(ctx, "Olio", nil, "ml", 900, 0, 0, 100, &alice, nil, nil, nil, nil, nil, nil)

	// 2 porzioni: 200 g pasta + 20 ml olio = 700+180 kcal → 440 per porzione.
	mia, _ := recs.Create(ctx, alice, "Pasta all'olio", nil, 2, false)
	if _, err := recs.AddItem(ctx, mia, alice, pasta, 200); err != nil {
		t.Fatal(err)
	}
	if _, err := recs.AddItem(ctx, mia, alice, olio, 20); err != nil {
		t.Fatal(err)
	}
	vuota, _ := recs.Create(ctx, alice, "Vuota", nil, 1, false)
	pubblica, _ := recs.Create(ctx, bob, "Pasta di Bob", nil, 1, true)
	if _, err := recs.AddItem(ctx, pubblica, bob, pasta, 100); err != nil {
		t.Fatal(err)
	}
	privata, _ := recs.Create(ctx, bob, "Segreta", nil, 1, false)
	if _, err := recs.AddItem(ctx, privata, bob, pasta, 100); err != nil {
		t.Fatal(err)
	}

	got, err := recs.PerServingByRecipe(ctx, alice)
	if err != nil {
		t.Fatalf("PerServingByRecipe: %v", err)
	}
	m := got[mia]
	if !approxEqual(m.Kcal, 440) || !approxEqual(m.Protein, 12) || !approxEqual(m.Carbs, 72) || !approxEqual(m.Fat, 11.5) {
		t.Errorf("per porzione = %+v, atteso 440 kcal, P 12, C 72, G 11,5", m)
	}
	if !got[vuota].IsZero() {
		t.Errorf("ricetta senza ingredienti = %+v, atteso zero", got[vuota])
	}
	if !approxEqual(got[pubblica].Kcal, 350) {
		t.Errorf("ricetta pubblica di altri = %.2f kcal, atteso 350", got[pubblica].Kcal)
	}
	if _, ok := got[privata]; ok {
		t.Error("le ricette private di altri non devono comparire")
	}
}
