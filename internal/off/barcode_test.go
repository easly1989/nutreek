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
		{" 4 006381 333931 ", "4006381333931", true}, // as printed under the bars
		{"96385074", "96385074", true},               // EAN-8
		{"036000291452", "0036000291452", true},      // UPC-A: OFF stores it with 13 digits
		{"4006381333932", "", false},                 // wrong check digit
		{"400638133393", "", false},                  // 12 digits but not a valid UPC
		{"4006381333", "", false},                    // unexpected length
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

// liveServer answers like OFF's product API and counts the calls.
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
		t.Errorf("with a complete local copy the network is not needed: %d calls", *calls)
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
		t.Errorf("want the complete live product, got %+v (%d calls)", p, *calls)
	}

	// Without the network the local copy remains, even when incomplete.
	p, err = c.FindByBarcode(context.Background(), "4006381333931", false)
	if err != nil || p.HasFullNutrients || *calls != 1 {
		t.Errorf("without live: %+v, %v (%d calls)", p, err, *calls)
	}
}

func TestFindByBarcodeNotFound(t *testing.T) {
	sqldb := openTestDB(t)
	srv, calls := liveServer(t, `{"status": 0}`)
	c := NewClient(sqldb, srv.URL, nil)

	if _, err := c.FindByBarcode(context.Background(), "96385074", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("without live: want ErrNotFound, got %v", err)
	}
	if *calls != 0 {
		t.Errorf("live=false must not call OFF")
	}
	if _, err := c.FindByBarcode(context.Background(), "96385074", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("live: want ErrNotFound, got %v", err)
	}
}

// An OFF failure is ErrUnavailable (the scanner says «riprova»), distinct
// from «not found».
func TestFindByBarcodeUnavailable(t *testing.T) {
	sqldb := openTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(sqldb, srv.URL, nil)

	_, err := c.FindByBarcode(context.Background(), "96385074", true)
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrUnavailable, got %v", err)
	}
}
