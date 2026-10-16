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

// BackupTo writes a consistent backup of the database with VACUUM INTO.
// Safe under WAL concurrency: SQLite reads from one snapshot for the whole
// operation, without blocking readers or the current writer.
// The operation creates (or overwrites) destPath.
func (s *Store) BackupTo(ctx context.Context, destPath string) error {
	if dir := filepath.Dir(destPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("backup: crea dir %q: %w", dir, err)
		}
	}
	// Remove any existing file: VACUUM INTO fails when the file exists.
	_ = os.Remove(destPath)
	if _, err := s.DB.ExecContext(ctx, "VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("backup: vacuum into: %w", err)
	}
	return nil
}

// PruneBackups keeps at most keep files in dir, deleting the oldest
// (lexicographic order, which is chronological order when the names follow
// the nutreek-backup-YYYYMMDD-HHMMSS.db pattern).
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

// BackupFileName builds a backup file name from the current timestamp.
func BackupFileName(dir string, t time.Time) string {
	return filepath.Join(dir, fmt.Sprintf("nutreek-backup-%s.db", t.UTC().Format("20060102-150405")))
}
