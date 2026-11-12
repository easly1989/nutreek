package db

import (
	"context"
	"fmt"
	"testing"
)

func TestNormalizeOFFName(t *testing.T) {
	cases := map[string]string{
		"  Salmone   affumicato \n": "Salmone affumicato",
		"VELLUTATA DI ZUCCA, ":      "Vellutata di zucca",
		"spinaci con mozzarella.":   "Spinaci con mozzarella",
		"Yogurt greco 0% grassi":    "Yogurt greco 0% grassi",
		"UHT":                       "UHT", // fewer than 4 letters: left alone
	}
	for in, want := range cases {
		if got := NormalizeOFFName(in); got != want {
			t.Errorf("NormalizeOFFName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateOFFNormalisesNameAndBrand(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()
	brand := "  Conad , Altra "
	id, err := ings.CreateOFF(ctx, "8000000000099", "VERDURE GRIGLIATE SURGELATE ", &brand, "g", 50, 2, 5, 1, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ings.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Verdure grigliate surgelate" || got.Brand == nil || *got.Brand != "Conad" {
		t.Errorf("got name=%q brand=%v", got.Name, got.Brand)
	}
}

func TestListAllActiveBeyondOnePage(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()
	const n = 450
	for i := 0; i < n; i++ {
		code := fmt.Sprintf("9%04d", i)
		if _, err := ings.CreateOFF(ctx, code, "Item "+code, nil, "g", 1, 1, 1, 1, nil, nil, nil, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	all, err := ings.ListAllActive(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Fatalf("got %d ingredients, want %d", len(all), n)
	}
}
