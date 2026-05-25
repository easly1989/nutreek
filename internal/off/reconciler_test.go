package off

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// newReconcilerFixture monta un fake OFF + DB sqlite temporaneo + repo, e
// ritorna il Reconciler pronto + handle al DB.
type fixture struct {
	srv  *httptest.Server
	store *db.Store
	ings *db.Ingredients
	rec  *Reconciler
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := strings.ToLower(r.URL.Query().Get("search_terms"))
		switch {
		case strings.Contains(q, "grana"):
			_, _ = w.Write([]byte(`{
				"products": [{
					"code": "8000000000017",
					"product_name": "Grana Padano DOP",
					"brands": "Consorzio Test",
					"nutriments": {
						"energy-kcal_100g": 384, "proteins_100g": 33,
						"carbohydrates_100g": 0, "fat_100g": 28
					}
				}]
			}`))
		case strings.Contains(q, "pane"):
			// Match con macro INCOMPLETE → deve essere scartato.
			_, _ = w.Write([]byte(`{
				"products": [{
					"code": "BARCODE_PANE",
					"product_name": "Pane integrale",
					"brands": "",
					"nutriments": { "proteins_100g": 8 }
				}]
			}`))
		default:
			_, _ = w.Write([]byte(`{"products": []}`))
		}
	}))
	t.Cleanup(srv.Close)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ings := db.NewIngredients(store.DB)
	client := NewClient(srv.URL, nil)
	// Il server fittizio fa anche da search backend.
	client.SearchBaseURL = srv.URL
	rec := &Reconciler{
		Ingredients: ings,
		Client:      client,
		MaxBatch:    20,
		MinAge:      1 * time.Hour, // basso per il test
	}
	return &fixture{srv: srv, store: store, ings: ings, rec: rec}
}

func TestReconcilerMatchAndNoMatch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// 3 locals: "Grana" → match, "Banana" → no result OFF, "Pane" → result
	// con macro incompleti (deve restare no-match), "Latte" → no result.
	idGrana, _ := f.ings.CreateLocal(ctx, "Grana Padano locale", nil, domain.UnitGram, 380, 32, 0, 28, nil)
	idBanana, _ := f.ings.CreateLocal(ctx, "Banana", nil, domain.UnitGram, 89, 1.1, 23, 0.3, nil)
	idPane, _ := f.ings.CreateLocal(ctx, "Pane casereccio", nil, domain.UnitGram, 250, 8, 50, 1, nil)

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
	id1, _ := f.ings.CreateLocal(ctx, "Grana Padano A", nil, domain.UnitGram, 380, 32, 0, 28, nil)
	id2, _ := f.ings.CreateLocal(ctx, "Grana Padano B", nil, domain.UnitGram, 380, 32, 0, 28, nil)

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
