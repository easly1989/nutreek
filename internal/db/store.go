package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps *sql.DB and centralizes opening and migrations.
type Store struct {
	DB *sql.DB
}

// Open opens (or creates) the SQLite database at path, applies safe pragmas
// and checks the connection. The parent directory is created when missing.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir %q: %w", dir, err)
		}
	}

	dsn := buildDSN(path)

	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := sqldb.Ping(); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// WAL allows concurrent reads + one writer. Transactions start
	// IMMEDIATE (see buildDSN), so concurrent writers queue up on
	// busy_timeout instead of failing.
	return &Store{DB: sqldb}, nil
}

// Migrate applies every embedded UP migration.
func (s *Store) Migrate(ctx context.Context) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.UpContext(ctx, s.DB, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

// Close closes the underlying connection.
func (s *Store) Close() error {
	return s.DB.Close()
}

// buildDSN builds the modernc.org/sqlite DSN with the required pragmas.
// Every pragma passed as a query arg is applied to new connections.
func buildDSN(path string) string {
	// url.Values avoids escaping problems with Windows paths.
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "synchronous(NORMAL)")
	// BEGIN IMMEDIATE: transactions take the write lock right away.
	// With a deferred BEGIN, two concurrent read→write transactions make the
	// second fail with SQLITE_BUSY without waiting for busy_timeout (see
	// TestConcurrentReadThenWriteTx).
	q.Add("_txlock", "immediate")
	return fmt.Sprintf("file:%s?%s", filepath.ToSlash(path), q.Encode())
}
