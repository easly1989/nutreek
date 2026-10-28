package db

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestRecipePhotos(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	recipes := NewRecipes(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "a@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "b@x.com", "h", "Bob")
	rec, err := recipes.Create(ctx, alice, "Porridge", nil, 1, true)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := recipes.Photo(ctx, rec, false); !errors.Is(err, ErrPhotoNotFound) {
		t.Fatalf("no photo yet: %v, want ErrPhotoNotFound", err)
	}
	if err := recipes.SetPhoto(ctx, rec, bob, []byte("p"), []byte("t")); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("Bob sets Alice's photo: %v, want ErrRecipeForbidden", err)
	}
	if err := recipes.SetPhoto(ctx, rec, alice, []byte("photo-1"), []byte("thumb-1")); err != nil {
		t.Fatal(err)
	}
	first, _ := recipes.PhotoETag(ctx, rec)
	if err := recipes.SetPhoto(ctx, rec, alice, []byte("photo-2"), []byte("thumb-2")); err != nil {
		t.Fatalf("replace: %v", err)
	}
	data, etag, err := recipes.Photo(ctx, rec, true)
	if err != nil || !bytes.Equal(data, []byte("thumb-2")) || etag == "" || etag == first {
		t.Errorf("thumb after replace: %q, etag %q (was %q), %v", data, etag, first, err)
	}
	if tags, _ := recipes.PhotoETagsByRecipe(ctx, bob); tags[rec] != etag {
		t.Errorf("Bob's list etags %v: the public recipe's photo is missing", tags)
	}

	// The copy gets the photo; deleting the source leaves the copy's.
	dup, err := recipes.Duplicate(ctx, rec, alice, "Porridge 2")
	if err != nil {
		t.Fatal(err)
	}
	if data, _, err := recipes.Photo(ctx, dup, false); err != nil || !bytes.Equal(data, []byte("photo-2")) {
		t.Errorf("duplicate's photo: %q, %v", data, err)
	}
	if err := recipes.Delete(ctx, rec, alice); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := store.DB.QueryRow(`SELECT count(*) FROM recipe_photos WHERE recipe_id = ?`, rec).Scan(&n); err != nil || n != 0 {
		t.Errorf("photos of the deleted recipe: %d, %v; want 0", n, err)
	}

	if err := recipes.DeletePhoto(ctx, dup, bob); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("Bob removes Alice's photo: %v, want ErrRecipeForbidden", err)
	}
	if err := recipes.DeletePhoto(ctx, dup, alice); err != nil {
		t.Fatal(err)
	}
	if etag, err := recipes.PhotoETag(ctx, dup); etag != "" || err != nil {
		t.Errorf("after remove: etag %q, %v", etag, err)
	}
}
