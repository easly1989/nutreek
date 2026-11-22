package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// Migration 0043 marks the OpenFoodFacts imports that still match their
// product as aligned and leaves the others, and the hand-made ones, off.
func TestMigration0043_OFFAligned(t *testing.T) {
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
	if err := goose.UpToContext(ctx, store.DB, "migrations", 42); err != nil {
		t.Fatalf("goose up-to 42: %v", err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	product := func(code string, kcal float64) {
		exec(`INSERT INTO off_products (barcode, product_name, brands, quantity, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients, updated_at)
			VALUES (?, 'P', '', '', ?, 9, 4, 5, 1, 0)`, code, kcal)
	}
	ingredient := func(source, code string, kcal float64) {
		var bc any
		if code != "" {
			bc = code
		}
		exec(`INSERT INTO ingredients (source, off_barcode, name, unit, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100) VALUES (?, ?, ?, 'g', ?, 9, 4, 5)`,
			source, bc, source+code, kcal)
	}
	product("1", 97)
	product("2", 97)
	ingredient("openfoodfacts", "1", 97)  // still the product
	ingredient("openfoodfacts", "2", 120) // edited since
	ingredient("openfoodfacts", "3", 97)  // the product is gone from off_products
	ingredient("local", "", 97)           // made by hand

	if err := goose.UpContext(ctx, store.DB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	got := map[string]int{}
	rows, err := store.DB.QueryContext(ctx, `SELECT name, off_aligned FROM ingredients`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var a int
		if err := rows.Scan(&name, &a); err != nil {
			t.Fatal(err)
		}
		got[name] = a
	}
	want := map[string]int{"openfoodfacts1": 1, "openfoodfacts2": 0, "openfoodfacts3": 0, "local": 0}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: off_aligned = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
	if err := goose.DownToContext(ctx, store.DB, "migrations", 42); err != nil {
		t.Fatalf("goose down: %v", err)
	}
}
