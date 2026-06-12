package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupTo crea un backup consistente del database usando VACUUM INTO.
// Sicuro in concorrenza WAL: SQLite garantisce uno snapshot-read durante
// l'intera operazione, senza bloccare i reader né il writer corrente.
// destPath viene creato (o sovrascritto) dall'operazione.
func (s *Store) BackupTo(ctx context.Context, destPath string) error {
	if dir := filepath.Dir(destPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("backup: crea dir %q: %w", dir, err)
		}
	}
	// Rimuove eventuale file pre-esistente: VACUUM INTO fallisce se il file esiste.
	_ = os.Remove(destPath)
	if _, err := s.DB.ExecContext(ctx, "VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("backup: vacuum into: %w", err)
	}
	return nil
}

// PruneBackups mantiene al massimo keep file nella directory dir, eliminando
// i più vecchi (ordine lessicografico, che corrisponde a ordine cronologico
// se i nomi seguono il pattern nutreek-backup-YYYYMMDD-HHMMSS.db).
func PruneBackups(dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("prune backups: readdir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "nutreek-backup-") && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}

	sort.Strings(files) // ordine cronologico
	for len(files) > keep {
		if err := os.Remove(files[0]); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("prune backups: remove %q: %w", files[0], err)
		}
		files = files[1:]
	}
	return nil
}

// BackupFileName genera un nome file per il backup usando il timestamp corrente.
func BackupFileName(dir string, t time.Time) string {
	return filepath.Join(dir, fmt.Sprintf("nutreek-backup-%s.db", t.UTC().Format("20060102-150405")))
}
