package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestMigration0029_ReferenceCodes checks that 0029 gives every seeded
// allergen and activity a code and leaves the rest of the data as it was:
// a row someone added by hand keeps its name and gets no code, and tagged
// ingredients and logged workouts keep pointing at the same rows.
func TestMigration0029_ReferenceCodes(t *testing.T) {
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
	if err := goose.UpToContext(ctx, store.DB, "migrations", 28); err != nil {
		t.Fatalf("goose up-to 28: %v", err)
	}

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, email, password_hash, display_name) VALUES (1, 'mig@example.test', 'x', 'Mig')`)
	mustExec(`INSERT INTO ingredients (id, source, name, unit, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100)
	          VALUES (1, 'local', 'Pane', 'g', 250, 8, 50, 1)`)
	mustExec(`INSERT INTO allergen_tags (id, name) VALUES (100, 'zafferano')`)
	mustExec(`INSERT INTO ingredient_allergens (ingredient_id, allergen_id)
	          SELECT 1, id FROM allergen_tags WHERE name IN ('glutine', 'zafferano')`)
	mustExec(`INSERT INTO workout_activities (id, name, kcal_per_hour, sort_order) VALUES (100, 'Sci', 400, 12)`)
	mustExec(`INSERT INTO workout_logs (user_id, activity_id, logged_at, duration_minutes, kcal_burned)
	          SELECT 1, id, '2026-10-12', 30, 240 FROM workout_activities WHERE name = 'Corsa'`)

	if err := goose.UpContext(ctx, store.DB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	count := func(q string) int {
		t.Helper()
		var n int
		if err := store.DB.QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return n
	}
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM allergen_tags WHERE code IS NULL`:                                     1,
		`SELECT COUNT(*) FROM allergen_tags WHERE id = 100 AND name = 'zafferano' AND code IS NULL`: 1,
		`SELECT COUNT(DISTINCT code) FROM allergen_tags`:                                            14,
		`SELECT COUNT(*) FROM workout_activities WHERE code IS NULL`:                                1,
		`SELECT COUNT(*) FROM workout_activities WHERE id = 100 AND name = 'Sci' AND code IS NULL`:  1,
		`SELECT COUNT(DISTINCT code) FROM workout_activities`:                                       11,
		`SELECT COUNT(*) FROM ingredient_allergens WHERE ingredient_id = 1`:                         2,
		`SELECT COUNT(*) FROM workout_logs wl JOIN workout_activities wa ON wa.id = wl.activity_id
		  WHERE wa.code = 'running' AND wa.name = 'Corsa'`: 1,
	} {
		if got := count(q); got != want {
			t.Errorf("%s: got %d, want %d", q, got, want)
		}
	}

	// The repositories read the codes, and an empty one for the hand-made rows.
	acts, err := NewWorkoutLogs(store.DB).ListActivities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]string{}
	for _, a := range acts {
		codes[a.Name] = a.Code
	}
	if codes["Palestra"] != "gym" || codes["Sci"] != "" {
		t.Errorf("activity codes = %v", codes)
	}
	tags, err := NewCategories(store.DB).AllergensForIngredients(ctx, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	codes = map[string]string{}
	for _, tag := range tags[1] {
		codes[tag.Name] = tag.Code
	}
	if len(codes) != 2 || codes["glutine"] != "gluten" || codes["zafferano"] != "" {
		t.Errorf("allergen codes = %v", codes)
	}

	if err := goose.DownToContext(ctx, store.DB, "migrations", 28); err != nil {
		t.Fatalf("goose down-to 28: %v", err)
	}
	if got := count(`SELECT COUNT(*) FROM ingredient_allergens WHERE ingredient_id = 1`); got != 2 {
		t.Errorf("after down: %d allergens on the ingredient, want 2", got)
	}
}
