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

// makeDumpGzip costruisce un dump JSONL.gz in memoria con le righe date.
// Ogni riga deve essere gia' JSON valido (un prodotto per riga).
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

	// 6 prodotti totali:
	//   - 3 italiani (uno con macro completi, uno incompleti, uno con codice vuoto)
	//   - 2 stranieri (devono essere skippati)
	//   - 1 riga malformata (deve essere skippata silenziosamente)
	dump := makeDumpGzip(t, []string{
		`{"code":"8000000000017","product_name":"Grana Padano DOP","brands":"Consorzio","countries_tags":["en:italy"],"nutriments":{"energy-kcal_100g":384,"proteins_100g":33,"carbohydrates_100g":0,"fat_100g":28}}`,
		`{"code":"FR_PRODUCT","product_name":"Camembert","countries_tags":["en:france"]}`,
		`{"code":"8000080002057","product_name":"Barrette Hero Light","brands":"Hero","countries_tags":["en:italy","en:germany"],"nutriments":{"proteins_100g":7,"carbohydrates_100g":60}}`,
		`{"code":"","product_name":"Senza codice","countries_tags":["en:italy"]}`,
		`{"code":"DE_PRODUCT","product_name":"Wurst","countries_tags":["en:germany"]}`,
		`{not valid json`,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if im := r.Header.Get("If-None-Match"); im != "" {
			// Simula 304 quando il client manda l'ETag conosciuto.
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
		t.Error("primo sync non deve essere skipped")
	}
	// Italiani importati: solo quelli con code != "" → 2 (Grana + Barrette).
	if stats.Imported != 2 {
		t.Errorf("imported: got %d want 2", stats.Imported)
	}
	if stats.ScannedTotal != 6 {
		t.Errorf("scanned: got %d want 6", stats.ScannedTotal)
	}

	// Verifica contenuto DB: Grana ha macro completi, Barrette no.
	var count int
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&count)
	if count != 2 {
		t.Errorf("off_products row count: got %d want 2", count)
	}

	var hasFull int
	_ = sqldb.QueryRow(`SELECT has_full_nutrients FROM off_products WHERE barcode = ?`, "8000000000017").Scan(&hasFull)
	if hasFull != 1 {
		t.Errorf("Grana deve avere has_full_nutrients=1, got %d", hasFull)
	}
	_ = sqldb.QueryRow(`SELECT has_full_nutrients FROM off_products WHERE barcode = ?`, "8000080002057").Scan(&hasFull)
	if hasFull != 0 {
		t.Errorf("Barrette deve avere has_full_nutrients=0 (manca kcal/fat), got %d", hasFull)
	}

	// Stato persistito.
	state, err := syncer.LoadState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.LastSyncedAt == nil {
		t.Error("last_synced_at non popolato")
	}
	if state.ProductsCount != 2 {
		t.Errorf("products_count: got %d want 2", state.ProductsCount)
	}
	if state.DumpETag != `"fake-etag-v1"` {
		t.Errorf("etag: got %q", state.DumpETag)
	}

	// Seconda chiamata: il server risponde 304 se l'ETag matcha → skip.
	stats2, err := syncer.SyncDump(context.Background())
	if err != nil {
		t.Fatalf("SyncDump 2: %v", err)
	}
	if !stats2.Skipped {
		t.Error("secondo sync con stesso ETag deve essere skipped")
	}
	// Il DB resta popolato.
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&count)
	if count != 2 {
		t.Errorf("dopo 304 le righe devono restare, got %d", count)
	}
}

func TestSyncerFTSPopulated(t *testing.T) {
	sqldb := openTestDB(t)
	dump := makeDumpGzip(t, []string{
		`{"code":"8000000000017","product_name":"Grana Padano DOP","brands":"Consorzio","countries_tags":["en:italy"],"nutriments":{"energy-kcal_100g":384,"proteins_100g":33,"carbohydrates_100g":0,"fat_100g":28}}`,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(dump)
	}))
	t.Cleanup(srv.Close)

	syncer := NewSyncer(sqldb, srv.URL, nil)
	if _, err := syncer.SyncDump(context.Background()); err != nil {
		t.Fatal(err)
	}

	// FTS5 popolato via trigger AFTER INSERT: ricerca client deve trovare il prodotto.
	c := NewClient(sqldb, "", nil)
	got, err := c.SearchByName(context.Background(), "grana padano")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Code != "8000000000017" {
		t.Errorf("ricerca FTS dopo sync: got %+v", got)
	}
}

func TestSyncerConcurrent(t *testing.T) {
	sqldb := openTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Dump vuoto ma valido — non importa il contenuto.
		_, _ = w.Write(makeDumpGzip(t, []string{}))
	}))
	t.Cleanup(srv.Close)

	syncer := NewSyncer(sqldb, srv.URL, nil)
	// Manually mark syncing → seconda chiamata deve fallire con ErrSyncInProgress.
	syncer.mu.Lock()
	syncer.syncing = true
	syncer.mu.Unlock()
	_, err := syncer.SyncDump(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gia' in corso") {
		t.Errorf("atteso ErrSyncInProgress, got %v", err)
	}
}
