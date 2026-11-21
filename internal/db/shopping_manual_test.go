package db

import (
	"context"
	"testing"
	"time"
)

// Suggestions come from what the user added by hand before: matched by the
// start of the text or of a word, the most used first, with the last quantity,
// and never from another user.
func TestSuggestManualItems(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	shop := NewShoppingLists(store.DB, NewWeeklyPlans(store.DB))
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	w1, _ := time.Parse("2006-01-02", "2026-10-05")
	w2 := w1.AddDate(0, 0, 7)
	l1, _ := shop.GetOrCreateList(ctx, alice, w1, 1)
	l2, _ := shop.GetOrCreateList(ctx, alice, w2, 1)
	lb, _ := shop.GetOrCreateList(ctx, bob, w1, 1)
	add := func(list, user int64, name string, q float64, unit string) {
		t.Helper()
		if _, err := shop.AddManualItem(ctx, list, user, name, q, unit); err != nil {
			t.Fatal(err)
		}
	}
	add(l1.ID, alice, "Candele", 2, "pz")
	add(l1.ID, alice, "Cannella", 1, "pz")
	add(l2.ID, alice, "candele", 4, "pz")
	add(l2.ID, alice, "Candele profumate", 1, "pz")
	add(l2.ID, alice, "100% cotone", 0, "g")
	add(lb.ID, bob, "Candela di Bob", 1, "pz")

	got, err := shop.SuggestManualItems(ctx, alice, "can", 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%d suggestions, want 3: %+v", len(got), got)
	}
	// The one used twice is first and keeps the quantity of the last time.
	if got[0].Uses != 2 || got[0].Quantity != 4 || got[0].Unit != "pz" || got[0].Description != "candele" {
		t.Errorf("first suggestion = %+v", got[0])
	}
	for _, s := range got {
		if s.Description == "Candela di Bob" {
			t.Error("another user's item suggested")
		}
	}
	// A word that is not the first also matches.
	if got, _ := shop.SuggestManualItems(ctx, alice, "profum", 6); len(got) != 1 || got[0].Description != "Candele profumate" {
		t.Errorf("word match: %+v", got)
	}
	// The limit, and the wildcards of LIKE are plain characters.
	if got, _ := shop.SuggestManualItems(ctx, alice, "can", 1); len(got) != 1 {
		t.Errorf("limit: %d", len(got))
	}
	if got, _ := shop.SuggestManualItems(ctx, alice, "%", 6); len(got) != 0 {
		t.Errorf("a percent sign matched %+v", got)
	}
	if got, _ := shop.SuggestManualItems(ctx, alice, "100%", 6); len(got) != 1 || got[0].Quantity != 0 {
		t.Errorf("an item without quantity: %+v", got)
	}
	if got, _ := shop.SuggestManualItems(ctx, alice, "  ", 6); len(got) != 0 {
		t.Errorf("empty prefix: %+v", got)
	}
}

// The copy sheet offers the earlier lists that have items added by hand, and a
// copy brings them over with their quantity, only the user's own.
func TestCopyManualItems(t *testing.T) {
	store := newTestStore(t)
	users := NewUsers(store.DB)
	shop := NewShoppingLists(store.DB, NewWeeklyPlans(store.DB))
	ctx := context.Background()

	alice, _ := users.Create(ctx, "alice@x.com", "h", "Alice")
	bob, _ := users.Create(ctx, "bob@x.com", "h", "Bob")
	w1, _ := time.Parse("2006-01-02", "2026-10-05")
	w2 := w1.AddDate(0, 0, 7)
	w3 := w2.AddDate(0, 0, 7)
	l1, _ := shop.GetOrCreateList(ctx, alice, w1, 1)
	l2, _ := shop.GetOrCreateList(ctx, alice, w2, 1) // no items by hand
	l3, _ := shop.GetOrCreateList(ctx, alice, w3, 1)
	lb, _ := shop.GetOrCreateList(ctx, bob, w1, 1)
	_, _ = shop.AddManualItem(ctx, l1.ID, alice, "Carta da forno", 2, "pz")
	if _, err := shop.AddUserItem(ctx, l1.ID, alice, "Sale grosso", "un sacchetto"); err != nil {
		t.Fatal(err)
	}
	_, _ = shop.AddManualItem(ctx, lb.ID, bob, "Segreto di Bob", 1, "pz")

	past, err := shop.PastManualLists(ctx, alice, w3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(past) != 1 || past[0].ListID != l1.ID || len(past[0].Items) != 2 || !past[0].WeekStart.Equal(w1) {
		t.Fatalf("past lists = %+v, want only the first week with its 2 items (the empty one and Bob's left out)", past)
	}
	if p, _ := shop.PastManualLists(ctx, alice, w1, 4); len(p) != 0 {
		t.Errorf("nothing is earlier than the first week, got %+v", p)
	}
	_ = l2

	var ids []int64
	for _, it := range past[0].Items {
		ids = append(ids, it.ID)
	}
	bobItems, _ := shop.PastManualLists(ctx, bob, w3, 4)
	ids = append(ids, bobItems[0].Items[0].ID, 99999) // someone else's, and one that does not exist
	n, err := shop.CopyManualItems(ctx, l3.ID, alice, ids)
	if err != nil || n != 2 {
		t.Fatalf("copied %d (%v), want 2", n, err)
	}
	full, _ := shop.LoadFull(ctx, l3.ID, alice)
	if len(full.UserItems) != 2 {
		t.Fatalf("%d items in the list, want 2: %+v", len(full.UserItems), full.UserItems)
	}
	by := map[string]string{}
	for _, u := range full.UserItems {
		by[u.Description] = u.QuantityText
	}
	if by["Carta da forno"] != "2 pz" || by["Sale grosso"] != "un sacchetto" {
		t.Errorf("copied = %v", by)
	}
	// The same text is not added twice: a second copy, or the same text from two lists.
	if n, _ := shop.CopyManualItems(ctx, l3.ID, alice, ids); n != 0 {
		t.Errorf("a second copy added %d items", n)
	}
	// Copying into a list that is not yours, or from the list itself, does nothing.
	if _, err := shop.CopyManualItems(ctx, l3.ID, bob, ids); err != ErrShoppingListNotFound {
		t.Errorf("Bob copies into Alice's list: %v", err)
	}
	if n, _ := shop.CopyManualItems(ctx, l1.ID, alice, ids); n != 0 {
		t.Errorf("copied %d into the list they come from", n)
	}
}
