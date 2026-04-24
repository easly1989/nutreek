package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return store
}

func TestUsersCreateAndFetch(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()

	id, err := users.Create(ctx, "  Alice@Example.com ", "hash123", "Alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id <= 0 {
		t.Errorf("id atteso > 0, got %d", id)
	}

	byEmail, err := users.GetByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail.ID != id {
		t.Errorf("id mismatch: %d vs %d", byEmail.ID, id)
	}
	if byEmail.Email != "alice@example.com" {
		t.Errorf("email non normalizzata: %q", byEmail.Email)
	}
	if byEmail.DisplayName != "Alice" {
		t.Errorf("display name mismatch: %q", byEmail.DisplayName)
	}
	if byEmail.PasswordHash != "hash123" {
		t.Errorf("password hash mismatch")
	}
	if byEmail.CreatedAt.IsZero() {
		t.Error("created_at zero")
	}

	byID, err := users.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Email != byEmail.Email {
		t.Errorf("GetByID email mismatch")
	}
}

func TestUsersCreateDuplicateEmail(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()

	if _, err := users.Create(ctx, "bob@example.com", "h", "Bob"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := users.Create(ctx, "BOB@example.com", "h2", "Bob2")
	if !errors.Is(err, ErrEmailAlreadyUsed) {
		t.Fatalf("atteso ErrEmailAlreadyUsed, got %v", err)
	}
}

func TestUsersGetNotFound(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()

	_, err := users.GetByEmail(ctx, "ghost@example.com")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("atteso ErrUserNotFound, got %v", err)
	}
	_, err = users.GetByID(ctx, 99999)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("atteso ErrUserNotFound, got %v", err)
	}
}
