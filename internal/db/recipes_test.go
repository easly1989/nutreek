package db

import (
	"context"
	"errors"
	"testing"
)

func TestRecipesVisibilityAndCRUD(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	recipes := NewRecipes(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, err := users.Create(ctx, "alice@x.com", "h", "Alice")
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	bob, err := users.Create(ctx, "bob@x.com", "h", "Bob")
	if err != nil {
		t.Fatalf("bob: %v", err)
	}

	// Una ricetta privata di Alice, una pubblica di Alice, una privata di Bob.
	priv, err := recipes.Create(ctx, alice, "Privata Alice", nil, 2, false)
	if err != nil {
		t.Fatalf("priv: %v", err)
	}
	pub, err := recipes.Create(ctx, alice, "Pubblica Alice", nil, 4, true)
	if err != nil {
		t.Fatalf("pub: %v", err)
	}
	bobPriv, err := recipes.Create(ctx, bob, "Privata Bob", nil, 1, false)
	if err != nil {
		t.Fatalf("bobPriv: %v", err)
	}

	// Bob vede: la sua privata + la pubblica di Alice (2). Non vede la privata di Alice.
	bobList, err := recipes.ListVisible(ctx, bob)
	if err != nil {
		t.Fatalf("ListVisible bob: %v", err)
	}
	if len(bobList) != 2 {
		t.Fatalf("Bob vede %d ricette, attese 2", len(bobList))
	}
	for _, r := range bobList {
		if r.ID == priv {
			t.Errorf("Bob non dovrebbe vedere la privata di Alice")
		}
	}

	// Alice vede tutte le sue (2) + nessuna di Bob (priv).
	aliceList, err := recipes.ListVisible(ctx, alice)
	if err != nil {
		t.Fatalf("ListVisible alice: %v", err)
	}
	if len(aliceList) != 2 {
		t.Fatalf("Alice vede %d ricette, attese 2 (le sue)", len(aliceList))
	}

	// GetWithItems: Bob legge la pubblica di Alice OK, ma riceve forbidden per la privata.
	if _, err := recipes.GetWithItems(ctx, pub, bob); err != nil {
		t.Errorf("Bob non riesce a leggere la pubblica di Alice: %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, priv, bob); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("atteso ErrRecipeForbidden per Bob su priv di Alice, got %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, 9999, alice); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("atteso ErrRecipeNotFound, got %v", err)
	}

	// Bob cerca di cancellare/aggiornare la pubblica di Alice → forbidden.
	if err := recipes.Update(ctx, pub, bob, "hijack", nil, 1, true); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("Bob update altrui dovrebbe essere forbidden, got %v", err)
	}
	if err := recipes.Delete(ctx, pub, bob); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("Bob delete altrui dovrebbe essere forbidden, got %v", err)
	}

	// Alice aggiunge un item alla pubblica.
	ingID, err := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice)
	if err != nil {
		t.Fatalf("ingredient: %v", err)
	}
	itemID, err := recipes.AddItem(ctx, pub, alice, ingID, 200)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	got, err := recipes.GetWithItems(ctx, pub, alice)
	if err != nil {
		t.Fatalf("GetWithItems after add: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Quantity != 200 || got.Items[0].Ingredient.Name != "Pane" {
		t.Errorf("item non come atteso: %+v", got.Items)
	}

	// Bob non puo' cancellare l'item di Alice.
	if err := recipes.RemoveItem(ctx, itemID, bob); !errors.Is(err, ErrRecipeItemNotFound) {
		t.Errorf("Bob remove item altrui dovrebbe ErrRecipeItemNotFound, got %v", err)
	}
	// Bob non vede comunque la ricetta privata di Alice → bobPriv = sanity check rimasta privata.
	if _, err := recipes.GetWithItems(ctx, bobPriv, bob); err != nil {
		t.Errorf("Bob legge la sua privata, got %v", err)
	}

	// Alice rimuove l'item.
	if err := recipes.RemoveItem(ctx, itemID, alice); err != nil {
		t.Fatalf("Alice RemoveItem: %v", err)
	}
	got, _ = recipes.GetWithItems(ctx, pub, alice)
	if len(got.Items) != 0 {
		t.Errorf("dopo RemoveItem ci sono %d item", len(got.Items))
	}

	// Alice cancella la ricetta privata.
	if err := recipes.Delete(ctx, priv, alice); err != nil {
		t.Fatalf("delete priv: %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, priv, alice); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("dopo delete priv dovrebbe essere not-found, got %v", err)
	}
}
