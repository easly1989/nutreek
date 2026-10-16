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

// «Crea l'ingrediente» dallo scanner: il codice letto arriva nel form e
// resta collegato al nuovo ingrediente, cosi' la scansione successiva lo trova.
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
		t.Fatalf("creazione: status %d, body %s", w.Code, w.Body.String())
	}
	ing, err := a.Ingredients.GetByBarcode(ctx, "4006381333931")
	if err != nil || ing.Name != "Biscotti del forno" || ing.Source != domain.IngredientSourceLocal {
		t.Fatalf("ingrediente per codice: %+v, %v", ing, err)
	}

	// Lo stesso codice una seconda volta: errore nel form, niente doppione.
	w := post("4006381333931")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Biscotti del forno") {
		t.Errorf("codice gia' usato: status %d, body %s", w.Code, w.Body.String())
	}
	if n := countIngredients(t, a); n != 1 {
		t.Errorf("ingredienti: %d, atteso 1", n)
	}

	// Un codice non valido non blocca nulla: si crea senza codice.
	if w := post("123"); w.Code != http.StatusNoContent {
		t.Errorf("codice non valido: status %d", w.Code)
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
