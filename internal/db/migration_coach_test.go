package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestMigration0025_PreservaDatiNutrizionista checks that rebuilding the
// nutritionist_* chain in migration 0025 keeps the existing data.
// This is the critical point: with FKs on, dropping the parent without the
// right sequence would cascade-delete meal types, variants and items.
func TestMigration0025_PreservaDatiNutrizionista(t *testing.T) {
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
	// Migrate up to 0024 (before coach mode).
	if err := goose.UpToContext(ctx, store.DB, "migrations", 24); err != nil {
		t.Fatalf("goose up-to 24: %v", err)
	}

	// Fill the chain with the pre-0025 schema.
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, email, password_hash, display_name) VALUES (1, 'mig@test.it', 'x', 'Mig')`)
	mustExec(`INSERT INTO ingredients (id, source, name, unit, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100)
	          VALUES (1, 'local', 'Riso', 'g', 350, 7, 78, 1)`)
	mustExec(`INSERT INTO nutritionist_plans (id, user_id, name) VALUES (10, 1, 'Piano Mig')`)
	mustExec(`INSERT INTO nutritionist_meal_types (id, plan_id, name) VALUES (20, 10, 'Pranzo')`)
	mustExec(`INSERT INTO nutritionist_meal_variations (id, meal_type_id, name) VALUES (30, 20, 'Base')`)
	mustExec(`INSERT INTO nutritionist_meal_items (id, variation_id, ingredient_id, quantity) VALUES (40, 30, 1, 100)`)

	// Apply 0025 (and later).
	if err := goose.UpContext(ctx, store.DB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	// The chain must be intact and the migrated plan active.
	var n int
	check := func(q string, want int) {
		t.Helper()
		if err := store.DB.QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		if n != want {
			t.Errorf("%s: want %d, got %d", q, want, n)
		}
	}
	check(`SELECT COUNT(*) FROM nutritionist_plans WHERE id = 10 AND user_id = 1 AND is_active = 1`, 1)
	check(`SELECT COUNT(*) FROM nutritionist_meal_types WHERE id = 20 AND plan_id = 10`, 1)
	check(`SELECT COUNT(*) FROM nutritionist_meal_variations WHERE id = 30 AND meal_type_id = 20`, 1)
	check(`SELECT COUNT(*) FROM nutritionist_meal_items WHERE id = 40 AND variation_id = 30`, 1)
	check(`SELECT COUNT(*) FROM users WHERE id = 1 AND role = 'user'`, 1)

	// The UNIQUE constraint on user_id must be gone: a second plan is fine.
	mustExec(`INSERT INTO nutritionist_plans (user_id, name, is_active) VALUES (1, 'Secondo', 0)`)
	// But a second ACTIVE plan must violate the partial index.
	if _, err := store.DB.ExecContext(ctx,
		`INSERT INTO nutritionist_plans (user_id, name, is_active) VALUES (1, 'Terzo', 1)`); err == nil {
		t.Error("two active plans for the same user must violate uniq_nutritionist_plans_active")
	}
}
