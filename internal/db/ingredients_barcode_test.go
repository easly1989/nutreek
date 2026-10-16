package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// An ingredient created by hand from the scanner keeps its code: the next
// scan finds it, and the reconciler leaves it alone (it is local on purpose,
// because the product was not on OpenFoodFacts).
func TestIngredientsLocalBarcode(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	id, err := ings.CreateLocal(ctx, "Biscotti del forno", nil, domain.UnitGram, 450, 7, 70, 15, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if err := ings.SetLocalBarcode(ctx, id, "4006381333931"); err != nil {
		t.Fatalf("SetLocalBarcode: %v", err)
	}
	got, err := ings.GetByBarcode(ctx, "4006381333931")
	if err != nil || got.ID != id || got.Source != domain.IngredientSourceLocal {
		t.Fatalf("GetByBarcode: %+v, %v", got, err)
	}

	// The same code cannot end up on two ingredients.
	other, _ := ings.CreateLocal(ctx, "Altro", nil, domain.UnitGram, 100, 1, 1, 1, nil, nil, nil, nil, nil, nil, nil)
	if err := ings.SetLocalBarcode(ctx, other, "4006381333931"); !errors.Is(err, ErrBarcodeAlreadyUsed) {
		t.Errorf("duplicate code: want ErrBarcodeAlreadyUsed, got %v", err)
	}

	// Locals only: an OFF import already has its code.
	offID, _ := ings.CreateOFF(ctx, "96385074", "Latte", nil, domain.UnitMilliliter, 64, 3, 5, 3.6, nil, nil, nil, nil, nil, nil, nil)
	if err := ings.SetLocalBarcode(ctx, offID, "036000291452"); !errors.Is(err, ErrIngredientNotFound) {
		t.Errorf("on an OFF row: want ErrIngredientNotFound, got %v", err)
	}

	locals, err := ings.LocalsToReconcile(ctx, 50, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("LocalsToReconcile: %v", err)
	}
	for _, l := range locals {
		if l.ID == id {
			t.Errorf("a local with a code must not be reconciled")
		}
	}
	if len(locals) != 1 || locals[0].ID != other {
		t.Errorf("to reconcile: want only %d, got %d items", other, len(locals))
	}
}
