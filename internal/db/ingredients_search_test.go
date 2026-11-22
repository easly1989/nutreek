package db

import (
	"context"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// A search matches every word of the query against the name, the brand or the
// viewer's own category of the ingredient, in any order (#224).
func TestSearchActiveEveryWord(t *testing.T) {
	store := newTestStore(t)
	ings := NewIngredients(store.DB)
	users := NewUsers(store.DB)
	cats := NewCategories(store.DB)
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	fage, mala := "Fage", "Malaga"
	mk := func(name string, brand *string) int64 {
		id, err := ings.CreateLocal(ctx, name, brand, domain.UnitGram, 100, 5, 20, 1, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("CreateLocal %q: %v", name, err)
		}
		return id
	}
	greco := mk("Yogurt greco 0%", &fage)
	mk("Yogurt bianco", &mala)
	mk("Pane 100% integrale", nil)
	dairy, err := cats.CreateCategory(ctx, alice, "Latticini")
	if err != nil {
		t.Fatal(err)
	}
	if err := cats.SetIngredientCategory(ctx, alice, greco, &dairy); err != nil {
		t.Fatal(err)
	}

	search := func(viewer int64, q string) []string {
		t.Helper()
		got, err := ings.SearchActive(ctx, viewer, q, 20)
		if err != nil {
			t.Fatalf("SearchActive %q: %v", q, err)
		}
		return names(got)
	}
	for _, tc := range []struct {
		q    string
		want int
	}{
		{"yogurt fage", 1},      // name, then brand
		{"fage yogurt", 1},      // brand first
		{"gre fa", 1},           // prefixes of both
		{"yogurt latticini", 1}, // name and the viewer's category
		{"fage latticini", 1},   // brand and category
		{"yogurt", 2},
		{"yogurt nessuno", 0}, // every word must match
		{"yogurt, fage!", 1},  // punctuation only separates
		{"100%", 1},           // a percent sign is text
		{"%", 2},              // and so is a lone one
		{"fage malaga", 0},
	} {
		if got := search(alice, tc.q); len(got) != tc.want {
			t.Errorf("%q: %d results %v, want %d", tc.q, len(got), got, tc.want)
		}
	}
	// A category belongs to the user who made it: Bob does not match on Alice's.
	if got := search(bob, "latticini"); len(got) != 0 {
		t.Errorf("Bob found Alice's category: %v", got)
	}
	// More words than count are cut, not an error.
	if got := search(alice, "yogurt greco fage a b c d e f g h i j"); len(got) != 0 && len(got) != 1 {
		t.Errorf("many words: %v", got)
	}
}
