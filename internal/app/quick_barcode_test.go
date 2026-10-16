package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// «Crea l'ingrediente» from the scanner: the scanned code reaches the form and
// stays linked to the new ingredient, so the next scan finds it.
func TestQuickIngredientWithBarcode(t *testing.T) {
	a := newTestApp(t)
	u := mustCreateTestUser(t, a, "scan@example.com", "password123")
	ctx := context.Background()

	post := func(barcode string) *httptest.ResponseRecorder {
		form := url.Values{"name": {"Biscotti del forno"}, "unit": {"g"}, "kcal": {"450"},
			"protein": {"7"}, "carbs": {"70"}, "fat": {"15"}, "barcode": {barcode}}
		r := httptest.NewRequest(http.MethodPost, "/quick/ingredient", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		a.handleQuickIngredientCreate(w, requestWithUser(r, u))
		return w
	}

	if w := post("4 006381 333931"); w.Code != http.StatusNoContent {
		t.Fatalf("creation: status %d, body %s", w.Code, w.Body.String())
	}
	ing, err := a.Ingredients.GetByBarcode(ctx, "4006381333931")
	if err != nil || ing.Name != "Biscotti del forno" || ing.Source != domain.IngredientSourceLocal {
		t.Fatalf("ingredient by code: %+v, %v", ing, err)
	}

	// The same code a second time: an error in the form, no duplicate.
	w := post("4006381333931")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Biscotti del forno") {
		t.Errorf("code already used: status %d, body %s", w.Code, w.Body.String())
	}
	if n := countIngredients(t, a); n != 1 {
		t.Errorf("ingredients: %d, want 1", n)
	}

	// An invalid code blocks nothing: the ingredient is created without a code.
	if w := post("123"); w.Code != http.StatusNoContent {
		t.Errorf("invalid code: status %d", w.Code)
	}
}

func countIngredients(t *testing.T, a *App) int {
	t.Helper()
	var n int
	if err := a.Store.DB.QueryRow(`SELECT count(*) FROM ingredients`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
