package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// allergenIDByName looks up a seeded allergen's ID by name (the 14 tags are
// inserted by migration 0016).
func allergenIDByName(t *testing.T, cats *Categories, name string) int64 {
	t.Helper()
	tags, err := cats.ListAllergens(context.Background())
	if err != nil {
		t.Fatalf("ListAllergens: %v", err)
	}
	for _, tag := range tags {
		if tag.Name == name {
			return tag.ID
		}
	}
	t.Fatalf("allergen %q not found in the seed", name)
	return 0
}

// The ingredient list shows each row's category and the edit form must
// preselect it (#122): CategoriesForIngredients loads them in one batch.
func TestCategoriesForIngredients(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	latticini, err := cats.CreateCategory(ctx, alice, "Latticini")
	if err != nil {
		t.Fatal(err)
	}
	yogurt, _ := ings.CreateLocal(ctx, "Yogurt", nil, "g", 57, 10, 4, 0.2, &alice, nil, nil, nil, nil, nil, nil)
	mela, _ := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, nil, nil, nil, nil, nil, nil)
	if err := cats.SetIngredientCategory(ctx, alice, yogurt, &latticini); err != nil {
		t.Fatal(err)
	}

	got, err := cats.CategoriesForIngredients(ctx, alice, []int64{yogurt, mela})
	if err != nil {
		t.Fatalf("CategoriesForIngredients: %v", err)
	}
	if c, ok := got[yogurt]; !ok || c.ID != latticini || c.Name != "Latticini" {
		t.Errorf("yogurt category = %+v, want Latticini", got[yogurt])
	}
	if _, ok := got[mela]; ok {
		t.Error("the apple has no category: it must not appear")
	}
	if empty, err := cats.CategoriesForIngredients(ctx, alice, nil); err != nil || len(empty) != 0 {
		t.Errorf("empty list = %v, %v", empty, err)
	}

	// nil removes the category.
	if err := cats.SetIngredientCategory(ctx, alice, yogurt, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, alice, []int64{yogurt}); len(got) != 0 {
		t.Errorf("after nil the category remains: %v", got)
	}
}

// Categories are per user (#143): Bob's choice does not touch Alice's on the
// same ingredient, and another user's category is rejected.
func TestCategoriesPerUser(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	frutta, _ := cats.CreateCategory(ctx, alice, "Frutta")
	snack, _ := cats.CreateCategory(ctx, bob, "Snack")
	mandorle, _ := ings.CreateLocal(ctx, "Mandorle", nil, "g", 579, 21, 22, 50, &alice, nil, nil, nil, nil, nil, nil)

	if err := cats.SetIngredientCategory(ctx, alice, mandorle, &frutta); err != nil {
		t.Fatal(err)
	}
	if err := cats.SetIngredientCategory(ctx, bob, mandorle, &snack); err != nil {
		t.Fatal(err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, alice, []int64{mandorle}); got[mandorle].ID != frutta {
		t.Errorf("Alice sees %+v, want her Frutta", got[mandorle])
	}
	if got, _ := cats.CategoriesForIngredients(ctx, bob, []int64{mandorle}); got[mandorle].ID != snack {
		t.Errorf("Bob sees %+v, want his Snack", got[mandorle])
	}

	// Bob cannot use Alice's category.
	if err := cats.SetIngredientCategory(ctx, bob, mandorle, &frutta); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("another user's category: err = %v, want ErrCategoryNotFound", err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, bob, []int64{mandorle}); got[mandorle].ID != snack {
		t.Errorf("after the rejection Bob sees %+v, want Snack still", got[mandorle])
	}

	// Deleting a category removes its assignments.
	if err := cats.DeleteCategory(ctx, snack, bob); err != nil {
		t.Fatal(err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, bob, []int64{mandorle}); len(got) != 0 {
		t.Errorf("deleted category still assigned: %v", got)
	}
}

// Migration 0028 copies the old ingredients.category_id column into the
// per-user table, on behalf of whoever owns the category.
func TestCategoriesMigrationBackfill(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	frutta, _ := cats.CreateCategory(ctx, alice, "Frutta")
	mela, _ := ings.CreateLocal(ctx, "Mela", nil, "g", 52, 0.3, 13.8, 0.2, &alice, nil, nil, nil, nil, nil, nil)
	if _, err := store.DB.ExecContext(ctx, `UPDATE ingredients SET category_id = ? WHERE id = ?`, frutta, mela); err != nil {
		t.Fatal(err)
	}
	// Rerun the migration's INSERT, taken from the real file: when the
	// migration ran, the test DB was empty.
	up, err := migrationsFS.ReadFile("migrations/0028_ingredient_user_categories.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(up), "INSERT OR IGNORE INTO ingredient_user_categories")
	end := strings.Index(string(up)[start:], ";")
	if start < 0 || end < 0 {
		t.Fatal("backfill INSERT not found in migration 0028")
	}
	if _, err := store.DB.ExecContext(ctx, string(up)[start:start+end]); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, alice, []int64{mela}); got[mela].ID != frutta {
		t.Errorf("after the backfill Alice sees %+v, want Frutta", got[mela])
	}
}

func TestAllergensSeedAndSet(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	// The 14 standard allergens must already be there.
	tags, err := cats.ListAllergens(ctx)
	if err != nil {
		t.Fatalf("ListAllergens: %v", err)
	}
	if len(tags) != 14 {
		t.Fatalf("want 14 seeded allergens, found %d", len(tags))
	}

	id, err := ings.CreateLocal(ctx, "Pane", nil, domain.UnitGram, 270, 9, 50, 3, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}

	glutine := allergenIDByName(t, cats, "glutine")
	soia := allergenIDByName(t, cats, "soia")

	if err := cats.SetIngredientAllergens(ctx, id, []int64{glutine, soia}); err != nil {
		t.Fatalf("SetIngredientAllergens: %v", err)
	}

	got, err := cats.GetIngredientAllergenIDs(ctx, id)
	if err != nil {
		t.Fatalf("GetIngredientAllergenIDs: %v", err)
	}
	if !got[glutine] || !got[soia] || len(got) != 2 {
		t.Fatalf("want allergens {glutine, soia}, got %v", got)
	}

	// Replace: setting only glutine must remove soia.
	if err := cats.SetIngredientAllergens(ctx, id, []int64{glutine}); err != nil {
		t.Fatalf("SetIngredientAllergens replace: %v", err)
	}
	got, _ = cats.GetIngredientAllergenIDs(ctx, id)
	if len(got) != 1 || !got[glutine] {
		t.Fatalf("after replace want only {glutine}, got %v", got)
	}

	// An empty list clears the tags.
	if err := cats.SetIngredientAllergens(ctx, id, nil); err != nil {
		t.Fatalf("SetIngredientAllergens clear: %v", err)
	}
	got, _ = cats.GetIngredientAllergenIDs(ctx, id)
	if len(got) != 0 {
		t.Fatalf("after clear want 0 tags, got %v", got)
	}
}

func TestAllergensForIngredientsBatch(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	a, _ := ings.CreateLocal(ctx, "Latte", nil, domain.UnitMilliliter, 64, 3, 5, 3, nil, nil, nil, nil, nil, nil, nil)
	b, _ := ings.CreateLocal(ctx, "Riso", nil, domain.UnitGram, 350, 7, 78, 1, nil, nil, nil, nil, nil, nil, nil)

	lattosio := allergenIDByName(t, cats, "lattosio")
	if err := cats.SetIngredientAllergens(ctx, a, []int64{lattosio}); err != nil {
		t.Fatalf("set: %v", err)
	}
	// b stays without allergens.

	byIng, err := cats.AllergensForIngredients(ctx, []int64{a, b})
	if err != nil {
		t.Fatalf("AllergensForIngredients: %v", err)
	}
	if len(byIng[a]) != 1 || byIng[a][0].Name != "lattosio" {
		t.Fatalf("ingredient a: want [lattosio], got %v", byIng[a])
	}
	if _, ok := byIng[b]; ok {
		t.Fatalf("ingredient b has no allergens and must not appear in the map")
	}

	// Empty slice: no query, empty map.
	empty, err := cats.AllergensForIngredients(ctx, nil)
	if err != nil {
		t.Fatalf("AllergensForIngredients(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("want an empty map, got %v", empty)
	}
}

func TestIngredientIDsByAllergen(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	a, _ := ings.CreateLocal(ctx, "Gamberi", nil, domain.UnitGram, 99, 24, 0, 1, nil, nil, nil, nil, nil, nil, nil)
	b, _ := ings.CreateLocal(ctx, "Cozze", nil, domain.UnitGram, 86, 12, 4, 2, nil, nil, nil, nil, nil, nil, nil)
	c, _ := ings.CreateLocal(ctx, "Mela", nil, domain.UnitGram, 52, 0, 14, 0, nil, nil, nil, nil, nil, nil, nil)

	crostacei := allergenIDByName(t, cats, "crostacei")
	_ = cats.SetIngredientAllergens(ctx, a, []int64{crostacei})
	_ = cats.SetIngredientAllergens(ctx, b, []int64{crostacei})
	// c has no allergens.

	ids, err := cats.IngredientIDsByAllergen(ctx, crostacei)
	if err != nil {
		t.Fatalf("IngredientIDsByAllergen: %v", err)
	}
	if !ids[a] || !ids[b] || ids[c] || len(ids) != 2 {
		t.Fatalf("want {a,b}, got %v", ids)
	}
}
