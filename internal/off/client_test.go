package off

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSearchByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi/search.pl" {
			t.Errorf("path inatteso: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("search_terms"); got != "grana padano" {
			t.Errorf("search_terms = %q", got)
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "Nutreek") {
			t.Errorf("UA non impostato: %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"count": 2,
			"page": 1,
			"products": [
				{
					"code": "8000000000017",
					"product_name": "Grana Padano DOP",
					"brands": "Consorzio Tutela, Generic",
					"quantity": "200 g",
					"nutriments": {
						"energy-kcal_100g": 384,
						"proteins_100g": 33,
						"carbohydrates_100g": 0,
						"fat_100g": 28
					}
				},
				{
					"code": "",
					"product_name": "skip me — codice vuoto",
					"nutriments": {}
				},
				{
					"code": "8000000000024",
					"product_name": "Grana Locale",
					"brands": "Brand X",
					"nutriments": {
						"proteins_100g": "32.5",
						"carbohydrates_100g": 0.5,
						"fat_100g": 27
					}
				}
			]
		}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, nil)
	got, err := c.SearchByName(context.Background(), "grana padano")
	if err != nil {
		t.Fatalf("SearchByName: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("attesi 2 prodotti (uno scartato per code vuoto), got %d", len(got))
	}
	first := got[0]
	if first.Code != "8000000000017" || first.Name != "Grana Padano DOP" {
		t.Errorf("first prodotto wrong: %+v", first)
	}
	if first.Brand != "Consorzio Tutela" {
		t.Errorf("brand: vogliamo solo il primo del CSV, got %q", first.Brand)
	}
	if !first.HasFullNutrients {
		t.Errorf("nutrienti dovrebbero essere completi")
	}
	if first.Kcal != 384 {
		t.Errorf("kcal: %v", first.Kcal)
	}
	// Secondo: stringa "32.5" deve parsare; manca kcal -> incompleto.
	second := got[1]
	if second.HasFullNutrients {
		t.Errorf("secondo prodotto manca kcal -> non dovrebbe essere completo")
	}
	if second.Protein != 32.5 {
		t.Errorf("protein da stringa non parsato: %v", second.Protein)
	}
}

func TestClientGetByBarcode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v2/product/") {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// energy_100g in kJ: il client dovrebbe convertire a kcal.
		_, _ = w.Write([]byte(`{
			"status": 1,
			"code": "1234567890",
			"product": {
				"code": "1234567890",
				"product_name": "Latte UHT",
				"brands": "Centrale",
				"nutriments": {
					"energy_100g": 200,
					"proteins_100g": 3.3,
					"carbohydrates_100g": 4.8,
					"fat_100g": 1.5
				}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, nil)
	p, err := c.GetByBarcode(context.Background(), "1234567890")
	if err != nil {
		t.Fatalf("GetByBarcode: %v", err)
	}
	if p.Code != "1234567890" || p.Name != "Latte UHT" {
		t.Errorf("product: %+v", p)
	}
	expectedKcal := 200.0 / 4.184
	if diff := p.Kcal - expectedKcal; diff > 0.01 || diff < -0.01 {
		t.Errorf("kcal da kJ: %v (expected ~%v)", p.Kcal, expectedKcal)
	}
	if !p.HasFullNutrients {
		t.Errorf("nutrienti dovrebbero risultare completi (kcal derivata da kJ)")
	}
}

func TestClientGetByBarcodeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": 0, "code": "9999"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, nil)
	_, err := c.GetByBarcode(context.Background(), "9999")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("atteso ErrNotFound, got %v", err)
	}
}

func TestClientHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, nil)
	_, err := c.SearchByName(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("atteso errore con 500, got %v", err)
	}
}
