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

// Store avvolge *sql.DB e centralizza apertura e migrazioni.
type Store struct {
	DB *sql.DB
}

// Open apre (o crea) il database SQLite al path indicato, applica pragmas
// sicuri e verifica la connessione. La directory parent viene creata se mancante.
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

	// WAL permette letture concorrenti + un writer. Lasciamo il default del
	// pool di database/sql: SQLite risponde BUSY e busy_timeout rilancia.
	return &Store{DB: sqldb}, nil
}

// Migrate applica tutte le migrazioni UP embedded.
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

// Close chiude la connessione sottostante.
func (s *Store) Close() error {
	return s.DB.Close()
}

// buildDSN costruisce il DSN modernc.org/sqlite con i pragmas richiesti.
// Ogni pragma passato come query arg viene applicato sulle connessioni nuove.
func buildDSN(path string) string {
	// Usiamo url.Values per evitare problemi di escaping con path Windows.
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return fmt.Sprintf("file:%s?%s", filepath.ToSlash(path), q.Encode())
}
