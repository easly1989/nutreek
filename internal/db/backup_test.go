package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupTo_CreatesFile(t *testing.T) {
	store := newTestStore(t)
	dest := filepath.Join(t.TempDir(), "backup.db")

	if err := store.BackupTo(context.Background(), dest); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("empty backup file")
	}
}

func TestBackupTo_OverwritesExisting(t *testing.T) {
	store := newTestStore(t)
	dest := filepath.Join(t.TempDir(), "backup.db")

	// Primo backup
	if err := store.BackupTo(context.Background(), dest); err != nil {
		t.Fatalf("primo BackupTo: %v", err)
	}
	// Second backup: overwrite
	if err := store.BackupTo(context.Background(), dest); err != nil {
		t.Fatalf("secondo BackupTo: %v", err)
	}
}

func TestPruneBackups_KeepsN(t *testing.T) {
	dir := t.TempDir()

	// Create 5 fake backup files with increasing timestamps
	for i := range 5 {
		name := BackupFileName(dir, time.Now().Add(time.Duration(i)*time.Second))
		f, err := os.Create(name)
		if err != nil {
			t.Fatalf("create test file: %v", err)
		}
		_ = f.Close()
	}

	// Keep only 3
	if err := PruneBackups(dir, 3); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("PruneBackups: want 3 file, got %d", len(entries))
	}
}
