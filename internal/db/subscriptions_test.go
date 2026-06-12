package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestUser(t *testing.T, store *Store, email string) int64 {
	t.Helper()
	users := NewUsers(store.DB)
	id, err := users.Create(context.Background(), email, "hash", "Test")
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}
	return id
}

func TestSubscriptions_GrantAndIsActive(t *testing.T) {
	store := newTestStore(t)
	subs := NewSubscriptions(store.DB)
	ctx := context.Background()
	userID := newTestUser(t, store, "premium@test.it")

	// Senza sottoscrizione: non attiva, GetByUser ritorna not found.
	active, err := subs.IsActive(ctx, userID)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if active {
		t.Error("IsActive: want false senza sottoscrizione")
	}
	if _, err := subs.GetByUser(ctx, userID); !errors.Is(err, ErrSubscriptionNotFound) {
		t.Errorf("GetByUser: want ErrSubscriptionNotFound, got %v", err)
	}

	// Grant senza scadenza.
	if err := subs.Grant(ctx, userID, "manual", nil, "", "test"); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	active, err = subs.IsActive(ctx, userID)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("IsActive: want true dopo grant")
	}

	sub, err := subs.GetByUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if sub.Source != "manual" || sub.Status != "active" || sub.ExpiresAt != nil {
		t.Errorf("sottoscrizione inattesa: %+v", sub)
	}
}

func TestSubscriptions_Expiry(t *testing.T) {
	store := newTestStore(t)
	subs := NewSubscriptions(store.DB)
	ctx := context.Background()
	userID := newTestUser(t, store, "expired@test.it")

	// Scadenza nel passato → non attiva.
	past := time.Now().Add(-time.Hour)
	if err := subs.Grant(ctx, userID, "manual", &past, "", ""); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	active, err := subs.IsActive(ctx, userID)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if active {
		t.Error("IsActive: want false con sottoscrizione scaduta")
	}

	// Re-grant (upsert) con scadenza futura → attiva.
	future := time.Now().Add(24 * time.Hour)
	if err := subs.Grant(ctx, userID, "manual", &future, "", ""); err != nil {
		t.Fatalf("Grant upsert: %v", err)
	}
	active, err = subs.IsActive(ctx, userID)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("IsActive: want true dopo re-grant con scadenza futura")
	}
}

func TestSubscriptions_Revoke(t *testing.T) {
	store := newTestStore(t)
	subs := NewSubscriptions(store.DB)
	ctx := context.Background()
	userID := newTestUser(t, store, "revoked@test.it")

	if err := subs.Grant(ctx, userID, "manual", nil, "", ""); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if err := subs.Revoke(ctx, userID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	active, err := subs.IsActive(ctx, userID)
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if active {
		t.Error("IsActive: want false dopo revoke")
	}
	// La riga resta per audit.
	sub, err := subs.GetByUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if sub.Status != "revoked" {
		t.Errorf("Status: want revoked, got %q", sub.Status)
	}
}

func TestSubscriptions_MapAndCount(t *testing.T) {
	store := newTestStore(t)
	subs := NewSubscriptions(store.DB)
	ctx := context.Background()
	u1 := newTestUser(t, store, "a@test.it")
	u2 := newTestUser(t, store, "b@test.it")
	newTestUser(t, store, "c@test.it") // senza sottoscrizione

	if err := subs.Grant(ctx, u1, "manual", nil, "", ""); err != nil {
		t.Fatalf("Grant u1: %v", err)
	}
	if err := subs.Grant(ctx, u2, "license", nil, "NUTREEK-TEST-KEY", ""); err != nil {
		t.Fatalf("Grant u2: %v", err)
	}
	if err := subs.Revoke(ctx, u2); err != nil {
		t.Fatalf("Revoke u2: %v", err)
	}

	m, err := subs.MapByUser(ctx)
	if err != nil {
		t.Fatalf("MapByUser: %v", err)
	}
	if len(m) != 2 {
		t.Errorf("MapByUser: want 2 righe, got %d", len(m))
	}
	if m[u2].LicenseKey == nil || *m[u2].LicenseKey != "NUTREEK-TEST-KEY" {
		t.Errorf("LicenseKey u2 non salvata: %+v", m[u2])
	}

	n, err := subs.CountActive(ctx)
	if err != nil {
		t.Fatalf("CountActive: %v", err)
	}
	if n != 1 {
		t.Errorf("CountActive: want 1, got %d", n)
	}
}
