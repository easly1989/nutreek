package db

import (
	"context"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// allergenIDByName cerca l'ID di un allergene seed per nome (i 14 tag sono
// inseriti dalla migrazione 0016).
func allergenIDByName(t *testing.T, cats *Categories, name string) int64 {
	t.Helper()
	tags, err := cats.ListAllergens(context.Background())
	if err != nil {
		t.Fatalf("ListAllergens: %v", err)
	}
	for _, tag := range tags {
		if tag.Name == name {
			return tag.ID
		}
	}
	t.Fatalf("allergene %q non trovato nei seed", name)
	return 0
}

func TestAllergensSeedAndSet(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	// I 14 allergeni standard devono essere già presenti.
	tags, err := cats.ListAllergens(ctx)
	if err != nil {
		t.Fatalf("ListAllergens: %v", err)
	}
	if len(tags) != 14 {
		t.Fatalf("attesi 14 allergeni seed, trovati %d", len(tags))
	}

	id, err := ings.CreateLocal(ctx, "Pane", nil, domain.UnitGram, 270, 9, 50, 3, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}

	glutine := allergenIDByName(t, cats, "glutine")
	soia := allergenIDByName(t, cats, "soia")

	if err := cats.SetIngredientAllergens(ctx, id, []int64{glutine, soia}); err != nil {
		t.Fatalf("SetIngredientAllergens: %v", err)
	}

	got, err := cats.GetIngredientAllergenIDs(ctx, id)
	if err != nil {
		t.Fatalf("GetIngredientAllergenIDs: %v", err)
	}
	if !got[glutine] || !got[soia] || len(got) != 2 {
		t.Fatalf("allergeni attesi {glutine, soia}, got %v", got)
	}

	// Replace: settare solo glutine deve rimuovere soia.
	if err := cats.SetIngredientAllergens(ctx, id, []int64{glutine}); err != nil {
		t.Fatalf("SetIngredientAllergens replace: %v", err)
	}
	got, _ = cats.GetIngredientAllergenIDs(ctx, id)
	if len(got) != 1 || !got[glutine] {
		t.Fatalf("dopo replace atteso solo {glutine}, got %v", got)
	}

	// Lista vuota azzera i tag.
	if err := cats.SetIngredientAllergens(ctx, id, nil); err != nil {
		t.Fatalf("SetIngredientAllergens clear: %v", err)
	}
	got, _ = cats.GetIngredientAllergenIDs(ctx, id)
	if len(got) != 0 {
		t.Fatalf("dopo clear attesi 0 tag, got %v", got)
	}
}

func TestAllergensForIngredientsBatch(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	a, _ := ings.CreateLocal(ctx, "Latte", nil, domain.UnitMilliliter, 64, 3, 5, 3, nil, nil, nil, nil, nil, nil, nil)
	b, _ := ings.CreateLocal(ctx, "Riso", nil, domain.UnitGram, 350, 7, 78, 1, nil, nil, nil, nil, nil, nil, nil)

	lattosio := allergenIDByName(t, cats, "lattosio")
	if err := cats.SetIngredientAllergens(ctx, a, []int64{lattosio}); err != nil {
		t.Fatalf("set: %v", err)
	}
	// b resta senza allergeni.

	byIng, err := cats.AllergensForIngredients(ctx, []int64{a, b})
	if err != nil {
		t.Fatalf("AllergensForIngredients: %v", err)
	}
	if len(byIng[a]) != 1 || byIng[a][0].Name != "lattosio" {
		t.Fatalf("ingrediente a: atteso [lattosio], got %v", byIng[a])
	}
	if _, ok := byIng[b]; ok {
		t.Fatalf("ingrediente b senza allergeni non deve comparire nella mappa")
	}

	// Slice vuota: nessuna query, mappa vuota.
	empty, err := cats.AllergensForIngredients(ctx, nil)
	if err != nil {
		t.Fatalf("AllergensForIngredients(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("attesa mappa vuota, got %v", empty)
	}
}

func TestIngredientIDsByAllergen(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	a, _ := ings.CreateLocal(ctx, "Gamberi", nil, domain.UnitGram, 99, 24, 0, 1, nil, nil, nil, nil, nil, nil, nil)
	b, _ := ings.CreateLocal(ctx, "Cozze", nil, domain.UnitGram, 86, 12, 4, 2, nil, nil, nil, nil, nil, nil, nil)
	c, _ := ings.CreateLocal(ctx, "Mela", nil, domain.UnitGram, 52, 0, 14, 0, nil, nil, nil, nil, nil, nil, nil)

	crostacei := allergenIDByName(t, cats, "crostacei")
	_ = cats.SetIngredientAllergens(ctx, a, []int64{crostacei})
	_ = cats.SetIngredientAllergens(ctx, b, []int64{crostacei})
	// c senza allergeni.

	ids, err := cats.IngredientIDsByAllergen(ctx, crostacei)
	if err != nil {
		t.Fatalf("IngredientIDsByAllergen: %v", err)
	}
	if !ids[a] || !ids[b] || ids[c] || len(ids) != 2 {
		t.Fatalf("attesi {a,b}, got %v", ids)
	}
}
