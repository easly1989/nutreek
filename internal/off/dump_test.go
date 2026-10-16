package off

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeDumpGzip builds an in-memory JSONL.gz dump with the given lines.
// Every line must already be valid JSON (one product per line).
func makeDumpGzip(t *testing.T, lines []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	for _, l := range lines {
		if _, err := gz.Write([]byte(l + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSyncerImport(t *testing.T) {
	sqldb := openTestDB(t)

	// 6 products in total:
	//   - 3 Italian (one with complete macros, one incomplete, one with an empty code)
	//   - 2 foreign (must be skipped)
	//   - 1 malformed line (must be skipped silently)
	// The real JSONL dump uses bare keys (without _100g): "proteins", not
	// "proteins_100g". The tests use this format to mirror reality.
	dump := makeDumpGzip(t, []string{
		`{"code":"8000000000017","product_name":"Grana Padano DOP","brands":"Consorzio","countries_tags":["en:italy"],"serving_quantity":30,"serving_quantity_unit":"g","nutriments":{"energy-kcal":384,"proteins":33,"carbohydrates":0,"fat":28}}`,
		`{"code":"FR_PRODUCT","product_name":"Camembert","countries_tags":["en:france"]}`,
		`{"code":"8000080002057","product_name":"Barrette Hero Light","brands":"Hero","countries_tags":["en:italy","en:germany"],"nutriments":{"proteins":7,"carbohydrates":60}}`,
		`{"code":"","product_name":"Senza codice","countries_tags":["en:italy"]}`,
		`{"code":"DE_PRODUCT","product_name":"Wurst","countries_tags":["en:germany"]}`,
		`{not valid json`,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if im := r.Header.Get("If-None-Match"); im != "" {
			// Simulate a 304 when the client sends the known ETag.
			if im == `"fake-etag-v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		w.Header().Set("ETag", `"fake-etag-v1"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(dump)
	}))
	t.Cleanup(srv.Close)

	syncer := NewSyncer(sqldb, srv.URL, nil)

	stats, err := syncer.SyncDump(context.Background())
	if err != nil {
		t.Fatalf("SyncDump: %v", err)
	}
	if stats.Skipped {
		t.Error("the first sync must not be skipped")
	}
	// Italian products imported: only those with code != "" → 2 (Grana + Barrette).
	if stats.Imported != 2 {
		t.Errorf("imported: got %d want 2", stats.Imported)
	}
	if stats.ScannedTotal != 6 {
		t.Errorf("scanned: got %d want 6", stats.ScannedTotal)
	}

	// Check the DB content: Grana has complete macros, Barrette does not.
	var count int
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&count)
	if count != 2 {
		t.Errorf("off_products row count: got %d want 2", count)
	}

	var hasFull int
	_ = sqldb.QueryRow(`SELECT has_full_nutrients FROM off_products WHERE barcode = ?`, "8000000000017").Scan(&hasFull)
	if hasFull != 1 {
		t.Errorf("Grana must have has_full_nutrients=1, got %d", hasFull)
	}
	_ = sqldb.QueryRow(`SELECT has_full_nutrients FROM off_products WHERE barcode = ?`, "8000080002057").Scan(&hasFull)
	if hasFull != 0 {
		t.Errorf("Barrette must have has_full_nutrients=0 (kcal/fat missing), got %d", hasFull)
	}

	// The serving reaches the lookup by code (#136): 30 g for the Grana,
	// unknown for the Barrette.
	cl := &Client{DB: sqldb}
	if p, err := cl.localByBarcode(context.Background(), "8000000000017"); err != nil || p == nil || p.ServingG == nil || *p.ServingG != 30 {
		t.Errorf("Grana: serving %v (err %v), want 30", p, err)
	}
	if p, err := cl.localByBarcode(context.Background(), "8000080002057"); err != nil || p == nil || p.ServingG != nil {
		t.Errorf("Barrette: serving %v (err %v), want nil", p, err)
	}

	// Stato persistito.
	state, err := syncer.LoadState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.LastSyncedAt == nil {
		t.Error("last_synced_at not filled")
	}
	if state.ProductsCount != 2 {
		t.Errorf("products_count: got %d want 2", state.ProductsCount)
	}
	if state.DumpETag != `"fake-etag-v1"` {
		t.Errorf("etag: got %q", state.DumpETag)
	}

	// Second call: the server answers 304 when the ETag matches → skip.
	stats2, err := syncer.SyncDump(context.Background())
	if err != nil {
		t.Fatalf("SyncDump 2: %v", err)
	}
	if !stats2.Skipped {
		t.Error("a second sync with the same ETag must be skipped")
	}
	// The DB stays filled.
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&count)
	if count != 2 {
		t.Errorf("after a 304 the rows must stay, got %d", count)
	}
}

func TestSyncerFTSPopulated(t *testing.T) {
	sqldb := openTestDB(t)
	dump := makeDumpGzip(t, []string{
		`{"code":"8000000000017","product_name":"Grana Padano DOP","brands":"Consorzio","countries_tags":["en:italy"],"nutriments":{"energy-kcal":384,"proteins":33,"carbohydrates":0,"fat":28}}`,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(dump)
	}))
	t.Cleanup(srv.Close)

	syncer := NewSyncer(sqldb, srv.URL, nil)
	if _, err := syncer.SyncDump(context.Background()); err != nil {
		t.Fatal(err)
	}

	// FTS5 filled via the AFTER INSERT trigger: the client search must find the product.
	c := NewClient(sqldb, "", nil)
	got, err := c.SearchByName(context.Background(), "grana padano")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Code != "8000000000017" {
		t.Errorf("FTS search after the sync: got %+v", got)
	}
}

func TestSyncerConcurrent(t *testing.T) {
	sqldb := openTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// An empty but valid dump — the content does not matter.
		_, _ = w.Write(makeDumpGzip(t, []string{}))
	}))
	t.Cleanup(srv.Close)

	syncer := NewSyncer(sqldb, srv.URL, nil)
	// Mark syncing by hand → the second call must fail with ErrSyncInProgress.
	syncer.mu.Lock()
	syncer.syncing = true
	syncer.mu.Unlock()
	_, err := syncer.SyncDump(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gia' in corso") {
		t.Errorf("want ErrSyncInProgress, got %v", err)
	}
}
