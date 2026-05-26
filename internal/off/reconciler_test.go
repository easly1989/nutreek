package off

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// fixture monta DB sqlite temporaneo con off_products seedata + repo +
// Reconciler. SearchByName ora interroga FTS5 locale, niente piu' mock HTTP.
type fixture struct {
	store *db.Store
	ings  *db.Ingredients
	rec   *Reconciler
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Seed: prodotti OFF "locali" disponibili al reconciler via FTS5.
	// - Grana Padano con macro completi → match per il local "Grana Padano locale".
	// - Pane integrale con macro INCOMPLETI → reconciler lo scarta.
	seedOFFProducts(t, store.DB, []Product{
		{Code: "8000000000017", Name: "Grana Padano DOP", Brand: "Consorzio Test", Kcal: 384, Protein: 33, Carbs: 0, Fat: 28, HasFullNutrients: true},
		{Code: "BARCODE_PANE", Name: "Pane integrale", Brand: "", Kcal: 0, Protein: 8, Carbs: 0, Fat: 0, HasFullNutrients: false},
	})

	ings := db.NewIngredients(store.DB)
	client := NewClient(store.DB, "", nil)
	rec := &Reconciler{
		Ingredients: ings,
		Client:      client,
		MaxBatch:    20,
		MinAge:      1 * time.Hour, // basso per il test
	}
	return &fixture{store: store, ings: ings, rec: rec}
}

func TestReconcilerMatchAndNoMatch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// 3 locals: "Grana" → match, "Banana" → no result OFF, "Pane" → result
	// con macro incompleti (deve restare no-match).
	idGrana, _ := f.ings.CreateLocal(ctx, "Grana Padano locale", nil, domain.UnitGram, 380, 32, 0, 28, nil, nil)
	idBanana, _ := f.ings.CreateLocal(ctx, "Banana", nil, domain.UnitGram, 89, 1.1, 23, 0.3, nil, nil)
	idPane, _ := f.ings.CreateLocal(ctx, "Pane casereccio", nil, domain.UnitGram, 250, 8, 50, 1, nil, nil)

	stats, err := f.rec.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Considered != 3 {
		t.Errorf("considered: got %d want 3", stats.Considered)
	}
	if stats.Matched != 1 {
		t.Errorf("matched: got %d want 1 (solo Grana)", stats.Matched)
	}
	if stats.NoMatch != 2 {
		t.Errorf("no_match: got %d want 2 (Banana + Pane scartato per macro incompleti)", stats.NoMatch)
	}
	if stats.Errors != 0 {
		t.Errorf("errors: got %d want 0", stats.Errors)
	}

	// Grana superseded da un nuovo OFF.
	got, err := f.ings.GetByID(ctx, idGrana)
	if err != nil {
		t.Fatal(err)
	}
	if got.SupersededBy == nil {
		t.Errorf("Grana non e' superseded")
	} else {
		off, _ := f.ings.GetByID(ctx, *got.SupersededBy)
		if off.Source != domain.IngredientSourceOFF {
			t.Errorf("la superseder non e' OFF: %v", off.Source)
		}
		if off.OFFBarcode == nil || *off.OFFBarcode != "8000000000017" {
			t.Errorf("barcode OFF inatteso: %v", off.OFFBarcode)
		}
	}

	// Banana e Pane non superseded ma touchati.
	for _, id := range []int64{idBanana, idPane} {
		ing, _ := f.ings.GetByID(ctx, id)
		if ing.SupersededBy != nil {
			t.Errorf("ing %d non doveva essere superseded", id)
		}
		if ing.LastVerifiedAt == nil {
			t.Errorf("ing %d last_verified_at non touchato", id)
		}
	}

	// Una seconda RunOnce subito dopo: nessun candidato (tutti hanno
	// last_verified_at appena settato, MinAge=1h).
	stats2, err := f.rec.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats2.Considered != 0 {
		t.Errorf("seconda esecuzione: considered %d, atteso 0", stats2.Considered)
	}
}

func TestReconcilerIdempotentTwoLocalsSameOFF(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Due locals con lo stesso nome → devono entrambi finire superseded
	// allo stesso OFF (idempotente sul barcode).
	id1, _ := f.ings.CreateLocal(ctx, "Grana Padano A", nil, domain.UnitGram, 380, 32, 0, 28, nil, nil)
	id2, _ := f.ings.CreateLocal(ctx, "Grana Padano B", nil, domain.UnitGram, 380, 32, 0, 28, nil, nil)

	stats, err := f.rec.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Matched != 2 {
		t.Errorf("matched: got %d want 2", stats.Matched)
	}
	g1, _ := f.ings.GetByID(ctx, id1)
	g2, _ := f.ings.GetByID(ctx, id2)
	if g1.SupersededBy == nil || g2.SupersededBy == nil || *g1.SupersededBy != *g2.SupersededBy {
		t.Errorf("entrambi i local devono puntare allo stesso OFF: %v vs %v", g1.SupersededBy, g2.SupersededBy)
	}
}
