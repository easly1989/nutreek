package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestMigration0030_UserLocale checks that users who signed up before 0030
// stay in Italian, while new ones follow the browser ("").
func TestMigration0030_UserLocale(t *testing.T) {
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
	if err := goose.UpToContext(ctx, store.DB, "migrations", 29); err != nil {
		t.Fatalf("goose up-to 29: %v", err)
	}
	if _, err := store.DB.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ('old@example.test', 'x', 'Old')`); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, store.DB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	users := NewUsers(store.DB)
	old, err := users.GetByEmail(ctx, "old@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if old.Locale != "it" {
		t.Errorf("existing user: locale = %q, want it", old.Locale)
	}
	id, err := users.Create(ctx, "new@example.test", "x", "New")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := users.GetByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Locale != "" {
		t.Errorf("new user: locale = %q, want empty", fresh.Locale)
	}
	if err := users.SetLocale(ctx, id, "en"); err != nil {
		t.Fatal(err)
	}
	if fresh, _ = users.GetByID(ctx, id); fresh.Locale != "en" {
		t.Errorf("after SetLocale: locale = %q, want en", fresh.Locale)
	}

	if err := goose.DownToContext(ctx, store.DB, "migrations", 29); err != nil {
		t.Fatalf("goose down-to 29: %v", err)
	}
}
