package off

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"
	"time"
)

// makeDumpGzip builds an in-memory .gz file with the given lines.
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

// csvHeader is a trimmed header of the CSV export: the columns the bootstrap
// reads, in another order, plus one it ignores.
var csvHeader = []string{
	"code", "url", "last_modified_t", "product_name", "brands", "quantity", "countries_tags",
	"serving_size", "serving_quantity", "image_small_url", "energy-kcal_100g", "energy_100g",
	"fat_100g", "saturated-fat_100g", "carbohydrates_100g", "sugars_100g", "fiber_100g",
	"proteins_100g", "sodium_100g",
}

type csvRow map[string]string

// makeCSVGzip builds a gzipped CSV export from rows, then the raw lines.
func makeCSVGzip(t *testing.T, rows []csvRow, raw ...string) []byte {
	t.Helper()
	lines := []string{strings.Join(csvHeader, "\t")}
	for _, r := range rows {
		cells := make([]string, len(csvHeader))
		for i, c := range csvHeader {
			cells[i] = r[c]
		}
		lines = append(lines, strings.Join(cells, "\t"))
	}
	return makeDumpGzip(t, append(lines, raw...))
}

// italian is a CSV row of an Italian product with complete macros.
func italian(code, name string, modified int64) csvRow {
	return csvRow{
		"code": code, "product_name": name, "last_modified_t": fmt.Sprint(modified),
		"countries_tags": "en:france,en:italy", "energy-kcal_100g": "384",
		"proteins_100g": "33", "carbohydrates_100g": "0", "fat_100g": "28",
	}
}

// deltaLine is a delta line of a product with complete macros.
func deltaLine(code, name, country string) string {
	return `{"code":"` + code + `","product_name":"` + name + `","countries_tags":["` + country + `"],` +
		`"nutriments":{"energy-kcal_100g":390,"proteins_100g":33,"carbohydrates_100g":0,"fat_100g":28}}`
}

// fakeOFF serves a CSV export, a delta index and its files, and records the
// paths asked for.
type fakeOFF struct {
	mu     sync.Mutex
	csv    []byte
	csvFn  http.HandlerFunc // replaces csv when set
	index  []string         // newest first, like the real one
	deltas map[string][]byte
	hits   []string
}

func (f *fakeOFF) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits = append(f.hits, r.URL.Path)
	csv, csvFn, index := f.csv, f.csvFn, strings.Join(f.index, "\n")
	file, isDelta := f.deltas[path.Base(r.URL.Path)]
	f.mu.Unlock()
	switch {
	case r.URL.Path == "/products.csv.gz" && csvFn != nil:
		csvFn(w, r)
	case r.URL.Path == "/products.csv.gz":
		_, _ = w.Write(csv)
	case r.URL.Path == "/delta/index.txt":
		_, _ = w.Write([]byte(index + "\n"))
	case isDelta:
		_, _ = w.Write(file)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeOFF) addDelta(t *testing.T, from, to int64, lines ...string) string {
	t.Helper()
	name := fmt.Sprintf("openfoodfacts_products_%d_%d.json.gz", from, to)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deltas[name] = makeDumpGzip(t, lines)
	f.index = append([]string{name}, f.index...)
	return name
}

// takeHits returns the paths asked for since the last call.
func (f *fakeOFF) takeHits() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.hits
	f.hits = nil
	return h
}

func newFakeOFF(t *testing.T) (*fakeOFF, *Syncer, *sql.DB) {
	t.Helper()
	f := &fakeOFF{deltas: map[string][]byte{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	sqldb := openTestDB(t)
	return f, NewSyncer(sqldb, srv.URL+"/products.csv.gz", srv.URL+"/delta/", nil), sqldb
}

func countCSV(hits []string) int {
	n := 0
	for _, h := range hits {
		if h == "/products.csv.gz" {
			n++
		}
	}
	return n
}

func searchCodes(t *testing.T, sqldb *sql.DB, q string) []string {
	t.Helper()
	got, err := NewClient(sqldb, "", nil).SearchByName(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, p := range got {
		codes = append(codes, p.Code)
	}
	return codes
}

func TestBootstrapFromCSV(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	grana := italian("8000000000017", "Grana Padano DOP", 1000)
	grana["brands"], grana["serving_quantity"] = "Consorzio,Altro", "30"
	grana["image_small_url"] = "https://images.openfoodfacts.org/images/products/800/000/000/0017/front_it.3.200.jpg"
	partial := csvRow{"code": "8000080002057", "product_name": "Barrette Hero Light", "last_modified_t": "1200",
		"countries_tags": "en:italy,en:germany", "proteins_100g": "7", "carbohydrates_100g": "60"}
	kJ := italian("8000000000031", "Biscotti kJ", 900)
	kJ["energy-kcal_100g"] = "1320"
	heavy := italian("8000000000048", "Biscotti pesanti", 900)
	heavy["proteins_100g"], heavy["carbohydrates_100g"], heavy["fat_100g"] = "20", "70", "16"
	energy := italian("8000000000055", "Pane", 900)
	energy["energy-kcal_100g"], energy["energy_100g"] = "", "1046"
	foreignImg := italian("8000000000062", "Pasta", 900)
	foreignImg["image_small_url"] = "https://evil.example/front.jpg"
	oldHost := italian("8000000000079", "Riso", 900)
	oldHost["image_small_url"] = "https://static.openfoodfacts.org/images/products/800/000/000/0079/front_it.1.200.jpg"
	noName := italian("8000000000086", "", 900)
	quoted := italian("8000000000093", `"Pasta ""Bio"""`, 900) // a field holding quotes is quoted
	f.csv = makeCSVGzip(t, []csvRow{
		grana, partial, kJ, heavy, energy, foreignImg, oldHost, noName, quoted,
		{"code": "3000000000001", "product_name": "Camembert", "last_modified_t": "5000", "countries_tags": "en:france"},
	})

	stats, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !stats.Bootstrap || stats.ScannedTotal != 10 || stats.Imported != 8 || stats.Deltas != 0 {
		t.Errorf("stats: %+v", stats)
	}
	type row struct {
		name, brand, image string
		kcal               float64
		full               bool
		serving            sql.NullFloat64
	}
	got := map[string]row{}
	rows, err := sqldb.Query(`SELECT barcode, product_name, brands, image_url, kcal_per_100, has_full_nutrients, serving_g FROM off_products`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code string
		var r row
		if err := rows.Scan(&code, &r.name, &r.brand, &r.image, &r.kcal, &r.full, &r.serving); err != nil {
			t.Fatal(err)
		}
		got[code] = r
	}
	_ = rows.Close()
	if len(got) != 8 {
		t.Errorf("products: %d, want 8: %+v", len(got), got)
	}
	g := got["8000000000017"]
	if !g.full || g.brand != "Consorzio" || g.serving.Float64 != 30 || g.image != grana["image_small_url"] {
		t.Errorf("Grana: %+v", g)
	}
	for code, want := range map[string]bool{
		"8000080002057": false, // two macros missing
		"8000000000031": false, // kJ entered as kcal
		"8000000000048": false, // 106 g of macros per 100 g
		"8000000000055": true,  // kcal from kJ
	} {
		if got[code].full != want {
			t.Errorf("%s: importable %v, want %v", code, got[code].full, want)
		}
	}
	if k := got["8000000000055"].kcal; k < 249.9 || k > 250.1 {
		t.Errorf("kcal from 1046 kJ: %v", k)
	}
	if got["8000000000062"].image != "" {
		t.Errorf("a photo from another host is kept: %q", got["8000000000062"].image)
	}
	if want := "https://images.openfoodfacts.org/images/products/800/000/000/0079/front_it.1.200.jpg"; got["8000000000079"].image != want {
		t.Errorf("old host photo: %q", got["8000000000079"].image)
	}
	if got["8000000000093"].name != `Pasta "Bio"` {
		t.Errorf("quoted name: %q", got["8000000000093"].name)
	}
	if codes := searchCodes(t, sqldb, "grana padano"); len(codes) != 1 || codes[0] != "8000000000017" {
		t.Errorf("FTS after the bootstrap: %v", codes)
	}

	st, err := syncer.LoadState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The high-water mark is the newest change of any country.
	if st.HighWater == nil || st.HighWater.Unix() != 5000 || st.LastBootstrapAt == nil || st.LastDeltaAt != nil ||
		st.ParserVersion != ParserVersion || st.ProductsCount != 8 || st.LastSyncedAt == nil {
		t.Errorf("state: %+v", st)
	}
	var staged int
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products_staging`).Scan(&staged)
	if staged != 0 {
		t.Errorf("staging left with %d rows", staged)
	}
}

func TestDeltasAfterHighWater(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	ctx := context.Background()
	f.csv = makeCSVGzip(t, []csvRow{
		italian("8000000000017", "Grana Padano", 1000),
		italian("8000080002057", "Barrette Hero", 1000),
		{"code": "3000000000001", "product_name": "Camembert", "last_modified_t": "2000", "countries_tags": "en:france"},
	})
	old := f.addDelta(t, 500, 1500, deltaLine("8000000000017", "Never applied", "en:italy"))
	d2 := f.addDelta(t, 1500, 2500, deltaLine("8000000000017", "Grana Second", "en:italy"))
	d3 := f.addDelta(t, 2500, 3500,
		deltaLine("8000000000017", "Grana Third", "en:italy"),
		deltaLine("8000000000024", "Nuovo Pesto", "en:italy"),
		deltaLine("8000080002057", "Barrette Hero", "en:france"), // leaves Italy
		deltaLine("3000000000002", "Brie", "en:france"),
		`{broken`,
	)

	stats, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	want := []string{"/products.csv.gz", "/delta/index.txt", "/delta/" + d2, "/delta/" + d3}
	if hits := f.takeHits(); strings.Join(hits, " ") != strings.Join(want, " ") {
		t.Errorf("downloads: %v, want %v (not %s)", hits, want, old)
	}
	if !stats.Bootstrap || stats.Deltas != 2 || stats.Removed != 1 {
		t.Errorf("stats: %+v", stats)
	}
	var name string
	var kcal float64
	_ = sqldb.QueryRow(`SELECT product_name, kcal_per_100 FROM off_products WHERE barcode = '8000000000017'`).Scan(&name, &kcal)
	if name != "Grana Third" || kcal != 390 {
		t.Errorf("Grana after the deltas: %q %v, want the newest delta's", name, kcal)
	}
	if codes := searchCodes(t, sqldb, "barrette"); len(codes) != 0 {
		t.Errorf("a product that left Italy is still found: %v", codes)
	}
	if codes := searchCodes(t, sqldb, "pesto"); len(codes) != 1 {
		t.Errorf("a new Italian product from a delta: %v", codes)
	}
	var n int
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&n)
	if n != 2 {
		t.Errorf("products: %d, want 2 (no foreign one)", n)
	}
	st, _ := syncer.LoadState(ctx)
	if st.HighWater.Unix() != 3500 || st.LastDeltaAt == nil || st.ProductsCount != 2 {
		t.Errorf("state: %+v", st)
	}

	// Nothing new: only the index is read.
	if _, err := syncer.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if hits := f.takeHits(); len(hits) != 1 || hits[0] != "/delta/index.txt" {
		t.Errorf("an idle sync downloaded %v", hits)
	}

	// The next day: only the new delta.
	d4 := f.addDelta(t, 3500, 4500, deltaLine("8000000000017", "Grana Fourth", "en:italy"))
	stats, err = syncer.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if hits := f.takeHits(); len(hits) != 2 || hits[1] != "/delta/"+d4 || stats.Bootstrap || stats.Deltas != 1 {
		t.Errorf("next day: %v, %+v", hits, stats)
	}
	if st, _ := syncer.LoadState(ctx); st.HighWater.Unix() != 4500 {
		t.Errorf("high water: %v", st.HighWater)
	}
}

func TestRebootstrapTriggers(t *testing.T) {
	cases := map[string]string{
		"a gap in the deltas":    `UPDATE off_sync_state SET high_water = 1000`,
		"another parser version": fmt.Sprintf(`UPDATE off_sync_state SET parser_version = %d`, ParserVersion-1),
		"a bootstrap 31 days old": fmt.Sprintf(`UPDATE off_sync_state SET last_bootstrap_at = %d`,
			time.Now().Add(-31*24*time.Hour).Unix()),
	}
	for name, update := range cases {
		t.Run(name, func(t *testing.T) {
			f, syncer, sqldb := newFakeOFF(t)
			ctx := context.Background()
			f.csv = makeCSVGzip(t, []csvRow{italian("8000000000017", "Grana Padano", 3000)})
			f.addDelta(t, 2000, 4000, deltaLine("8000000000017", "Grana", "en:italy"))
			if _, err := syncer.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			if countCSV(f.takeHits()) != 1 {
				t.Fatal("no first bootstrap")
			}
			if _, err := sqldb.Exec(update); err != nil {
				t.Fatal(err)
			}
			stats, err := syncer.Sync(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if countCSV(f.takeHits()) != 1 || !stats.Bootstrap {
				t.Errorf("no bootstrap: %+v", stats)
			}
			// Bootstrapped again, the deltas are enough.
			if _, err := syncer.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			if countCSV(f.takeHits()) != 0 {
				t.Error("bootstrapped twice")
			}
		})
	}
}

// bigCSV is an export of n Italian biscuits, enough for several batches,
// then the raw lines.
func bigCSV(t *testing.T, n int, raw ...string) []byte {
	rows := make([]csvRow, n)
	for i := range rows {
		rows[i] = italian(fmt.Sprintf("900%010d", i), fmt.Sprintf("Biscotto %d", i), 5000)
	}
	return makeCSVGzip(t, rows, raw...)
}

func setCSV(f *fakeOFF, csv []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.csv = csv
}

func countProducts(t *testing.T, sqldb *sql.DB) int {
	t.Helper()
	var n int
	if err := sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Up to 1% of malformed rows is noise; more means a broken export.
func TestBootstrapMalformedRows(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	bad := []string{"1\ttoo few", "2\ttoo few", "3\ttoo few", "4\ttoo few"}
	setCSV(f, bigCSV(t, 300, bad[:3]...)) // 3 of 303
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("1%% of malformed rows: %v", err)
	}
	setCSV(f, makeCSVGzip(t, []csvRow{italian("8000000000017", "Grana Padano", 1000)}, bad...)) // 4 of 5
	if _, err := syncer.Bootstrap(context.Background()); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("want a malformed-rows error, got %v", err)
	}
	if n := countProducts(t, sqldb); n != 300 {
		t.Errorf("products after a malformed export: %d, want 300", n)
	}
}

// An export that shrank below 80% of the products in use is not swapped in.
func TestBootstrapRefusesShrunkExport(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	setCSV(f, bigCSV(t, 2000))
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, _ := syncer.LoadState(context.Background())
	setCSV(f, bigCSV(t, 1500))
	if _, err := syncer.Bootstrap(context.Background()); err == nil {
		t.Fatal("a 75% export must be refused")
	}
	if n := countProducts(t, sqldb); n != 2000 {
		t.Errorf("products after a refused swap: %d, want 2000", n)
	}
	if st, _ := syncer.LoadState(context.Background()); !st.LastBootstrapAt.Equal(*before.LastBootstrapAt) || st.LastError == "" {
		t.Errorf("state after a refused swap: %+v", st)
	}
	setCSV(f, bigCSV(t, 1700))
	if _, err := syncer.Bootstrap(context.Background()); err != nil {
		t.Fatalf("an 85%% export: %v", err)
	}
	if n := countProducts(t, sqldb); n != 1700 {
		t.Errorf("products after an 85%% export: %d, want 1700", n)
	}
}

// A swap that fails midway leaves the state as it was: the next sync
// bootstraps again and completes it.
func TestSwapFailureMidway(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	bootstrapGrana(t, f, syncer)
	// Biscuit 5500 is in the second chunk of the swap.
	if _, err := sqldb.Exec(`CREATE TRIGGER fail_swap BEFORE INSERT ON off_products
		WHEN new.barcode = '9000000005500' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	setCSV(f, bigCSV(t, 6000))
	// Forced: the next, plain sync has no reason of its own to bootstrap.
	if _, err := syncer.Bootstrap(context.Background()); err == nil {
		t.Fatal("the swap must fail")
	}
	if st, _ := syncer.LoadState(context.Background()); st.HighWater != nil {
		t.Errorf("high water after a failed swap: %v, want none", st.HighWater)
	}
	if n := countProducts(t, sqldb); n != swapChunk {
		t.Errorf("products after a failed swap: %d, want the first chunk", n)
	}
	if _, err := sqldb.Exec(`DROP TRIGGER fail_swap`); err != nil {
		t.Fatal(err)
	}
	stats, err := syncer.Sync(context.Background())
	if err != nil || !stats.Bootstrap {
		t.Fatalf("next sync: %+v, %v", stats, err)
	}
	if n := countProducts(t, sqldb); n != 6000 {
		t.Errorf("products after the retry: %d, want 6000", n)
	}
}

// A product dated in the future does not push the high-water mark past now.
func TestHighWaterClampedToNow(t *testing.T) {
	f, syncer, _ := newFakeOFF(t)
	setCSV(f, makeCSVGzip(t, []csvRow{italian("8000000000017", "Grana Padano", time.Now().Add(10*24*time.Hour).Unix())}))
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, _ := syncer.LoadState(context.Background())
	if st.HighWater.After(time.Now()) {
		t.Errorf("high water in the future: %v", st.HighWater)
	}
}

// An export older than every delta: they all apply, oldest first, and the
// next day reads only the new delta instead of the CSV again.
func TestExportOlderThanDeltas(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	ctx := context.Background()
	setCSV(f, makeCSVGzip(t, []csvRow{italian("8000000000017", "Grana Padano", 1000)}))
	d1 := f.addDelta(t, 2000, 3000, deltaLine("8000000000017", "Grana Second", "en:italy"))
	d2 := f.addDelta(t, 3000, 4000, deltaLine("8000000000017", "Grana Third", "en:italy"))
	stats, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/products.csv.gz", "/delta/index.txt", "/delta/" + d1, "/delta/" + d2}
	if hits := f.takeHits(); strings.Join(hits, " ") != strings.Join(want, " ") || stats.Deltas != 2 {
		t.Errorf("downloads: %v, want %v; %+v", hits, want, stats)
	}
	var name string
	_ = sqldb.QueryRow(`SELECT product_name FROM off_products`).Scan(&name)
	if st, _ := syncer.LoadState(ctx); st.HighWater.Unix() != 4000 || name != "Grana Third" {
		t.Errorf("high water %v, name %q", st.HighWater, name)
	}
	d3 := f.addDelta(t, 4000, 5000, deltaLine("8000000000017", "Grana Fourth", "en:italy"))
	if _, err := syncer.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if hits := f.takeHits(); len(hits) != 2 || hits[1] != "/delta/"+d3 {
		t.Errorf("next day: %v", hits)
	}
}

func bootstrapGrana(t *testing.T, f *fakeOFF, syncer *Syncer) {
	t.Helper()
	f.csv = makeCSVGzip(t, []csvRow{italian("8000000000017", "Grana Padano", 1000)})
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSearchDuringBootstrap(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	bootstrapGrana(t, f, syncer)

	// The next export arrives in two halves; the test looks in between.
	next := bigCSV(t, 4000)
	release := make(chan struct{})
	f.mu.Lock()
	f.csvFn = func(w http.ResponseWriter, _ *http.Request) {
		half := len(next) / 2
		_, _ = w.Write(next[:half])
		w.(http.Flusher).Flush()
		<-release
		_, _ = w.Write(next[half:])
	}
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := syncer.Bootstrap(context.Background())
		done <- err
	}()

	deadline := time.Now().Add(10 * time.Second)
	for staged := 0; staged < dumpBatchSize; {
		if time.Now().After(deadline) {
			close(release)
			t.Fatalf("staging never filled: %d rows", staged)
		}
		time.Sleep(20 * time.Millisecond)
		_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products_staging`).Scan(&staged)
	}
	if codes := searchCodes(t, sqldb, "grana"); len(codes) != 1 {
		t.Errorf("mid-bootstrap the old product is gone: %v", codes)
	}
	if codes := searchCodes(t, sqldb, "biscotto"); len(codes) != 0 {
		t.Errorf("mid-bootstrap the new products show: %v", codes)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if codes := searchCodes(t, sqldb, "grana"); len(codes) != 0 {
		t.Errorf("after the swap the old product stays: %v", codes)
	}
	var n int
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products_fts WHERE off_products_fts MATCH 'biscotto'`).Scan(&n)
	if n != 4000 {
		t.Errorf("FTS rows after the swap: %d, want 4000", n)
	}
}

func TestFailedBootstrapKeepsData(t *testing.T) {
	f, syncer, sqldb := newFakeOFF(t)
	bootstrapGrana(t, f, syncer)
	before, _ := syncer.LoadState(context.Background())

	// The connection drops halfway through the export.
	next := bigCSV(t, 4000)
	f.mu.Lock()
	f.csv = next[:len(next)/2]
	f.mu.Unlock()
	if _, err := syncer.Bootstrap(context.Background()); err == nil {
		t.Fatal("a truncated export must fail")
	}
	if codes := searchCodes(t, sqldb, "grana"); len(codes) != 1 {
		t.Errorf("the old product is gone: %v", codes)
	}
	var n int
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&n)
	if n != 1 {
		t.Errorf("products after a failed bootstrap: %d, want 1", n)
	}
	st, _ := syncer.LoadState(context.Background())
	if st.LastError == "" || st.HighWater.Unix() != before.HighWater.Unix() || !st.LastBootstrapAt.Equal(*before.LastBootstrapAt) {
		t.Errorf("state after a failed bootstrap: %+v", st)
	}

	// An export without Italian products does not empty the table either.
	f.mu.Lock()
	f.csv = makeCSVGzip(t, []csvRow{{"code": "3000000000001", "product_name": "Brie", "last_modified_t": "9000", "countries_tags": "en:france"}})
	f.mu.Unlock()
	if _, err := syncer.Bootstrap(context.Background()); err == nil {
		t.Error("an export without Italian products must fail")
	}
	_ = sqldb.QueryRow(`SELECT COUNT(*) FROM off_products`).Scan(&n)
	if n != 1 {
		t.Errorf("products after an empty export: %d, want 1", n)
	}
}

func TestSyncerConcurrent(t *testing.T) {
	_, syncer, _ := newFakeOFF(t)
	// Mark syncing by hand → the second call must fail with ErrSyncInProgress.
	syncer.mu.Lock()
	syncer.syncing = true
	syncer.mu.Unlock()
	if _, err := syncer.Sync(context.Background()); !errors.Is(err, ErrSyncInProgress) {
		t.Errorf("want ErrSyncInProgress, got %v", err)
	}
}
