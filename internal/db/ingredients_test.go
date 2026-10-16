package db

import (
	"context"
	"errors"
	"testing"
	"time"

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
	id, err := ings.CreateLocal(ctx, "Farina di ceci", &brand, domain.UnitGram, 380, 22, 50, 6, &uid, nil, nil, nil, nil, nil, nil)
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
		id, err := ings.CreateLocal(ctx, name, nil, domain.UnitGram, 100, 5, 20, 1, nil, nil, nil, nil, nil, nil, nil)
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
	pack, serving := 500.0, 30.0
	id, err := ings.CreateOFF(ctx, "8000000000017", "Grana Padano", &brand, domain.UnitGram, 384, 33, 0, 28, nil, &pack, &serving, nil, nil, nil, nil)
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
	// La porzione OFF diventa la quantita' proposta nel picker (#136).
	if got.DefaultQuantity == nil || *got.DefaultQuantity != 30 {
		t.Errorf("default_quantity: got %v want 30", got.DefaultQuantity)
	}
	if got.UnitWeightG == nil || *got.UnitWeightG != 500 {
		t.Errorf("unit_weight_g: got %v want 500", got.UnitWeightG)
	}
	if got.Source != domain.IngredientSourceOFF {
		t.Errorf("source: got %q want openfoodfacts", got.Source)
	}
	if got.LastVerifiedAt == nil {
		t.Errorf("last_verified_at non settato")
	}

	// Duplicate barcode -> ErrBarcodeAlreadyUsed
	_, err = ings.CreateOFF(ctx, "8000000000017", "Altro", nil, domain.UnitGram, 1, 1, 1, 1, nil, nil, nil, nil, nil, nil, nil)
	if !errors.Is(err, ErrBarcodeAlreadyUsed) {
		t.Fatalf("atteso ErrBarcodeAlreadyUsed, got %v", err)
	}

	// GetByBarcode su barcode inesistente -> not found
	_, err = ings.GetByBarcode(ctx, "0000000000000")
	if !errors.Is(err, ErrIngredientNotFound) {
		t.Fatalf("atteso ErrIngredientNotFound, got %v", err)
	}
}

func TestIngredientsMicronutrienti(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	fiber := 3.2
	sugars := 1.1
	satFat := 0.8
	sodium := 0.4

	id, err := ings.CreateLocal(ctx, "Avena", nil, domain.UnitGram, 389, 17, 66, 7, nil, nil, nil, &fiber, &sugars, &satFat, &sodium)
	if err != nil {
		t.Fatalf("CreateLocal con micronutrienti: %v", err)
	}
	got, err := ings.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.FiberPer100 == nil || *got.FiberPer100 != fiber {
		t.Errorf("FiberPer100: %v", got.FiberPer100)
	}
	if got.SugarsPer100 == nil || *got.SugarsPer100 != sugars {
		t.Errorf("SugarsPer100: %v", got.SugarsPer100)
	}
	if got.SaturatedFatPer100 == nil || *got.SaturatedFatPer100 != satFat {
		t.Errorf("SaturatedFatPer100: %v", got.SaturatedFatPer100)
	}
	if got.SodiumPer100 == nil || *got.SodiumPer100 != sodium {
		t.Errorf("SodiumPer100: %v", got.SodiumPer100)
	}

	// Update azzera i micronutrienti (nil = NULL).
	if err := ings.Update(ctx, id, "Avena aggiornata", nil, domain.UnitGram, 389, 17, 66, 7, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = ings.GetByID(ctx, id)
	if got.FiberPer100 != nil {
		t.Errorf("FiberPer100 dovrebbe essere nil dopo update a nil, got %v", *got.FiberPer100)
	}
}

func TestIngredientsReconcileHelpers(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	// 3 locals: A, B, C. Tutti senza last_verified_at.
	idA, _ := ings.CreateLocal(ctx, "A", nil, "g", 100, 5, 10, 1, nil, nil, nil, nil, nil, nil, nil)
	idB, _ := ings.CreateLocal(ctx, "B", nil, "g", 100, 5, 10, 1, nil, nil, nil, nil, nil, nil, nil)
	idC, _ := ings.CreateLocal(ctx, "C", nil, "g", 100, 5, 10, 1, nil, nil, nil, nil, nil, nil, nil)
	// Un OFF (non deve apparire fra i candidati).
	idOFF, err := ings.CreateOFF(ctx, "BARCODE1", "OFF", nil, "g", 100, 5, 10, 1, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Touch B come "verificato di recente" (ora).
	if err := ings.TouchVerified(ctx, idB); err != nil {
		t.Fatal(err)
	}

	// Ora: locals non verificati o verificati prima di "now-1ms" → A e C
	// (B e' stato touchato a CURRENT_TIMESTAMP, quindi >= 1ms fa non e' vero).
	// Per essere sicuri uso una soglia futura: quelli con last_verified_at
	// PRIMA di "now+1h" sono tutti, ma escludo B che e' "appena ora".
	// Useremo invece "1 minuto fa" come threshold: A e C (NULL) appaiono, B no.
	sinceAgo := time.Now().Add(-1 * time.Minute)
	candidates, err := ings.LocalsToReconcile(ctx, 10, sinceAgo)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := map[int64]bool{}
	for _, c := range candidates {
		gotIDs[c.ID] = true
	}
	if !gotIDs[idA] || !gotIDs[idC] {
		t.Errorf("A e C dovrebbero essere candidati, got %v", gotIDs)
	}
	if gotIDs[idB] {
		t.Errorf("B appena verificato non dovrebbe essere candidato")
	}
	if gotIDs[idOFF] {
		t.Errorf("OFF non deve mai essere candidato (source != local)")
	}

	// MarkSuperseded: A diventa superseded da OFF.
	if err := ings.MarkSuperseded(ctx, idA, idOFF); err != nil {
		t.Fatal(err)
	}
	got2, _ := ings.GetByID(ctx, idA)
	if got2.SupersededBy == nil || *got2.SupersededBy != idOFF {
		t.Errorf("A non superseded da OFF: %v", got2.SupersededBy)
	}
	if got2.LastVerifiedAt == nil {
		t.Errorf("A last_verified_at non aggiornato")
	}

	// MarkSuperseded su un OFF deve fallire (no rows affected).
	if err := ings.MarkSuperseded(ctx, idOFF, idA); !errors.Is(err, ErrIngredientNotFound) {
		t.Errorf("MarkSuperseded su OFF dovrebbe ErrIngredientNotFound, got %v", err)
	}

	// Adesso A e' superseded → non e' piu' candidato.
	candidates, _ = ings.LocalsToReconcile(ctx, 10, sinceAgo)
	for _, c := range candidates {
		if c.ID == idA {
			t.Errorf("A superseded non dovrebbe ricomparire fra i candidati")
		}
	}
}

func names(in []*domain.Ingredient) []string {
	out := make([]string, len(in))
	for i, x := range in {
		out[i] = x.Name
	}
	return out
}

// Un prodotto OFF importato da un utente ha lui come autore (#143); il
// reconciler passa nil.
func TestCreateOFFCreatedBy(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	id, err := ings.CreateOFF(ctx, "8000000000024", "Ceci", nil, domain.UnitGram, 120, 7, 16, 2, &alice, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ings.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedBy == nil || *got.CreatedBy != alice {
		t.Errorf("CreatedBy = %v, atteso %d", got.CreatedBy, alice)
	}
}
