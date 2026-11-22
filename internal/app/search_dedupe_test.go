package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

// A product the user already has as an ingredient shows once, as the
// ingredient, even when that ingredient is not among the first local rows (#225).
func TestSearchShowsAnImportedProductOnce(t *testing.T) {
	a := newTestApp(t)
	a.OFF = off.NewClient(a.Store.DB, "http://127.0.0.1:1", nil)
	user := mustCreateTestUser(t, a, "una@esempio.test", "password-demo")
	ctx := context.Background()

	// 25 ingredients match "zeta"; the one imported from the product sorts last.
	for i := 0; i < 25; i++ {
		if _, err := a.Ingredients.CreateLocal(ctx, fmt.Sprintf("Zeta %02d", i), nil, domain.UnitGram, 1, 1, 1, 1, &user.ID, nil, nil, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Ingredients.CreateOFF(ctx, "8000000000041", "Zeta zz importato", nil, domain.UnitGram, 100, 5, 20, 1, &user.ID, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.DB.Exec(`INSERT INTO off_products (barcode, product_name, brands, quantity,
		kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, updated_at) VALUES
		('8000000000041', 'Zeta zz importato', '', '', 100, 5, 20, 1, 1, 0),
		('8000000000042', 'Zeta altro prodotto', '', '', 90, 4, 18, 2, 1, 0)`); err != nil {
		t.Fatal(err)
	}

	rows, _, err := a.searchMixed(contextWithUser(ctx, user), "zeta", true)
	if err != nil {
		t.Fatal(err)
	}
	var offs []string
	for _, r := range rows {
		if r.IsOFF() {
			offs = append(offs, r.OFF.Code)
		}
	}
	if len(offs) != 1 || offs[0] != "8000000000042" {
		t.Errorf("products shown from OpenFoodFacts: %v, want only the one not imported yet", offs)
	}

	// The ingredient is beyond the 20 local rows, yet it is shown once as the
	// ingredient and not as the product: it is not lost.
	if n := countRows(rows, "Zeta zz importato", "8000000000041"); n != 1 {
		t.Errorf("the imported ingredient and its product show %d times, want 1", n)
	}
	// Renamed so that it no longer matches the query: the product still finds it.
	var id int64
	if err := a.Store.DB.QueryRow(`SELECT id FROM ingredients WHERE off_barcode = '8000000000041'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := a.Ingredients.Update(ctx, id, "Altro nome", nil, domain.UnitGram, 100, 5, 20, 1, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	rows, _, err = a.searchMixed(contextWithUser(ctx, user), "zeta", true)
	if err != nil {
		t.Fatal(err)
	}
	if n := countRows(rows, "Altro nome", "8000000000041"); n != 1 {
		t.Errorf("a renamed import shows %d times, want 1 (as the ingredient)", n)
	}
	for _, r := range rows {
		if r.IsOFF() && r.OFF.Code == "8000000000041" {
			t.Error("the product of an ingredient the viewer has is offered to import")
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ingredients/search?q=zeta&off=1", nil)
	a.handleIngredientsSearchPartial(rec, requestWithUser(req, user))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Altro nome") {
		t.Errorf("the page does not show the renamed import: code %d", rec.Code)
	}
}

// countRows counts how many rows stand for an ingredient: the local row with the
// name, or the OpenFoodFacts row with the barcode.
func countRows(rows []ingredientRow, name, code string) int {
	n := 0
	for _, r := range rows {
		if (r.Local != nil && r.Local.Name == name) || (r.IsOFF() && r.OFF.Code == code) {
			n++
		}
	}
	return n
}
