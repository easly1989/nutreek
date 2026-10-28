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

// DefaultDumpURL is the URL of the full OpenFoodFacts JSONL.gz dump. ~0.9GB
// compressed, ~9-15GB decompressed, ~3.5M products in total. We stream it
// line by line and filter down to the ~250k Italian ones.
const DefaultDumpURL = "https://static.openfoodfacts.org/data/openfoodfacts-products.jsonl.gz"

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

// ParserVersion is the version of the dump parser, stored in off_sync_state
// after every import. Bump it whenever a change to the parser must reach the
// data already imported: the next sync downloads the dump again even when its
// ETag is unchanged. 1 = the dump's nutrition/images shape (#204).
const ParserVersion = 1

// ErrSyncInProgress is returned by SyncDump when another sync is already
// running (the mutex prevents concurrency, from the admin endpoint too).
var ErrSyncInProgress = errors.New("sync OFF gia' in corso")

// SyncStats summarizes the outcome of a SyncDump.
type SyncStats struct {
	ScannedTotal int  // lines processed (any country)
	Imported     int  // prodotti italiani inseriti
	Skipped      bool // true when the ETag is unchanged → no download
	ETag         string
	DurationMs   int64
}

// Syncer downloads the public OFF dump and fills off_products. Meant to run
// in the background (a weekly cron + a manual admin trigger). It uses a
// mutex to guarantee only ONE sync in flight at a time.
type Syncer struct {
	DB     *sql.DB
	URL    string
	HTTP   *http.Client
	Logger *slog.Logger

	mu      sync.Mutex
	syncing bool
}

// NewSyncer builds a Syncer with sane defaults. A long HTTP timeout because
// downloading the full dump can take 5-15 minutes.
func NewSyncer(db *sql.DB, url string, logger *slog.Logger) *Syncer {
	if url == "" {
		url = DefaultDumpURL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{
		DB:     db,
		URL:    url,
		HTTP:   &http.Client{Timeout: 30 * time.Minute},
		Logger: logger,
	}
}

// IsSyncing returns true when a sync is currently running. Used by the admin
// endpoint to return 409 without queueing requests.
func (s *Syncer) IsSyncing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncing
}

// SyncDump downloads the JSONL.gz dump, stream-parses its products, filters
// the Italian ones and refills off_products. Idempotent: at the end of the
// import the table reflects exactly the dump OFF released.
//
// It uses If-None-Match with the saved ETag to skip the download when the
// server has not updated. On error it updates last_error in off_sync_state
// but does NOT delete the existing data.
func (s *Syncer) SyncDump(ctx context.Context) (SyncStats, error) {
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

	stats, err := s.doSync(ctx)
	stats.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		s.markError(ctx, err)
		return stats, err
	}
	s.markSuccess(ctx, stats)
	return stats, nil
}

func (s *Syncer) doSync(ctx context.Context) (SyncStats, error) {
	previousETag, version, _ := s.readLastETag(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return SyncStats{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	// Data imported by another parser version is read again from scratch.
	if previousETag != "" && version == ParserVersion {
		req.Header.Set("If-None-Match", previousETag)
	}

	s.Logger.Info("off sync: starting download", "url", s.URL, "prev_etag", previousETag)
	res, err := s.HTTP.Do(req)
	if err != nil {
		return SyncStats{}, fmt.Errorf("download dump: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotModified {
		s.Logger.Info("off sync: dump not modified (304), skip")
		return SyncStats{Skipped: true, ETag: previousETag}, nil
	}
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return SyncStats{}, fmt.Errorf("dump status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	contentLen := res.ContentLength
	s.Logger.Info("off sync: response received, starting decompress+ingest",
		"content_length_bytes", contentLen, "etag", res.Header.Get("ETag"))

	// Wrap the body in a progressReader so we log the MB downloaded every
	// 50MB. Without it the user sees no activity while the ~900MB compressed
	// download runs (5-30 min depending on bandwidth).
	body := &progressReader{
		R:         res.Body,
		Total:     contentLen,
		Logger:    s.Logger,
		LogEvery:  50 * 1024 * 1024,
		LogPrefix: "off sync: download",
	}

	gz, err := gzip.NewReader(body)
	if err != nil {
		return SyncStats{}, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	newETag := res.Header.Get("ETag")
	stats, err := s.ingest(ctx, gz, newETag)
	if err != nil {
		return stats, err
	}
	stats.ETag = newETag
	return stats, nil
}

// ingest reads the decompressed JSONL stream, filters by country and batches
// the INSERTs in transactions of dumpBatchSize. It deletes off_products in a
// separate transaction BEFORE starting, so the new data fully replaces the
// old (no orphans from a previous dump).
func (s *Syncer) ingest(ctx context.Context, r io.Reader, newETag string) (SyncStats, error) {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM off_products`); err != nil {
		return SyncStats{}, fmt.Errorf("truncate off_products: %w", err)
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), scannerBufferMax)

	stats := SyncStats{}
	batch := make([]Product, 0, dumpBatchSize)
	now := time.Now().Unix()

	lastLogScanned := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.ScannedTotal++

		// Log progress every 100k scanned lines (even when they are not
		// Italian): the user sees the parser working even before enough IT
		// products are imported to trigger the log on Imported.
		if stats.ScannedTotal-lastLogScanned >= 100000 {
			s.Logger.Info("off sync: parsing",
				"scanned", stats.ScannedTotal, "imported_it", stats.Imported)
			lastLogScanned = stats.ScannedTotal
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var d dumpProduct
		if err := json.Unmarshal(line, &d); err != nil {
			// Malformed line: skip it silently, the full dump is best effort.
			// Logging every error would clog the logs.
			continue
		}
		if !containsTag(d.CountriesTags, italyTag) {
			continue
		}
		p := d.toProduct()
		if p.Code == "" || p.Name == "" {
			continue
		}
		batch = append(batch, p)

		if len(batch) >= dumpBatchSize {
			if err := s.flushBatch(ctx, batch, now); err != nil {
				return stats, err
			}
			stats.Imported += len(batch)
			batch = batch[:0]
		}
	}
	if err := scanner.Err(); err != nil {
		return stats, fmt.Errorf("scan dump: %w", err)
	}
	if len(batch) > 0 {
		if err := s.flushBatch(ctx, batch, now); err != nil {
			return stats, err
		}
		stats.Imported += len(batch)
	}
	s.Logger.Info("off sync: ingest done",
		"imported", stats.Imported, "scanned", stats.ScannedTotal, "new_etag", newETag)
	return stats, nil
}

// flushBatch runs a batch INSERT in a single transaction. It uses a prepared
// statement to avoid the parsing overhead on every row.
func (s *Syncer) flushBatch(ctx context.Context, batch []Product, ts int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO off_products (
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
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range batch {
		hasFull := 0
		if p.HasFullNutrients {
			hasFull = 1
		}
		if _, err := stmt.ExecContext(ctx,
			p.Code, p.Name, p.Brand, p.Quantity,
			p.Kcal, p.Protein, p.Carbs, p.Fat,
			hasFull, p.ImageURL, p.ServingG, ts,
		); err != nil {
			return fmt.Errorf("insert %s: %w", p.Code, err)
		}
	}
	return tx.Commit()
}

// dumpProduct is the subset of fields of the OFF dump we care about. Same
// schema as rawProduct plus countries_tags for the country filter.
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

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}

// progressReader logs every LogEvery bytes read from the underlying stream.
// Used to give feedback while the compressed dump downloads (5-30 min of
// plain Reads with no visible events). Total = ContentLength when known, 0
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

// ---- stato sync persistito --------------------------------------------------

// readLastETag returns the ETag of the last imported dump and the parser
// version that imported it.
func (s *Syncer) readLastETag(ctx context.Context) (string, int, error) {
	var etag sql.NullString
	var version int
	err := s.DB.QueryRowContext(ctx, `SELECT dump_etag, parser_version FROM off_sync_state WHERE id = 1`).Scan(&etag, &version)
	if err != nil {
		return "", 0, err
	}
	return etag.String, version, nil
}

func (s *Syncer) markAttempt(ctx context.Context) {
	_, err := s.DB.ExecContext(ctx, `UPDATE off_sync_state SET last_attempt_at = ? WHERE id = 1`, time.Now().Unix())
	if err != nil {
		s.Logger.Warn("off sync: markAttempt failed", "err", err)
	}
}

func (s *Syncer) markSuccess(ctx context.Context, stats SyncStats) {
	count := stats.Imported
	if stats.Skipped {
		// We do not update the count: we would reflect the current state.
		row := s.DB.QueryRowContext(ctx, `SELECT products_count FROM off_sync_state WHERE id = 1`)
		_ = row.Scan(&count)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE off_sync_state
		SET last_synced_at = ?, last_error = NULL, products_count = ?, dump_etag = ?, parser_version = ?
		WHERE id = 1`,
		time.Now().Unix(), count, stats.ETag, ParserVersion,
	)
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
	LastSyncedAt  *time.Time
	LastAttemptAt *time.Time
	LastError     string
	ProductsCount int
	DumpETag      string
	ParserVersion int // the parser version of the last import
	Syncing       bool
}

// LoadState reads the current state from the DB. An error only on real
// problems (the singleton row is created by the migration).
func (s *Syncer) LoadState(ctx context.Context) (*State, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT last_synced_at, last_attempt_at, last_error, products_count, dump_etag, parser_version
		FROM off_sync_state WHERE id = 1`)
	var (
		synced, attempt sql.NullInt64
		errMsg, etag    sql.NullString
		count, version  int
	)
	if err := row.Scan(&synced, &attempt, &errMsg, &count, &etag, &version); err != nil {
		return nil, err
	}
	st := &State{
		LastError:     errMsg.String,
		ProductsCount: count,
		DumpETag:      etag.String,
		ParserVersion: version,
		Syncing:       s.IsSyncing(),
	}
	if synced.Valid {
		t := time.Unix(synced.Int64, 0)
		st.LastSyncedAt = &t
	}
	if attempt.Valid {
		t := time.Unix(attempt.Int64, 0)
		st.LastAttemptAt = &t
	}
	return st, nil
}
