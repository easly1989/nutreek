package db

import (
	"context"
	"errors"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestIngredientsCreateAndGet(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	uid, err := users.Create(ctx, "creator@example.com", "h", "Creator")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	brand := "Molino X"
	id, err := ings.CreateLocal(ctx, "Farina di ceci", &brand, domain.UnitGram, 380, 22, 50, 6, &uid)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id atteso > 0, got %d", id)
	}

	got, err := ings.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Source != domain.IngredientSourceLocal {
		t.Errorf("source: got %q want local", got.Source)
	}
	if got.Unit != domain.UnitGram {
		t.Errorf("unit: got %q want g", got.Unit)
	}
	if got.Brand == nil || *got.Brand != "Molino X" {
		t.Errorf("brand: %v", got.Brand)
	}
	if got.CreatedBy == nil || *got.CreatedBy != uid {
		t.Errorf("created_by: %v", got.CreatedBy)
	}
	if got.SupersededBy != nil {
		t.Errorf("nuovo ingrediente non dovrebbe essere superseded")
	}
	if got.KcalPer100 != 380 {
		t.Errorf("kcal: %v", got.KcalPer100)
	}
}

func TestIngredientsGetNotFound(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	_, err := ings.GetByID(context.Background(), 9999)
	if !errors.Is(err, ErrIngredientNotFound) {
		t.Fatalf("atteso ErrIngredientNotFound, got %v", err)
	}
}

func TestIngredientsSearchAndList(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	mk := func(name string) int64 {
		id, err := ings.CreateLocal(ctx, name, nil, domain.UnitGram, 100, 5, 20, 1, nil)
		if err != nil {
			t.Fatalf("CreateLocal %q: %v", name, err)
		}
		return id
	}
	mk("Banana")
	mk("Pane integrale")
	mk("Pancetta affumicata")
	supersededID := mk("Pane bianco")

	// Marca un ingrediente come superseded: non deve apparire ne' in search
	// ne' in list.
	target := mk("Pane comune")
	if _, err := store.DB.ExecContext(ctx,
		`UPDATE ingredients SET superseded_by = ? WHERE id = ?`, target, supersededID); err != nil {
		t.Fatalf("set superseded_by: %v", err)
	}

	results, err := ings.SearchActive(ctx, "pan", 10)
	if err != nil {
		t.Fatalf("SearchActive: %v", err)
	}
	if len(results) != 3 { // Pane integrale, Pancetta affumicata, Pane comune
		t.Fatalf("attesi 3 risultati, got %d (%v)", len(results), names(results))
	}
	for _, r := range results {
		if r.SupersededBy != nil {
			t.Errorf("search ha incluso un superseded: %v", r.Name)
		}
	}

	all, err := ings.ListActive(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("attesi 4 attivi (Banana + 3 Pan*), got %d", len(all))
	}
	if all[0].Name != "Banana" {
		t.Errorf("ListActive non ordinato per nome: primo=%q", all[0].Name)
	}
}

func TestIngredientsCreateOFFAndDup(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	brand := "Consorzio"
	id, err := ings.CreateOFF(ctx, "8000000000017", "Grana Padano", &brand, domain.UnitGram, 384, 33, 0, 28)
	if err != nil {
		t.Fatalf("CreateOFF: %v", err)
	}
	got, err := ings.GetByBarcode(ctx, "8000000000017")
	if err != nil {
		t.Fatalf("GetByBarcode: %v", err)
	}
	if got.ID != id {
		t.Errorf("id mismatch: %d vs %d", got.ID, id)
	}
	if got.Source != domain.IngredientSourceOFF {
		t.Errorf("source: got %q want openfoodfacts", got.Source)
	}
	if got.LastVerifiedAt == nil {
		t.Errorf("last_verified_at non settato")
	}

	// Duplicate barcode -> ErrBarcodeAlreadyUsed
	_, err = ings.CreateOFF(ctx, "8000000000017", "Altro", nil, domain.UnitGram, 1, 1, 1, 1)
	if !errors.Is(err, ErrBarcodeAlreadyUsed) {
		t.Fatalf("atteso ErrBarcodeAlreadyUsed, got %v", err)
	}

	// GetByBarcode su barcode inesistente -> not found
	_, err = ings.GetByBarcode(ctx, "0000000000000")
	if !errors.Is(err, ErrIngredientNotFound) {
		t.Fatalf("atteso ErrIngredientNotFound, got %v", err)
	}
}

func names(in []*domain.Ingredient) []string {
	out := make([]string, len(in))
	for i, x := range in {
		out[i] = x.Name
	}
	return out
}
