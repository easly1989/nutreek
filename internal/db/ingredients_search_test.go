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
	// Eight words count; a ninth, which matches nothing, is dropped.
	if got := search(alice, "yogurt greco fage latticini yo gre fa lat zzzz"); len(got) != 1 {
		t.Errorf("a ninth word must be dropped: %v", got)
	}
	if got := search(alice, "yogurt greco fage latticini yo gre fa zzzz lat"); len(got) != 0 {
		t.Errorf("a word among the first eight that matches nothing must exclude: %v", got)
	}
	// An exact name comes first, whatever the alphabet says.
	mk("Banana pane", nil)
	mk("Pane", nil)
	if got := search(alice, "pane"); len(got) < 2 || got[0] != "Pane" {
		t.Errorf("exact name first: %v", got)
	}
}
