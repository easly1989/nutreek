package db

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAPITokens_CreateListRevokeAuthenticate(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	tokens := NewAPITokens(store.DB, users)
	ctx := context.Background()

	userID := newTestUser(t, store, "token@test.it")

	// The list starts empty.
	list, err := tokens.List(ctx, userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("initial list: want 0, got %d", len(list))
	}

	// Create: the plaintext token has the right prefix.
	plain, err := tokens.Create(ctx, userID, "test-token")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(plain, "nutreek_") {
		t.Errorf("token without the nutreek_ prefix: %q", plain)
	}

	// After creation the list has one item.
	list, err = tokens.List(ctx, userID)
	if err != nil {
		t.Fatalf("List after create: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List: want 1, got %d", len(list))
	}
	if list[0].Name != "test-token" {
		t.Errorf("nome token: want test-token, got %q", list[0].Name)
	}
	if list[0].LastUsedAt != nil {
		t.Error("last_used_at must be nil at creation")
	}

	// Authenticate: a valid token returns the user and updates last_used_at.
	user, err := tokens.Authenticate(ctx, plain)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if user.ID != userID {
		t.Errorf("userID: want %d, got %d", userID, user.ID)
	}

	// After Authenticate, last_used_at is set.
	list, _ = tokens.List(ctx, userID)
	if list[0].LastUsedAt == nil {
		t.Error("last_used_at must be set after Authenticate")
	}

	// Authenticate with a wrong token: ErrAPITokenNotFound.
	_, err = tokens.Authenticate(ctx, "nutreek_invalido")
	if !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("token errato: want ErrAPITokenNotFound, got %v", err)
	}

	// Revoke with the wrong userID: ErrAPITokenNotFound.
	other := newTestUser(t, store, "other-token@test.it")
	if err := tokens.Revoke(ctx, list[0].ID, other); !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("revoca altrui: want ErrAPITokenNotFound, got %v", err)
	}

	// A correct revoke: empty list, the token no longer authenticates.
	if err := tokens.Revoke(ctx, list[0].ID, userID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	list, _ = tokens.List(ctx, userID)
	if len(list) != 0 {
		t.Errorf("after revoke: want 0, got %d", len(list))
	}
	_, err = tokens.Authenticate(ctx, plain)
	if !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("authenticate after revoke: want ErrAPITokenNotFound, got %v", err)
	}
}
