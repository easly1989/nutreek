package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// Migration 0042 puts the package size in the OFF full-text index: products
// already in off_products before it are found by size afterwards, and going
// back down keeps the name search working.
func TestMigration0042_OFFIndexGainsSize(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mig.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.UpToContext(ctx, store.DB, "migrations", 41); err != nil {
		t.Fatalf("goose up-to 41: %v", err)
	}
	insert := func(code, name, brand, qty string) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, `
			INSERT INTO off_products (barcode, product_name, brands, quantity, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, updated_at)
			VALUES (?, ?, ?, ?, 97, 9, 4, 5, 1, 0)`, code, name, brand, qty); err != nil {
			t.Fatal(err)
		}
	}
	insert("1", "Yogurt greco", "Fage", "150 g")
	insert("2", "Yogurt bianco", "Parmalat", "125 g")
	count := func(match string) int {
		t.Helper()
		var n int
		if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM off_products_fts WHERE off_products_fts MATCH ?`, match).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`"150"*`); n != 0 {
		t.Fatalf("before 0042 the size is not indexed, got %d", n)
	}

	if err := goose.UpContext(ctx, store.DB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	if n := count(`"fage"* "150"*`); n != 1 {
		t.Errorf("after 0042: brand and size found %d, want 1", n)
	}
	if n := count(`"yogurt"*`); n != 2 {
		t.Errorf("after 0042: name found %d, want 2", n)
	}
	// The triggers of the new index keep it in step.
	insert("3", "Yogurt magro", "Fage", "500 g")
	if _, err := store.DB.ExecContext(ctx, `UPDATE off_products SET quantity = '170 g' WHERE barcode = '1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `DELETE FROM off_products WHERE barcode = '2'`); err != nil {
		t.Fatal(err)
	}
	if n := count(`"500"*`); n != 1 {
		t.Errorf("a product added after 0042: %d, want 1", n)
	}
	if n := count(`"150"*`); n != 0 {
		t.Errorf("an updated size must leave the old one: %d", n)
	}
	if n := count(`"170"*`); n != 1 {
		t.Errorf("an updated size must be found: %d", n)
	}
	if n := count(`"parmalat"*`); n != 0 {
		t.Errorf("a deleted product must leave the index: %d", n)
	}

	if err := goose.DownToContext(ctx, store.DB, "migrations", 41); err != nil {
		t.Fatalf("goose down: %v", err)
	}
	if n := count(`"fage"*`); n != 2 {
		t.Errorf("after the down migration the name search found %d, want 2", n)
	}
}
