package off

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The OpenFoodFacts files the Syncer reads (#211).
const (
	// DefaultCSVURL is the full CSV export: ~0.8 GB compressed, ~4M products,
	// regenerated daily. A bootstrap streams it and keeps the ~250k Italian ones.
	DefaultCSVURL = "https://static.openfoodfacts.org/data/en.openfoodfacts.org.products.csv.gz"
	// DefaultDeltaURL is the folder of index.txt and of the daily deltas it
	// lists (the last ~13 days, 20-80 MB each, the dump's JSONL shape).
	DefaultDeltaURL = "https://static.openfoodfacts.org/data/delta/"
)

// italyTag is OFF's canonical tag for the products sold in Italy.
// countries_tags is an array of normalized strings (en:<country>).
const italyTag = "en:italy"

// dumpBatchSize is the number of products accumulated for every INSERT
// transaction. 1000 is a good compromise: a short lock, high throughput.
const dumpBatchSize = 1000

// scannerBufferMax is the largest JSONL line the Scanner accepts. Some
// products have very long descriptions / ingredients, up to hundreds of KB.
// 4MB is wide enough not to discard anything.
const scannerBufferMax = 4 * 1024 * 1024

// bootstrapMaxAge is how long the deltas alone keep the data: they never list
// the products OpenFoodFacts deleted, so a bootstrap this old is redone.
const bootstrapMaxAge = 30 * 24 * time.Hour

// ParserVersion is the version of the parser, stored in off_sync_state by
// every bootstrap. Bump it whenever a change to the parser must reach the
// data already imported: the next sync bootstraps from the CSV again.
// 1 = the dump's nutrition/images shape (#204), 2 = the CSV export (#211).
const ParserVersion = 2

// ErrSyncInProgress is returned by Sync when another sync is already
// running (the mutex prevents concurrency, from the admin endpoint too).
var ErrSyncInProgress = errors.New("sync OFF gia' in corso")

// SyncStats summarizes the outcome of a Sync.
type SyncStats struct {
	Bootstrap    bool // true when the CSV export was imported
	Deltas       int  // delta files applied
	ScannedTotal int  // CSV rows and delta lines read (any country)
	Imported     int  // Italian products written
	Removed      int  // local products a delta no longer lists as Italian
	DurationMs   int64
}

// Syncer keeps off_products in step with OpenFoodFacts: a bootstrap from the
// CSV export, then the daily deltas. Meant to run in the background (a daily
// check + a manual admin trigger). It uses a mutex to guarantee only ONE sync
// in flight at a time.
type Syncer struct {
	DB       *sql.DB
	CSVURL   string
	DeltaURL string // the folder of index.txt and of the delta files
	HTTP     *http.Client
	Logger   *slog.Logger

	mu      sync.Mutex
	syncing bool
}

// NewSyncer builds a Syncer with sane defaults. A long HTTP timeout because
// downloading the CSV export can take 5-15 minutes.
func NewSyncer(db *sql.DB, csvURL, deltaURL string, logger *slog.Logger) *Syncer {
	if csvURL == "" {
		csvURL = DefaultCSVURL
	}
	if deltaURL == "" {
		deltaURL = DefaultDeltaURL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{
		DB:       db,
		CSVURL:   csvURL,
		DeltaURL: deltaURL,
		HTTP:     &http.Client{Timeout: 30 * time.Minute},
		Logger:   logger,
	}
}

// IsSyncing returns true when a sync is currently running. Used by the admin
// endpoint to return 409 without queueing requests.
func (s *Syncer) IsSyncing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncing
}

// Sync brings off_products up to date: a CSV bootstrap when one is due (see
// bootstrapReason), then every delta newer than the high-water mark, oldest
// first. On error it updates last_error in off_sync_state; a failed bootstrap
// leaves the previous products in place.
func (s *Syncer) Sync(ctx context.Context) (SyncStats, error) { return s.run(ctx, false) }

// Bootstrap is Sync with a CSV bootstrap even when none is due.
func (s *Syncer) Bootstrap(ctx context.Context) (SyncStats, error) { return s.run(ctx, true) }

func (s *Syncer) run(ctx context.Context, force bool) (SyncStats, error) {
	s.mu.Lock()
	if s.syncing {
		s.mu.Unlock()
		return SyncStats{}, ErrSyncInProgress
	}
	s.syncing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.syncing = false
		s.mu.Unlock()
	}()

	start := time.Now()
	s.markAttempt(ctx)

	stats, err := s.doSync(ctx, force)
	stats.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		s.markError(ctx, err)
		return stats, err
	}
	s.markSuccess(ctx)
	return stats, nil
}

func (s *Syncer) doSync(ctx context.Context, force bool) (SyncStats, error) {
	var stats SyncStats
	st, err := s.LoadState(ctx)
	if err != nil {
		return stats, fmt.Errorf("load state: %w", err)
	}
	reason := bootstrapReason(st, force)
	var pending []delta
	if reason == "" {
		index, err := s.fetchIndex(ctx)
		if err != nil {
			return stats, err
		}
		var ok bool
		if pending, ok = pendingDeltas(index, st.HighWater.Unix()); !ok {
			reason = "the deltas no longer reach the high-water mark"
		}
	}
	if reason != "" {
		s.Logger.Info("off sync: bootstrap from the CSV export", "reason", reason, "url", s.CSVURL)
		highWater, err := s.bootstrap(ctx, &stats)
		if err != nil {
			return stats, err
		}
		index, err := s.fetchIndex(ctx)
		if err != nil {
			return stats, err
		}
		var ok bool
		if pending, ok = pendingDeltas(index, highWater); !ok {
			// Bootstrapping again would not help: move the high-water mark on.
			s.Logger.Warn("off sync: the CSV export is older than every delta; the changes in between "+
				"are missing until the next bootstrap", "high_water", highWater)
			pending, _ = pendingDeltas(index, oldestFrom(index))
		}
	}
	for _, d := range pending {
		if err := s.applyDelta(ctx, d, &stats); err != nil {
			return stats, fmt.Errorf("delta %s: %w", d.name, err)
		}
		stats.Deltas++
	}
	s.Logger.Info("off sync: done", "bootstrap", stats.Bootstrap, "deltas", stats.Deltas,
		"imported", stats.Imported, "removed", stats.Removed, "scanned", stats.ScannedTotal)
	return stats, nil
}

// bootstrapReason says why the next sync must start from the CSV export, ""
// when the deltas are enough (the index can still find a gap).
func bootstrapReason(st *State, force bool) string {
	switch {
	case force:
		return "forced"
	case st.HighWater == nil:
		return "no bootstrap yet"
	case st.ParserVersion != ParserVersion:
		return "parser version changed"
	case st.LastBootstrapAt == nil || time.Since(*st.LastBootstrapAt) > bootstrapMaxAge:
		return "the last bootstrap is older than 30 days"
	}
	return ""
}

// get GETs url and fails on a status other than 2xx.
func (s *Syncer) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	res, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		_ = res.Body.Close()
		return nil, fmt.Errorf("download %s: status %d: %s", url, res.StatusCode, strings.TrimSpace(string(body)))
	}
	return res, nil
}

// openGzip downloads a .gz file and returns it decompressed, logging the MB
// downloaded every 50MB: without it nothing shows while the ~0.8 GB export
// downloads (5-30 min depending on bandwidth).
func (s *Syncer) openGzip(ctx context.Context, url string) (io.ReadCloser, error) {
	res, err := s.get(ctx, url)
	if err != nil {
		return nil, err
	}
	s.Logger.Info("off sync: downloading", "url", url, "content_length_bytes", res.ContentLength)
	gz, err := gzip.NewReader(&progressReader{
		R:         res.Body,
		Total:     res.ContentLength,
		Logger:    s.Logger,
		LogEvery:  50 * 1024 * 1024,
		LogPrefix: "off sync: download",
	})
	if err != nil {
		_ = res.Body.Close()
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	return gzipBody{gz, res.Body}, nil
}

type gzipBody struct {
	*gzip.Reader
	body io.Closer
}

func (g gzipBody) Close() error {
	_ = g.Reader.Close()
	return g.body.Close()
}

// flushBatch writes a batch into table (off_products or its staging copy) in
// a single transaction, with prepared statements. A product without a name
// is one a delta no longer lists as Italian: it is deleted. It returns the
// rows deleted.
func (s *Syncer) flushBatch(ctx context.Context, table string, batch []Product, ts int64) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ins, err := tx.PrepareContext(ctx, `
		INSERT INTO `+table+` (
			barcode, product_name, brands, quantity,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			has_full_nutrients, image_url, serving_g, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(barcode) DO UPDATE SET
			product_name = excluded.product_name,
			brands = excluded.brands,
			quantity = excluded.quantity,
			kcal_per_100 = excluded.kcal_per_100,
			protein_per_100 = excluded.protein_per_100,
			carbs_per_100 = excluded.carbs_per_100,
			fat_per_100 = excluded.fat_per_100,
			has_full_nutrients = excluded.has_full_nutrients,
			image_url = excluded.image_url,
			serving_g = excluded.serving_g,
			updated_at = excluded.updated_at`)
	if err != nil {
		return 0, fmt.Errorf("prepare insert: %w", err)
	}
	defer ins.Close()
	del, err := tx.PrepareContext(ctx, `DELETE FROM `+table+` WHERE barcode = ?`)
	if err != nil {
		return 0, fmt.Errorf("prepare delete: %w", err)
	}
	defer del.Close()

	removed := 0
	for _, p := range batch {
		if p.Name == "" {
			res, err := del.ExecContext(ctx, p.Code)
			if err != nil {
				return 0, fmt.Errorf("delete %s: %w", p.Code, err)
			}
			n, _ := res.RowsAffected()
			removed += int(n)
			continue
		}
		hasFull := 0
		if p.HasFullNutrients {
			hasFull = 1
		}
		if _, err := ins.ExecContext(ctx,
			p.Code, p.Name, p.Brand, p.Quantity,
			p.Kcal, p.Protein, p.Carbs, p.Fat,
			hasFull, ThumbURL(p.ImageURL), p.ServingG, ts,
		); err != nil {
			return 0, fmt.Errorf("insert %s: %w", p.Code, err)
		}
	}
	return removed, tx.Commit()
}

// dumpProduct is the subset of fields of a dump/delta line we care about.
// Same schema as rawProduct plus countries_tags for the country filter.
type dumpProduct struct {
	Code                string                     `json:"code"`
	ProductName         string                     `json:"product_name"`
	Brands              string                     `json:"brands"`
	Quantity            string                     `json:"quantity"`
	CountriesTags       []string                   `json:"countries_tags"`
	Nutriments          map[string]json.RawMessage `json:"nutriments"`
	ImageFrontSmallURL  string                     `json:"image_front_small_url"`
	ImageURL            string                     `json:"image_url"`
	ServingQuantity     json.RawMessage            `json:"serving_quantity"`
	ServingQuantityUnit string                     `json:"serving_quantity_unit"`
	ServingSize         string                     `json:"serving_size"`
	NutritionDataPer    string                     `json:"nutrition_data_per"`
	Nutrition           json.RawMessage            `json:"nutrition"`
	Images              json.RawMessage            `json:"images"`
}

// toProduct reuses the logic of rawProduct.toProduct() by building an
// equivalent rawProduct. It avoids duplication: the nutrient parsing is
// identical for the dump and for the live API response.
func (d dumpProduct) toProduct() Product {
	raw := rawProduct{
		Code:                d.Code,
		ProductName:         d.ProductName,
		Brands:              d.Brands,
		Quantity:            d.Quantity,
		Nutriments:          d.Nutriments,
		ImageFrontSmallURL:  d.ImageFrontSmallURL,
		ImageURL:            d.ImageURL,
		ServingQuantity:     d.ServingQuantity,
		ServingQuantityUnit: d.ServingQuantityUnit,
		ServingSize:         d.ServingSize,
		NutritionDataPer:    d.NutritionDataPer,
		Nutrition:           d.Nutrition,
		Images:              d.Images,
	}
	return raw.toProduct()
}

// scanJSONL calls fn for every product of a JSONL stream. A malformed line is
// skipped silently: the files are best effort, and logging every error would
// clog the logs. It returns the lines read.
func scanJSONL(ctx context.Context, r io.Reader, fn func(dumpProduct) error) (int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), scannerBufferMax)
	n := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		n++
		var d dumpProduct
		if json.Unmarshal(scanner.Bytes(), &d) != nil {
			continue
		}
		if err := fn(d); err != nil {
			return n, err
		}
	}
	if err := scanner.Err(); err != nil {
		return n, fmt.Errorf("scan: %w", err)
	}
	return n, nil
}

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

// progressReader logs every LogEvery bytes read from the underlying stream.
// Used to give feedback while a large file downloads (5-30 min of plain
// Reads with no visible events). Total = ContentLength when known, 0 or -1
// when unknown (chunked transfer).
type progressReader struct {
	R         io.Reader
	Total     int64
	Logger    *slog.Logger
	LogEvery  int64
	LogPrefix string

	read    int64
	nextLog int64
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.R.Read(buf)
	p.read += int64(n)
	if p.nextLog == 0 {
		p.nextLog = p.LogEvery
	}
	if p.read >= p.nextLog {
		if p.Total > 0 {
			pct := float64(p.read) / float64(p.Total) * 100
			p.Logger.Info(p.LogPrefix,
				"read_mb", p.read/(1024*1024),
				"total_mb", p.Total/(1024*1024),
				"pct", fmt.Sprintf("%.1f", pct))
		} else {
			p.Logger.Info(p.LogPrefix, "read_mb", p.read/(1024*1024))
		}
		p.nextLog += p.LogEvery
	}
	return n, err
}

// ---- persisted sync state ---------------------------------------------------

func (s *Syncer) markAttempt(ctx context.Context) {
	_, err := s.DB.ExecContext(ctx, `UPDATE off_sync_state SET last_attempt_at = ? WHERE id = 1`, time.Now().Unix())
	if err != nil {
		s.Logger.Warn("off sync: markAttempt failed", "err", err)
	}
}

func (s *Syncer) markSuccess(ctx context.Context) {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE off_sync_state
		SET last_synced_at = ?, last_error = NULL, products_count = (SELECT COUNT(*) FROM off_products)
		WHERE id = 1`, time.Now().Unix())
	if err != nil {
		s.Logger.Warn("off sync: markSuccess failed", "err", err)
	}
}

func (s *Syncer) markError(ctx context.Context, syncErr error) {
	_, err := s.DB.ExecContext(ctx, `UPDATE off_sync_state SET last_error = ? WHERE id = 1`, syncErr.Error())
	if err != nil {
		s.Logger.Warn("off sync: markError failed", "err", err)
	}
}

// State is the representation of off_sync_state used by the admin endpoint.
type State struct {
	LastSyncedAt    *time.Time
	LastAttemptAt   *time.Time
	LastError       string
	ProductsCount   int
	ParserVersion   int        // the parser version of the last bootstrap
	LastBootstrapAt *time.Time // the last CSV import
	LastDeltaAt     *time.Time // the last delta applied
	HighWater       *time.Time // the data is up to date to here
	Syncing         bool
}

// LoadState reads the current state from the DB. An error only on real
// problems (the singleton row is created by the migration).
func (s *Syncer) LoadState(ctx context.Context) (*State, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT last_synced_at, last_attempt_at, last_error, products_count, parser_version,
			last_bootstrap_at, last_delta_at, high_water
		FROM off_sync_state WHERE id = 1`)
	var (
		synced, attempt, bootstrap, delta, high sql.NullInt64
		errMsg                                  sql.NullString
		count, version                          int
	)
	if err := row.Scan(&synced, &attempt, &errMsg, &count, &version, &bootstrap, &delta, &high); err != nil {
		return nil, err
	}
	at := func(v sql.NullInt64) *time.Time {
		if !v.Valid {
			return nil
		}
		t := time.Unix(v.Int64, 0)
		return &t
	}
	return &State{
		LastSyncedAt:    at(synced),
		LastAttemptAt:   at(attempt),
		LastError:       errMsg.String,
		ProductsCount:   count,
		ParserVersion:   version,
		LastBootstrapAt: at(bootstrap),
		LastDeltaAt:     at(delta),
		HighWater:       at(high),
		Syncing:         s.IsSyncing(),
	}, nil
}
