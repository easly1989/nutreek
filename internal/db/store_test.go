package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestOpenAndMigrate verifica che il DB venga creato sul filesystem,
// i pragmas siano attivi e tutte le migrazioni embedded applicabili.
func TestOpenAndMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Pragmas sanity check.
	var fk int
	if err := store.DB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("pragma foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys atteso 1, got %d", fk)
	}

	var journal string
	if err := store.DB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("pragma journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode atteso wal, got %s", journal)
	}

	// Le tabelle principali devono esistere.
	tables := []string{
		"users", "ingredients", "recipes", "recipe_ingredients",
		"meal_templates", "meal_template_slots",
		"weekly_plans", "meals", "meal_items",
	}
	for _, table := range tables {
		var name string
		err := store.DB.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("tabella %q non trovata dopo migrate: %v", table, err)
		}
	}

	// Re-run Migrate deve essere idempotente.
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate secondario: %v", err)
	}
}
