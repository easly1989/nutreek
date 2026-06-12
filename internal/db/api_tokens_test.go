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

	// Lista inizialmente vuota.
	list, err := tokens.List(ctx, userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("lista iniziale: want 0, got %d", len(list))
	}

	// Creazione: il token in chiaro ha il prefisso corretto.
	plain, err := tokens.Create(ctx, userID, "test-token")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(plain, "nutreek_") {
		t.Errorf("token senza prefisso nutreek_: %q", plain)
	}

	// Dopo la creazione la lista ha un elemento.
	list, err = tokens.List(ctx, userID)
	if err != nil {
		t.Fatalf("List dopo create: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List: want 1, got %d", len(list))
	}
	if list[0].Name != "test-token" {
		t.Errorf("nome token: want test-token, got %q", list[0].Name)
	}
	if list[0].LastUsedAt != nil {
		t.Error("last_used_at deve essere nil alla creazione")
	}

	// Authenticate: token valido restituisce l'utente e aggiorna last_used_at.
	user, err := tokens.Authenticate(ctx, plain)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if user.ID != userID {
		t.Errorf("userID: want %d, got %d", userID, user.ID)
	}

	// Dopo authenticate last_used_at è valorizzato.
	list, _ = tokens.List(ctx, userID)
	if list[0].LastUsedAt == nil {
		t.Error("last_used_at deve essere valorizzato dopo Authenticate")
	}

	// Authenticate con token errato: ErrAPITokenNotFound.
	_, err = tokens.Authenticate(ctx, "nutreek_invalido")
	if !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("token errato: want ErrAPITokenNotFound, got %v", err)
	}

	// Revoca con userID errato: ErrAPITokenNotFound.
	other := newTestUser(t, store, "other-token@test.it")
	if err := tokens.Revoke(ctx, list[0].ID, other); !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("revoca altrui: want ErrAPITokenNotFound, got %v", err)
	}

	// Revoca corretta: lista vuota, token non più autenticabile.
	if err := tokens.Revoke(ctx, list[0].ID, userID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	list, _ = tokens.List(ctx, userID)
	if len(list) != 0 {
		t.Errorf("dopo revoca: want 0, got %d", len(list))
	}
	_, err = tokens.Authenticate(ctx, plain)
	if !errors.Is(err, ErrAPITokenNotFound) {
		t.Errorf("autenticazione dopo revoca: want ErrAPITokenNotFound, got %v", err)
	}
}
