package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

// A search that is a valid barcode becomes a search by code: first the
// user's own ingredients, then the local copy of OFF, the network only once
// typing stops (includeOFF).
func TestSearchMixedBarcode(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	liveCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		liveCalls++
		http.Error(w, "giu'", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	a.OFF = off.NewClient(a.Store.DB, srv.URL, nil)

	localID, _ := a.Ingredients.CreateLocal(ctx, "Biscotti del forno", nil, domain.UnitGram, 450, 7, 70, 15, nil, nil, nil, nil, nil, nil, nil)
	if err := a.Ingredients.SetLocalBarcode(ctx, localID, "4006381333931"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.DB.Exec(`INSERT INTO off_products (barcode, product_name, brands, quantity,
		kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, updated_at)
		VALUES ('96385074', 'Fiocchi d''avena', 'Molino', '500 g', 372, 13.5, 58.7, 7, 1, 0),
		       ('8015555555556', 'Crusca', 'Molino', '500 g', 0, 0, 0, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}

	rows, warn, err := a.searchMixed(ctx, "4 006381 333931", true)
	if err != nil || warn != "" || len(rows) != 1 || rows[0].LocalID() != localID {
		t.Errorf("own ingredient: %+v, %q, %v", rows, warn, err)
	}

	rows, _, err = a.searchMixed(ctx, "96385074", false)
	if err != nil || len(rows) != 1 || !rows[0].IsOFF() || rows[0].Barcode() != "96385074" {
		t.Errorf("local OFF copy: %+v, %v", rows, err)
	}

	// A local copy without the four macros cannot be imported: not offered (#204).
	rows, warn, err = a.searchMixed(ctx, "8015555555556", false)
	if err != nil || len(rows) != 0 || warn != "" || liveCalls != 0 {
		t.Errorf("incomplete local OFF copy: %+v, %q, %v, %d calls", rows, warn, err, liveCalls)
	}

	rows, warn, err = a.searchMixed(ctx, "036000291452", false)
	if err != nil || len(rows) != 0 || warn != "" || liveCalls != 0 {
		t.Errorf("unknown without the network: %+v, %q, %v, %d calls", rows, warn, err, liveCalls)
	}

	rows, warn, err = a.searchMixed(ctx, "036000291452", true)
	if err != nil || len(rows) != 0 || warn == "" || liveCalls != 1 {
		t.Errorf("OFF down: %+v, %q, %v, %d calls", rows, warn, err, liveCalls)
	}
}
