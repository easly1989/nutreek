package off

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// csvNutrients are the CSV export's per-100 g columns the parser reads, under
// the keys rawProduct.toProduct looks up in the API's nutriments.
var csvNutrients = []string{
	"energy-kcal_100g", "energy_100g", "proteins_100g", "carbohydrates_100g", "fat_100g",
	"fiber_100g", "sugars_100g", "saturated-fat_100g", "sodium_100g",
}

// csvColumns are the other columns of the CSV export the bootstrap reads.
var csvColumns = []string{
	"code", "last_modified_t", "product_name", "brands", "quantity", "countries_tags",
	"serving_quantity", "serving_size", "image_small_url",
}

// bootstrap imports the CSV export into off_products_staging, then swaps it
// into off_products. It returns the high-water mark: the newest
// last_modified_t of the export, any country.
func (s *Syncer) bootstrap(ctx context.Context, stats *SyncStats) (int64, error) {
	stats.Bootstrap = true
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM off_products_staging`); err != nil {
		return 0, fmt.Errorf("clear staging: %w", err)
	}
	body, err := s.openGzip(ctx, s.CSVURL)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	// Tab-separated, a header row; a field holding a quote is quoted, its
	// quotes doubled. encoding/csv has no line limit.
	r := csv.NewReader(body)
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		return 0, fmt.Errorf("read CSV header: %w", err)
	}
	width := len(header)
	col := make(map[string]int, width)
	for i, name := range header {
		col[name] = i
	}
	for _, name := range append(csvColumns, csvNutrients...) {
		if _, ok := col[name]; !ok {
			return 0, fmt.Errorf("CSV export: column %q missing", name)
		}
	}

	var highWater int64
	now := time.Now().Unix()
	batch := make([]Product, 0, dumpBatchSize)
	flush := func() error {
		if _, err := s.flushBatch(ctx, "off_products_staging", batch, now); err != nil {
			return err
		}
		stats.Imported += len(batch)
		batch = batch[:0]
		return nil
	}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read CSV: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		stats.ScannedTotal++
		if stats.ScannedTotal%100000 == 0 {
			s.Logger.Info("off sync: parsing", "scanned", stats.ScannedTotal, "imported_it", stats.Imported+len(batch))
		}
		if len(rec) != width {
			continue // a malformed row
		}
		if t, err := strconv.ParseInt(rec[col["last_modified_t"]], 10, 64); err == nil && t > highWater {
			highWater = t
		}
		if !containsTag(strings.Split(rec[col["countries_tags"]], ","), italyTag) {
			continue
		}
		p := csvProduct(rec, col).toProduct()
		if p.Code == "" || p.Name == "" {
			continue
		}
		batch = append(batch, p)
		if len(batch) >= dumpBatchSize {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if len(batch) > 0 {
		if err := flush(); err != nil {
			return 0, err
		}
	}
	// An export without Italian products is broken: it must not empty the table.
	if stats.Imported == 0 || highWater == 0 {
		return 0, fmt.Errorf("CSV export: no Italian product in %d rows", stats.ScannedTotal)
	}
	if err := s.swap(ctx, highWater); err != nil {
		return 0, err
	}
	s.Logger.Info("off sync: bootstrap done", "imported", stats.Imported, "scanned", stats.ScannedTotal, "high_water", highWater)
	return highWater, nil
}

// csvProduct maps a row of the CSV export to the API's shape, so the same
// rawProduct.toProduct rules decide what is importable.
func csvProduct(rec []string, col map[string]int) rawProduct {
	get := func(name string) string { return rec[col[name]] }
	nutriments := make(map[string]json.RawMessage, len(csvNutrients))
	for _, name := range csvNutrients {
		if v := csvNumber(get(name)); v != nil {
			nutriments[name] = v
		}
	}
	return rawProduct{
		Code:               get("code"),
		ProductName:        get("product_name"),
		Brands:             get("brands"),
		Quantity:           get("quantity"),
		Nutriments:         nutriments,
		ImageFrontSmallURL: get("image_small_url"),
		ServingQuantity:    csvNumber(get("serving_quantity")),
		ServingSize:        get("serving_size"),
	}
}

// csvNumber turns a CSV cell into a JSON number, nil when empty or not a
// finite number.
func csvNumber(s string) json.RawMessage {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return json.RawMessage(strconv.FormatFloat(v, 'g', -1, 64))
}

// swap moves the staging table into off_products in one transaction: the
// products gone from the export are deleted, the new and changed ones written
// (the FTS5 triggers follow), the unchanged ones left alone. With WAL the
// searches read the old products until the commit.
func (s *Syncer) swap(ctx context.Context, highWater int64) error {
	start := time.Now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin swap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM off_products WHERE barcode NOT IN (SELECT barcode FROM off_products_staging)`,
		// WHERE true: without it SQLite reads ON CONFLICT as a join constraint.
		`INSERT INTO off_products (
			barcode, product_name, brands, quantity,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			has_full_nutrients, image_url, serving_g, updated_at)
		SELECT barcode, product_name, brands, quantity,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			has_full_nutrients, image_url, serving_g, updated_at
		FROM off_products_staging WHERE true
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
			updated_at = excluded.updated_at
		WHERE (off_products.product_name, off_products.brands, off_products.quantity,
			off_products.kcal_per_100, off_products.protein_per_100, off_products.carbs_per_100,
			off_products.fat_per_100, off_products.has_full_nutrients, off_products.image_url,
			off_products.serving_g)
		IS NOT (excluded.product_name, excluded.brands, excluded.quantity,
			excluded.kcal_per_100, excluded.protein_per_100, excluded.carbs_per_100,
			excluded.fat_per_100, excluded.has_full_nutrients, excluded.image_url,
			excluded.serving_g)`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("swap: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE off_sync_state SET high_water = ?, last_bootstrap_at = ?, parser_version = ? WHERE id = 1`,
		highWater, time.Now().Unix(), ParserVersion); err != nil {
		return fmt.Errorf("swap: state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("swap: commit: %w", err)
	}
	s.Logger.Info("off sync: staging swapped in", "duration_ms", time.Since(start).Milliseconds())
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM off_products_staging`); err != nil {
		s.Logger.Warn("off sync: clear staging failed", "err", err)
	}
	return nil
}
