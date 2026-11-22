package off

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/db"
)

// openTestDB sets up a temporary SQLite DB with every migration applied.
// Used by every test that queries off_products / FTS5.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return store.DB
}

// seedOFFProducts inserts directly into the off_products table. The
// AFTER INSERT triggers take care of filling the FTS5 index.
func seedOFFProducts(t *testing.T, sqldb *sql.DB, products []Product) {
	t.Helper()
	for _, p := range products {
		hasFull := 0
		if p.HasFullNutrients {
			hasFull = 1
		}
		_, err := sqldb.Exec(`
			INSERT INTO off_products (
				barcode, product_name, brands, quantity,
				kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
				has_full_nutrients, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			p.Code, p.Name, p.Brand, p.Quantity,
			p.Kcal, p.Protein, p.Carbs, p.Fat, hasFull)
		if err != nil {
			t.Fatalf("seed %s: %v", p.Code, err)
		}
	}
}

func TestSearchByNameFTS5(t *testing.T) {
	sqldb := openTestDB(t)
	seedOFFProducts(t, sqldb, []Product{
		{Code: "8000000000017", Name: "Grana Padano DOP", Brand: "Consorzio", Kcal: 384, Protein: 33, Carbs: 0, Fat: 28, HasFullNutrients: true},
		{Code: "8000000000024", Name: "Parmigiano Reggiano", Brand: "Consorzio", Kcal: 392, Protein: 32, Carbs: 0, Fat: 30, HasFullNutrients: true},
		{Code: "8000080002057", Name: "Barrette Hero Light al Cioccolato", Brand: "Hero", Kcal: 350, Protein: 7, Carbs: 60, Fat: 7, HasFullNutrients: true},
		{Code: "8000000000048", Name: "Müesli croccante all'avena", Brand: "Conad", Kcal: 420, Protein: 10, Carbs: 65, Fat: 12, HasFullNutrients: true},
		{Code: "9999999999999", Name: "Pasta integrale", Brand: "GenericoX", Kcal: 0, Protein: 0, Carbs: 0, Fat: 0, HasFullNutrients: false},
	})

	c := NewClient(sqldb, "", nil)

	cases := []struct {
		name      string
		query     string
		wantCode  string // the first result expected
		wantCount int
	}{
		{"prefix match", "grana", "8000000000017", 1},
		{"due token AND", "grana padano", "8000000000017", 1},
		{"prefix sull'ultimo termine", "grana pad", "8000000000017", 1},
		{"barcode-target Hero Light", "barrette hero light cioccolato", "8000080002057", 1},
		{"diacritici - muesli trova Müesli", "muesli croccante avena", "8000000000048", 1},
		{"apostrofo - all'avena tokenizza ok", "muesli all'avena", "8000000000048", 1},
		{"no match", "nessuncibo", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.SearchByName(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("SearchByName: %v", err)
			}
			if len(got) != tc.wantCount {
				t.Errorf("count: got %d want %d (results=%+v)", len(got), tc.wantCount, got)
				return
			}
			if tc.wantCount > 0 && got[0].Code != tc.wantCode {
				t.Errorf("first.Code: got %s want %s", got[0].Code, tc.wantCode)
			}
		})
	}
}

// The words of a query may come from the name, the brand or the package size,
// in any order (#224).
func TestSearchByNameBrandAndSize(t *testing.T) {
	sqldb := openTestDB(t)
	seedOFFProducts(t, sqldb, []Product{
		{Code: "1000000000001", Name: "Yogurt greco", Brand: "Fage", Quantity: "150 g", Kcal: 97, Protein: 9, Carbs: 4, Fat: 5, HasFullNutrients: true},
		{Code: "1000000000002", Name: "Yogurt greco", Brand: "Fage", Quantity: "500 g", Kcal: 97, Protein: 9, Carbs: 4, Fat: 5, HasFullNutrients: true},
		{Code: "1000000000003", Name: "Yogurt bianco", Brand: "Parmalat", Quantity: "125 g", Kcal: 60, Protein: 4, Carbs: 5, Fat: 3, HasFullNutrients: true},
	})
	c := NewClient(sqldb, "", nil)
	for _, tc := range []struct {
		name, query string
		want        []string
	}{
		// The same product in two sizes is one result (the search collapses
		// exact duplicates); a size in the query picks the size.
		{"name then brand", "yogurt fage", []string{"1000000000001"}},
		{"brand first", "fage yogurt", []string{"1000000000001"}},
		{"name, brand and size", "yogurt fage 150", []string{"1000000000001"}},
		{"size with its unit", "parmalat 125 g", []string{"1000000000003"}},
		{"size of the other one", "greco 500", []string{"1000000000002"}},
		{"a word that is nowhere", "yogurt fage 999", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.SearchByName(context.Background(), tc.query)
			if err != nil {
				t.Fatal(err)
			}
			var codes []string
			for _, p := range got {
				codes = append(codes, p.Code)
			}
			if len(codes) != len(tc.want) {
				t.Fatalf("%q: %v, want %v", tc.query, codes, tc.want)
			}
			if tc.name == "name then brand" || tc.name == "brand first" {
				return // one result, whichever size the search keeps
			}
			for _, w := range tc.want {
				found := false
				for _, c := range codes {
					found = found || c == w
				}
				if !found {
					t.Errorf("%q: %v is missing %s", tc.query, codes, w)
				}
			}
		})
	}

	// The index can be rebuilt from the table (the migration does it).
	if _, err := sqldb.Exec(`INSERT INTO off_products_fts(off_products_fts) VALUES('rebuild')`); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.SearchByName(context.Background(), "fage 500"); len(got) != 1 {
		t.Errorf("after a rebuild: %d results, want 1", len(got))
	}
}

func TestSearchByNameEmpty(t *testing.T) {
	sqldb := openTestDB(t)
	c := NewClient(sqldb, "", nil)
	got, err := c.SearchByName(context.Background(), "  ")
	if err != nil {
		t.Fatalf("SearchByName empty: %v", err)
	}
	if got != nil {
		t.Errorf("an empty query must return nil, got %+v", got)
	}
}

func TestSearchByNameNoDB(t *testing.T) {
	c := NewClient(nil, "", nil)
	_, err := c.SearchByName(context.Background(), "grana")
	if err == nil {
		t.Fatal("want an error with a nil DB")
	}
}

func TestBuildFTSMatch(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"grana padano", `"grana"* "padano"*`},
		{"grana pad", `"grana"* "pad"*`},
		{"all'avena", `"all"* "avena"*`},
		{`prodotto "speciale"`, `"prodotto"* "speciale"*`},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := buildFTSMatch(tc.in); got != tc.want {
			t.Errorf("buildFTSMatch(%q): got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestClientGetByBarcode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v2/product/") {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
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

	c := NewClient(nil, srv.URL, nil)
	p, err := c.GetByBarcode(context.Background(), "1234567890")
	if err != nil {
		t.Fatalf("GetByBarcode: %v", err)
	}
	if p.Code != "1234567890" || p.Name != "Latte UHT" {
		t.Errorf("product: %+v", p)
	}
	expectedKcal := 200.0 / 4.184
	if diff := p.Kcal - expectedKcal; diff > 0.01 || diff < -0.01 {
		t.Errorf("kcal from kJ: %v (expected ~%v)", p.Kcal, expectedKcal)
	}
	if !p.HasFullNutrients {
		t.Errorf("nutrients should come out complete (kcal derived from kJ)")
	}
}

func TestClientGetByBarcodeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": 0, "code": "9999"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(nil, srv.URL, nil)
	_, err := c.GetByBarcode(context.Background(), "9999")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// The v2 API today answers 404 (with status:0 in the body) to a code it does
// not know: that is «not found», not an OFF failure.
func TestClientGetByBarcodeNotFound404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"9999","status":0,"status_verbose":"product not found"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(nil, srv.URL, nil)
	_, err := c.GetByBarcode(context.Background(), "9999")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
