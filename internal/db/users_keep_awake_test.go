package db

import (
	"context"
	"errors"
	"testing"
)

// The keep-awake setting is off for a new user, saves per user and refuses an
// unknown one (#249).
func TestUsersKeepAwake(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	get := func(id int64) bool {
		u, err := users.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return u.KeepAwake
	}
	if get(alice) {
		t.Fatal("must start off")
	}
	if err := users.SetKeepAwake(ctx, alice, true); err != nil {
		t.Fatal(err)
	}
	if !get(alice) || get(bob) {
		t.Error("on for Alice only")
	}
	if err := users.SetKeepAwake(ctx, alice, false); err != nil || get(alice) {
		t.Errorf("off again: %v", err)
	}
	if err := users.SetKeepAwake(ctx, 9999, true); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown user: %v", err)
	}
	all, err := users.ListAll(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListAll: %v (%d)", err, len(all))
	}
}
