package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// Un ingrediente creato a mano dallo scanner tiene il codice: lo scanner
// successivo lo trova, e il reconciler non lo tocca (e' locale apposta,
// perche' su OpenFoodFacts il prodotto non c'era).
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

	// Lo stesso codice non puo' finire su due ingredienti.
	other, _ := ings.CreateLocal(ctx, "Altro", nil, domain.UnitGram, 100, 1, 1, 1, nil, nil, nil, nil, nil, nil, nil)
	if err := ings.SetLocalBarcode(ctx, other, "4006381333931"); !errors.Is(err, ErrBarcodeAlreadyUsed) {
		t.Errorf("codice doppio: atteso ErrBarcodeAlreadyUsed, got %v", err)
	}

	// Solo per i locali: un import OFF ha gia' il suo codice.
	offID, _ := ings.CreateOFF(ctx, "96385074", "Latte", nil, domain.UnitMilliliter, 64, 3, 5, 3.6, nil, nil, nil, nil, nil, nil)
	if err := ings.SetLocalBarcode(ctx, offID, "036000291452"); !errors.Is(err, ErrIngredientNotFound) {
		t.Errorf("su un OFF: atteso ErrIngredientNotFound, got %v", err)
	}

	locals, err := ings.LocalsToReconcile(ctx, 50, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("LocalsToReconcile: %v", err)
	}
	for _, l := range locals {
		if l.ID == id {
			t.Errorf("un locale con codice non va riconciliato")
		}
	}
	if len(locals) != 1 || locals[0].ID != other {
		t.Errorf("da riconciliare: atteso solo %d, got %d voci", other, len(locals))
	}
}
