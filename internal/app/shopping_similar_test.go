package app

import (
	"testing"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// Two names are similar when they name the same product, not when they share a
// single word.
func TestSimilarNames(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Yogurt greco 0%", "Altoproteico yogurt greco bianco 0% (coppetta 150 g)", true},
		{"Mela", "mela", true},
		{"Pomodori", "Pomodori ciliegini", false},
		{"Pasta", "Pasta integrale", false},
		{"Latte parzialmente scremato", "Latte scremato", true}, // offered; the user keeps them apart if they differ
		{"Pasta integrale", "Pasta integrale bio", true},
		{"Caffè", "Caffe", true},
		{"Olio extravergine d'oliva", "Olio extravergine di oliva", true},
		{"", "Mela", false},
		{"Riso basmati", "Riso integrale", false},
	}
	for _, c := range cases {
		if got := similarNames(c.a, c.b); got != c.want {
			t.Errorf("similarNames(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// A notice goes under the first of two similar rows still to buy: two
// ingredients of the meals offer a choice (the more used is the proposal), an
// item by hand merges into the ingredient of the meals, a pair kept apart is
// not offered, and quantities that do not add up are not offered either.
func TestShoppingViewSimilar(t *testing.T) {
	a := shopItem(1, 10, "Yogurt greco 0%", 400, false, 0)
	b := shopItem(2, 11, "Altoproteico yogurt greco bianco 0%", 150, false, 0)
	pasta := shopItem(3, 12, "Pasta integrale", 500, false, 0)
	manual := shoppingUserItemView{Item: domain.ShoppingUserItem{ID: 7, Description: "Pasta integrale bio", Quantity: 250, Unit: "g"}}
	free := shoppingUserItemView{Item: domain.ShoppingUserItem{ID: 8, Description: "Yogurt greco bianco 0%", QuantityText: "una confezione"}}

	v := buildShoppingView([]domain.ShoppingListItem{a, b, pasta}, []shoppingUserItemView{manual}, nil, db.ShoppingGroupNone, nil)
	v = v.withSimilar(nil, map[int64]int{10: 1, 11: 5})
	var yog, pas *shoppingSimilar
	for _, r := range v.Rows {
		if r.Similar == nil {
			continue
		}
		switch r.Similar.A {
		case "i10", "i11":
			yog = r.Similar
		case "i12":
			pas = r.Similar
		}
	}
	if yog == nil || len(yog.Options) != 2 || !yog.HasSum || yog.Sum != 550 {
		t.Fatalf("yogurt notice = %+v", yog)
	}
	var primary string
	for _, o := range yog.Options {
		if o.Primary {
			primary = o.Keep
		}
	}
	if primary != "i11" {
		t.Errorf("proposed to keep %q, want i11 (the more used)", primary)
	}
	if pas == nil || len(pas.Options) != 1 || pas.Options[0].Keep != "i12" || pas.Options[0].Drop != "u7" || pas.Sum != 750 {
		t.Fatalf("pasta notice = %+v", pas)
	}

	apart := buildShoppingView([]domain.ShoppingListItem{a, b}, nil, nil, db.ShoppingGroupNone, nil).
		withSimilar(map[string]bool{db.PairKey("i10", "i11"): true}, nil)
	for _, r := range apart.Rows {
		if r.Similar != nil {
			t.Errorf("a pair kept apart was offered: %+v", r.Similar)
		}
	}
	noSum := buildShoppingView([]domain.ShoppingListItem{b}, []shoppingUserItemView{free}, nil, db.ShoppingGroupNone, nil).withSimilar(nil, nil)
	for _, r := range noSum.Rows {
		if r.Similar != nil {
			t.Errorf("a free-text quantity does not add up, but was offered: %+v", r.Similar)
		}
	}
}
