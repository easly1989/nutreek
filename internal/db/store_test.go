package db

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestOpenAndMigrate checks that the DB is created on the filesystem, the
// pragmas are on and every embedded migration applies.
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
		t.Errorf("foreign_keys want 1, got %d", fk)
	}

	var journal string
	if err := store.DB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("pragma journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode want wal, got %s", journal)
	}

	// The main tables must exist.
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
			t.Errorf("table %q not found after migrate: %v", table, err)
		}
	}

	// Rerunning Migrate must be idempotent.
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate secondario: %v", err)
	}
}

// TestConcurrentReadThenWriteTx reproduces the repositories' transaction
// pattern (read, then write) from two goroutines. With a deferred BEGIN both
// take the read snapshot and the second to write gets SQLITE_BUSY without
// waiting for busy_timeout; with _txlock=immediate the second waits for the
// first to commit and then proceeds.
func TestConcurrentReadThenWriteTx(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tx.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.DB.ExecContext(ctx, `CREATE TABLE counter (n INTEGER)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	var bothRead sync.WaitGroup
	bothRead.Add(2)
	barrier := make(chan struct{})
	go func() { bothRead.Wait(); close(barrier) }()
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- func() error {
				tx, err := store.DB.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback() }()
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM counter`).Scan(&n); err != nil {
					return err
				}
				// Wait for the other transaction to have read too. With
				// BEGIN IMMEDIATE the other one is blocked in BeginTx: the
				// timeout avoids waiting for it forever.
				bothRead.Done()
				select {
				case <-barrier:
				case <-time.After(500 * time.Millisecond):
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO counter (n) VALUES (?)`, n+1); err != nil {
					return err
				}
				return tx.Commit()
			}()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent transaction failed: %v", err)
		}
	}
}
