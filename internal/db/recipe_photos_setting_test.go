package db

import (
	"context"
	"errors"
	"testing"
)

// The recipe photos of the list are on for a new user and the choice is saved (#251).
func TestShowRecipePhotosSetting(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ctx := context.Background()

	id, _ := users.Create(ctx, "photos@x.com", "h", "Photos")
	if u, _ := users.GetByID(ctx, id); !u.ShowRecipePhotos {
		t.Error("a new user has the recipe photos off")
	}
	if err := users.SetShowRecipePhotos(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if u, _ := users.GetByID(ctx, id); u.ShowRecipePhotos {
		t.Error("the setting was not saved")
	}
	if err := users.SetShowRecipePhotos(ctx, 9999, true); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("an unknown user = %v, want ErrUserNotFound", err)
	}
}
