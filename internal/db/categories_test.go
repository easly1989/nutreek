package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// allergenIDByName cerca l'ID di un allergene seed per nome (i 14 tag sono
// inseriti dalla migrazione 0016).
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
	t.Fatalf("allergene %q non trovato nei seed", name)
	return 0
}

// La lista ingredienti mostra la categoria di ogni riga e il form di modifica
// la deve pre-selezionare (#122): CategoriesForIngredients le carica in batch.
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
		t.Errorf("categoria dello yogurt = %+v, attesa Latticini", got[yogurt])
	}
	if _, ok := got[mela]; ok {
		t.Error("la mela non ha categoria: non deve comparire")
	}
	if empty, err := cats.CategoriesForIngredients(ctx, alice, nil); err != nil || len(empty) != 0 {
		t.Errorf("lista vuota = %v, %v", empty, err)
	}

	// nil toglie la categoria.
	if err := cats.SetIngredientCategory(ctx, alice, yogurt, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, alice, []int64{yogurt}); len(got) != 0 {
		t.Errorf("dopo nil la categoria resta: %v", got)
	}
}

// La categoria e' di ogni utente (#143): la scelta di Bob non tocca quella
// di Alice sullo stesso ingrediente, e una categoria altrui viene rifiutata.
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
		t.Errorf("Alice vede %+v, attesa la sua Frutta", got[mandorle])
	}
	if got, _ := cats.CategoriesForIngredients(ctx, bob, []int64{mandorle}); got[mandorle].ID != snack {
		t.Errorf("Bob vede %+v, atteso il suo Snack", got[mandorle])
	}

	// Bob non puo' usare la categoria di Alice.
	if err := cats.SetIngredientCategory(ctx, bob, mandorle, &frutta); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("categoria altrui: err = %v, atteso ErrCategoryNotFound", err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, bob, []int64{mandorle}); got[mandorle].ID != snack {
		t.Errorf("dopo il rifiuto Bob vede %+v, atteso ancora Snack", got[mandorle])
	}

	// Cancellare una categoria toglie le sue assegnazioni.
	if err := cats.DeleteCategory(ctx, snack, bob); err != nil {
		t.Fatal(err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, bob, []int64{mandorle}); len(got) != 0 {
		t.Errorf("categoria cancellata ancora assegnata: %v", got)
	}
}

// La migrazione 0028 copia la vecchia colonna ingredients.category_id nella
// tabella per utente, a nome di chi possiede la categoria.
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
	// Rilancia l'INSERT della migrazione, preso dal file vero: al momento
	// della migrazione il DB di test era vuoto.
	up, err := migrationsFS.ReadFile("migrations/0028_ingredient_user_categories.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(up), "INSERT OR IGNORE INTO ingredient_user_categories")
	end := strings.Index(string(up)[start:], ";")
	if start < 0 || end < 0 {
		t.Fatal("INSERT di backfill non trovato nella migrazione 0028")
	}
	if _, err := store.DB.ExecContext(ctx, string(up)[start:start+end]); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if got, _ := cats.CategoriesForIngredients(ctx, alice, []int64{mela}); got[mela].ID != frutta {
		t.Errorf("dopo il backfill Alice vede %+v, attesa Frutta", got[mela])
	}
}

func TestAllergensSeedAndSet(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	// I 14 allergeni standard devono essere già presenti.
	tags, err := cats.ListAllergens(ctx)
	if err != nil {
		t.Fatalf("ListAllergens: %v", err)
	}
	if len(tags) != 14 {
		t.Fatalf("attesi 14 allergeni seed, trovati %d", len(tags))
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
		t.Fatalf("allergeni attesi {glutine, soia}, got %v", got)
	}

	// Replace: settare solo glutine deve rimuovere soia.
	if err := cats.SetIngredientAllergens(ctx, id, []int64{glutine}); err != nil {
		t.Fatalf("SetIngredientAllergens replace: %v", err)
	}
	got, _ = cats.GetIngredientAllergenIDs(ctx, id)
	if len(got) != 1 || !got[glutine] {
		t.Fatalf("dopo replace atteso solo {glutine}, got %v", got)
	}

	// Lista vuota azzera i tag.
	if err := cats.SetIngredientAllergens(ctx, id, nil); err != nil {
		t.Fatalf("SetIngredientAllergens clear: %v", err)
	}
	got, _ = cats.GetIngredientAllergenIDs(ctx, id)
	if len(got) != 0 {
		t.Fatalf("dopo clear attesi 0 tag, got %v", got)
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
	// b resta senza allergeni.

	byIng, err := cats.AllergensForIngredients(ctx, []int64{a, b})
	if err != nil {
		t.Fatalf("AllergensForIngredients: %v", err)
	}
	if len(byIng[a]) != 1 || byIng[a][0].Name != "lattosio" {
		t.Fatalf("ingrediente a: atteso [lattosio], got %v", byIng[a])
	}
	if _, ok := byIng[b]; ok {
		t.Fatalf("ingrediente b senza allergeni non deve comparire nella mappa")
	}

	// Slice vuota: nessuna query, mappa vuota.
	empty, err := cats.AllergensForIngredients(ctx, nil)
	if err != nil {
		t.Fatalf("AllergensForIngredients(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("attesa mappa vuota, got %v", empty)
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
	// c senza allergeni.

	ids, err := cats.IngredientIDsByAllergen(ctx, crostacei)
	if err != nil {
		t.Fatalf("IngredientIDsByAllergen: %v", err)
	}
	if !ids[a] || !ids[b] || ids[c] || len(ids) != 2 {
		t.Fatalf("attesi {a,b}, got %v", ids)
	}
}
