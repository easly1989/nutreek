package db

import (
	"context"
	"errors"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
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

	// A private recipe of Alice, a public one of Alice, a private one of Bob.
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

	// Bob sees: his private one + Alice's public one (2). Not Alice's private one.
	bobList, err := recipes.ListVisible(ctx, bob)
	if err != nil {
		t.Fatalf("ListVisible bob: %v", err)
	}
	if len(bobList) != 2 {
		t.Fatalf("Bob sees %d recipes, want 2", len(bobList))
	}
	for _, r := range bobList {
		if r.ID == priv {
			t.Errorf("Bob should not see Alice's private recipe")
		}
	}

	// Alice sees all of hers (2) + none of Bob's (private).
	aliceList, err := recipes.ListVisible(ctx, alice)
	if err != nil {
		t.Fatalf("ListVisible alice: %v", err)
	}
	if len(aliceList) != 2 {
		t.Fatalf("Alice sees %d recipes, want 2 (her own)", len(aliceList))
	}

	// GetWithItems: Bob reads Alice's public recipe, but gets forbidden on the private one.
	if _, err := recipes.GetWithItems(ctx, pub, bob); err != nil {
		t.Errorf("Bob cannot read Alice's public recipe: %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, priv, bob); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("want ErrRecipeForbidden for Bob on Alice's private recipe, got %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, 9999, alice); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("want ErrRecipeNotFound, got %v", err)
	}

	// Bob tries to delete/update Alice's public recipe → forbidden.
	if err := recipes.Update(ctx, pub, bob, "hijack", nil, 1, true); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("Bob updating another user's recipe should be forbidden, got %v", err)
	}
	if err := recipes.Delete(ctx, pub, bob); !errors.Is(err, ErrRecipeForbidden) {
		t.Errorf("Bob deleting another user's recipe should be forbidden, got %v", err)
	}

	// Alice adds an item to the public recipe.
	ingID, err := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
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
		t.Errorf("item not as expected: %+v", got.Items)
	}

	// Bob cannot delete Alice's item.
	if err := recipes.RemoveItem(ctx, itemID, bob); !errors.Is(err, ErrRecipeItemNotFound) {
		t.Errorf("Bob remove item altrui dovrebbe ErrRecipeItemNotFound, got %v", err)
	}
	// Bob's own private recipe → bobPriv = sanity check that it stayed private.
	if _, err := recipes.GetWithItems(ctx, bobPriv, bob); err != nil {
		t.Errorf("Bob reads his own private recipe, got %v", err)
	}

	// Alice removes the item.
	if err := recipes.RemoveItem(ctx, itemID, alice); err != nil {
		t.Fatalf("Alice RemoveItem: %v", err)
	}
	got, _ = recipes.GetWithItems(ctx, pub, alice)
	if len(got.Items) != 0 {
		t.Errorf("after RemoveItem there are %d items", len(got.Items))
	}

	// Alice deletes the private recipe.
	if err := recipes.Delete(ctx, priv, alice); err != nil {
		t.Fatalf("delete priv: %v", err)
	}
	if _, err := recipes.GetWithItems(ctx, priv, alice); !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("after delete the private recipe should be not found, got %v", err)
	}
}

func TestRecipesNameUniqueness(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	recipes := NewRecipes(store.DB)
	ctx := context.Background()

	alice, err := users.Create(ctx, "a@x.com", "h", "Alice")
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	bob, err := users.Create(ctx, "b@x.com", "h", "Bob")
	if err != nil {
		t.Fatalf("bob: %v", err)
	}

	id1, err := recipes.Create(ctx, alice, "Pasta al pomodoro", nil, 2, false)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Same name, same user → ErrRecipeNameTaken.
	if _, err := recipes.Create(ctx, alice, "Pasta al pomodoro", nil, 2, false); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("want ErrRecipeNameTaken, got %v", err)
	}
	// Case-insensitive: "PASTA AL POMODORO" → ErrRecipeNameTaken.
	if _, err := recipes.Create(ctx, alice, "PASTA AL POMODORO", nil, 2, false); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("want ErrRecipeNameTaken case-insensitive, got %v", err)
	}
	// Same name, different user → OK.
	if _, err := recipes.Create(ctx, bob, "Pasta al pomodoro", nil, 2, false); err != nil {
		t.Errorf("Bob must be able to use Alice's name: %v", err)
	}

	// Update to a name already used → ErrRecipeNameTaken.
	id2, err := recipes.Create(ctx, alice, "Carbonara", nil, 2, false)
	if err != nil {
		t.Fatalf("create id2: %v", err)
	}
	if err := recipes.Update(ctx, id2, alice, "Pasta al pomodoro", nil, 2, false); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("rename to an existing name: want ErrRecipeNameTaken, got %v", err)
	}
	// Update to the same name (no-op) → OK.
	if err := recipes.Update(ctx, id1, alice, "Pasta al pomodoro", nil, 3, false); err != nil {
		t.Errorf("update with the same name (no-op): %v", err)
	}
	// NameExists helper.
	exists, err := recipes.NameExists(ctx, alice, "pasta AL pomodoro", 0)
	if err != nil || !exists {
		t.Errorf("NameExists case-insensitive: %v %v", exists, err)
	}
	exists, _ = recipes.NameExists(ctx, alice, "pasta AL pomodoro", id1)
	if exists {
		t.Errorf("NameExists with excludeID = self must return false")
	}
}

func TestRecipesDuplicate(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	recipes := NewRecipes(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "a@x.com", "h", "Alice")
	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 350, 12, 70, 1.5, &alice, nil, nil, nil, nil, nil, nil)

	src, _ := recipes.Create(ctx, alice, "Origine", nil, 2, false)
	if _, err := recipes.AddItem(ctx, src, alice, pane, 100); err != nil {
		t.Fatalf("AddItem pane: %v", err)
	}
	if _, err := recipes.AddItem(ctx, src, alice, pasta, 80); err != nil {
		t.Fatalf("AddItem pasta: %v", err)
	}

	dupID, err := recipes.Duplicate(ctx, src, alice, "Copia di Origine")
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dupID == src {
		t.Errorf("Duplicate returned the same id")
	}
	dup, err := recipes.GetWithItems(ctx, dupID, alice)
	if err != nil {
		t.Fatalf("GetWithItems dup: %v", err)
	}
	if dup.Name != "Copia di Origine" {
		t.Errorf("nome copia: %q", dup.Name)
	}
	if dup.Servings != 2 {
		t.Errorf("servings not copied: %d", dup.Servings)
	}
	if len(dup.Items) != 2 {
		t.Errorf("copied items: %d (want 2)", len(dup.Items))
	}
	// Same name → blocked.
	if _, err := recipes.Duplicate(ctx, src, alice, "Origine"); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("duplicate with an existing name: want ErrRecipeNameTaken, got %v", err)
	}
}

func TestRecipesIdenticalContent(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	recipes := NewRecipes(store.DB)
	ings := NewIngredients(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "a@x.com", "h", "Alice")
	pane, _ := ings.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &alice, nil, nil, nil, nil, nil, nil)
	pasta, _ := ings.CreateLocal(ctx, "Pasta", nil, "g", 350, 12, 70, 1.5, &alice, nil, nil, nil, nil, nil, nil)

	r1, _ := recipes.Create(ctx, alice, "Ricetta A", nil, 2, false)
	_, _ = recipes.AddItem(ctx, r1, alice, pane, 100)
	_, _ = recipes.AddItem(ctx, r1, alice, pasta, 80)

	// items = the same set → match
	sameItems := []domain.RecipeItem{
		{IngredientID: pane, Quantity: 100},
		{IngredientID: pasta, Quantity: 80},
	}
	name, err := recipes.IdenticalContentRecipe(ctx, alice, sameItems, 0)
	if err != nil {
		t.Fatalf("IdenticalContentRecipe: %v", err)
	}
	if name != "Ricetta A" {
		t.Errorf("want a match with Ricetta A, got %q", name)
	}

	// The same set but with the source excluded → no match.
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, sameItems, r1)
	if name != "" {
		t.Errorf("with excludeID=r1: want no match, got %q", name)
	}

	// A different quantity → no match.
	diff := []domain.RecipeItem{
		{IngredientID: pane, Quantity: 99},
		{IngredientID: pasta, Quantity: 80},
	}
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, diff, 0)
	if name != "" {
		t.Errorf("different qty: want no match, got %q", name)
	}

	// A different order → match (the signature is sorted).
	reversed := []domain.RecipeItem{
		{IngredientID: pasta, Quantity: 80},
		{IngredientID: pane, Quantity: 100},
	}
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, reversed, 0)
	if name != "Ricetta A" {
		t.Errorf("reversed order: want a match with Ricetta A, got %q", name)
	}

	// Empty items → no match (empty recipes do not collide).
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, nil, 0)
	if name != "" {
		t.Errorf("empty items: want no match, got %q", name)
	}
}

func TestRecipesBulkDeleteAndUsages(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	recipes := NewRecipes(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "a@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "b@x.com", "h", "Bob")

	r1, _ := recipes.Create(ctx, alice, "R1", nil, 1, false)
	r2, _ := recipes.Create(ctx, alice, "R2", nil, 1, false)
	r3Bob, _ := recipes.Create(ctx, bob, "R3 Bob", nil, 1, false)

	// Alice deletes r1, r2 and tries to delete r3Bob (not hers) → only 2 deleted.
	n, err := recipes.BulkDelete(ctx, []int64{r1, r2, r3Bob}, alice)
	if err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if n != 2 {
		t.Errorf("BulkDelete: deleted %d, want 2 (r3Bob is not Alice's)", n)
	}
	// Check that r3Bob still exists.
	if _, err := recipes.GetWithItems(ctx, r3Bob, bob); err != nil {
		t.Errorf("r3Bob should not have been deleted: %v", err)
	}

	// CountMealUsages on a missing recipe → 0.
	count, err := recipes.CountMealUsages(ctx, 99999)
	if err != nil {
		t.Fatalf("CountMealUsages: %v", err)
	}
	if count != 0 {
		t.Errorf("count on a missing recipe: %d", count)
	}
}
