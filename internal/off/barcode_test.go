package off

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeBarcode(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"4006381333931", "4006381333931", true},     // EAN-13
		{" 4 006381 333931 ", "4006381333931", true}, // come stampato sotto le barre
		{"96385074", "96385074", true},               // EAN-8
		{"036000291452", "0036000291452", true},      // UPC-A: OFF lo salva a 13 cifre
		{"4006381333932", "", false},                 // cifra di controllo sbagliata
		{"400638133393", "", false},                  // 12 cifre ma non un UPC valido
		{"4006381333", "", false},                    // lunghezza non prevista
		{"40063813339a", "", false},
		{"grana", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeBarcode(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NormalizeBarcode(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// liveServer risponde come l'API prodotto di OFF e conta le chiamate.
func liveServer(t *testing.T, body string) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

const liveAvena = `{"status": 1, "product": {"code": "4006381333931", "product_name": "Fiocchi d'avena",
	"brands": "Molino", "nutriments": {"energy-kcal_100g": 372, "proteins_100g": 13.5,
	"carbohydrates_100g": 58.7, "fat_100g": 7}}}`

func TestFindByBarcodeLocalFirst(t *testing.T) {
	sqldb := openTestDB(t)
	seedOFFProducts(t, sqldb, []Product{
		{Code: "4006381333931", Name: "Fiocchi d'avena", Kcal: 372, Protein: 13.5, Carbs: 58.7, Fat: 7, HasFullNutrients: true},
	})
	srv, calls := liveServer(t, liveAvena)
	c := NewClient(sqldb, srv.URL, nil)

	p, err := c.FindByBarcode(context.Background(), "4006381333931", true)
	if err != nil {
		t.Fatalf("FindByBarcode: %v", err)
	}
	if p.Name != "Fiocchi d'avena" || !p.HasFullNutrients {
		t.Errorf("prodotto: %+v", p)
	}
	if *calls != 0 {
		t.Errorf("con la copia locale completa non serve la rete: %d chiamate", *calls)
	}
}

func TestFindByBarcodeIncompleteLocalGoesLive(t *testing.T) {
	sqldb := openTestDB(t)
	seedOFFProducts(t, sqldb, []Product{{Code: "4006381333931", Name: "Fiocchi d'avena"}})
	srv, calls := liveServer(t, liveAvena)
	c := NewClient(sqldb, srv.URL, nil)

	p, err := c.FindByBarcode(context.Background(), "4006381333931", true)
	if err != nil {
		t.Fatalf("FindByBarcode: %v", err)
	}
	if !p.HasFullNutrients || p.Kcal != 372 || *calls != 1 {
		t.Errorf("atteso il prodotto live completo, got %+v (%d chiamate)", p, *calls)
	}

	// Senza rete resta la copia locale, anche se incompleta.
	p, err = c.FindByBarcode(context.Background(), "4006381333931", false)
	if err != nil || p.HasFullNutrients || *calls != 1 {
		t.Errorf("senza live: %+v, %v (%d chiamate)", p, err, *calls)
	}
}

func TestFindByBarcodeNotFound(t *testing.T) {
	sqldb := openTestDB(t)
	srv, calls := liveServer(t, `{"status": 0}`)
	c := NewClient(sqldb, srv.URL, nil)

	if _, err := c.FindByBarcode(context.Background(), "96385074", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("senza live: atteso ErrNotFound, got %v", err)
	}
	if *calls != 0 {
		t.Errorf("live=false non deve chiamare OFF")
	}
	if _, err := c.FindByBarcode(context.Background(), "96385074", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("live: atteso ErrNotFound, got %v", err)
	}
}

// Un guasto di OFF e' ErrUnavailable (lo scanner dice «riprova»), distinto
// da «non trovato».
func TestFindByBarcodeUnavailable(t *testing.T) {
	sqldb := openTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(sqldb, srv.URL, nil)

	_, err := c.FindByBarcode(context.Background(), "96385074", true)
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotFound) {
		t.Errorf("atteso ErrUnavailable, got %v", err)
	}
}
