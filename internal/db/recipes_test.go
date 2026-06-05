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

	// Stesso nome stesso utente → ErrRecipeNameTaken.
	if _, err := recipes.Create(ctx, alice, "Pasta al pomodoro", nil, 2, false); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("atteso ErrRecipeNameTaken, got %v", err)
	}
	// Case-insensitive: "PASTA AL POMODORO" → ErrRecipeNameTaken.
	if _, err := recipes.Create(ctx, alice, "PASTA AL POMODORO", nil, 2, false); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("atteso ErrRecipeNameTaken case-insensitive, got %v", err)
	}
	// Stesso nome ma utente diverso → OK.
	if _, err := recipes.Create(ctx, bob, "Pasta al pomodoro", nil, 2, false); err != nil {
		t.Errorf("Bob deve poter usare lo stesso nome di Alice: %v", err)
	}

	// Update a un nome già usato → ErrRecipeNameTaken.
	id2, err := recipes.Create(ctx, alice, "Carbonara", nil, 2, false)
	if err != nil {
		t.Fatalf("create id2: %v", err)
	}
	if err := recipes.Update(ctx, id2, alice, "Pasta al pomodoro", nil, 2, false); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("rename a nome esistente: atteso ErrRecipeNameTaken, got %v", err)
	}
	// Update allo stesso nome (no-op) → OK.
	if err := recipes.Update(ctx, id1, alice, "Pasta al pomodoro", nil, 3, false); err != nil {
		t.Errorf("update con stesso nome (no-op): %v", err)
	}
	// NameExists helper.
	exists, err := recipes.NameExists(ctx, alice, "pasta AL pomodoro", 0)
	if err != nil || !exists {
		t.Errorf("NameExists case-insensitive: %v %v", exists, err)
	}
	exists, _ = recipes.NameExists(ctx, alice, "pasta AL pomodoro", id1)
	if exists {
		t.Errorf("NameExists con excludeID = self deve restituire false")
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
		t.Errorf("Duplicate ha ritornato lo stesso id")
	}
	dup, err := recipes.GetWithItems(ctx, dupID, alice)
	if err != nil {
		t.Fatalf("GetWithItems dup: %v", err)
	}
	if dup.Name != "Copia di Origine" {
		t.Errorf("nome copia: %q", dup.Name)
	}
	if dup.Servings != 2 {
		t.Errorf("servings non copiato: %d", dup.Servings)
	}
	if len(dup.Items) != 2 {
		t.Errorf("items copiati: %d (attesi 2)", len(dup.Items))
	}
	// Stesso nome → blocco.
	if _, err := recipes.Duplicate(ctx, src, alice, "Origine"); !errors.Is(err, ErrRecipeNameTaken) {
		t.Errorf("duplicate con nome esistente: atteso ErrRecipeNameTaken, got %v", err)
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

	// items = stesso set → match
	sameItems := []domain.RecipeItem{
		{IngredientID: pane, Quantity: 100},
		{IngredientID: pasta, Quantity: 80},
	}
	name, err := recipes.IdenticalContentRecipe(ctx, alice, sameItems, 0)
	if err != nil {
		t.Fatalf("IdenticalContentRecipe: %v", err)
	}
	if name != "Ricetta A" {
		t.Errorf("atteso match con Ricetta A, got %q", name)
	}

	// Stesso set ma esclusa la sorgente → nessun match.
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, sameItems, r1)
	if name != "" {
		t.Errorf("con excludeID=r1: atteso no match, got %q", name)
	}

	// Quantità diversa → no match.
	diff := []domain.RecipeItem{
		{IngredientID: pane, Quantity: 99},
		{IngredientID: pasta, Quantity: 80},
	}
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, diff, 0)
	if name != "" {
		t.Errorf("qty diversa: atteso no match, got %q", name)
	}

	// Ordine diverso → match (firma è ordinata).
	reversed := []domain.RecipeItem{
		{IngredientID: pasta, Quantity: 80},
		{IngredientID: pane, Quantity: 100},
	}
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, reversed, 0)
	if name != "Ricetta A" {
		t.Errorf("ordine inverso: atteso match Ricetta A, got %q", name)
	}

	// Items vuoti → no match (ricette vuote non collidono).
	name, _ = recipes.IdenticalContentRecipe(ctx, alice, nil, 0)
	if name != "" {
		t.Errorf("items vuoti: atteso no match, got %q", name)
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

	// Alice cancella r1, r2 e prova a cancellare r3Bob (non sua) → solo 2 cancellate.
	n, err := recipes.BulkDelete(ctx, []int64{r1, r2, r3Bob}, alice)
	if err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if n != 2 {
		t.Errorf("BulkDelete: cancellate %d, attese 2 (r3Bob non di Alice)", n)
	}
	// Verifica che r3Bob esista ancora.
	if _, err := recipes.GetWithItems(ctx, r3Bob, bob); err != nil {
		t.Errorf("r3Bob non dovrebbe essere stata cancellata: %v", err)
	}

	// CountMealUsages su ricetta inesistente → 0.
	count, err := recipes.CountMealUsages(ctx, 99999)
	if err != nil {
		t.Fatalf("CountMealUsages: %v", err)
	}
	if count != 0 {
		t.Errorf("count su ricetta inesistente: %d", count)
	}
}
