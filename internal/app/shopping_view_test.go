package app

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func shopItem(id, ingID int64, name string, qty float64, checked bool, unitWeight float64) domain.ShoppingListItem {
	ing := &domain.Ingredient{ID: ingID, Name: name, Unit: domain.UnitGram}
	if unitWeight > 0 {
		ing.UnitWeightG = &unitWeight
	}
	return domain.ShoppingListItem{ID: id, IngredientID: ingID, TotalQuantity: qty, Checked: checked, Ingredient: ing}
}

// Items are grouped by the ingredient's category, in category order; those
// without a category end up in «Altro», last. The checked ones leave the
// groups and go to «Nel carrello».
func TestBuildShoppingView(t *testing.T) {
	items := []domain.ShoppingListItem{
		shopItem(1, 10, "Banana", 240, false, 120),
		shopItem(2, 11, "Ceci", 400, false, 0),
		shopItem(3, 12, "Mela", 450, true, 150),
		shopItem(4, 13, "Sale", 5, false, 0),
		shopItem(5, 14, "Yogurt", 1190, false, 0),
	}
	cats := map[int64]domain.IngredientCategory{
		10: {ID: 1, Name: "Frutta e verdura", SortOrder: 1},
		12: {ID: 1, Name: "Frutta e verdura", SortOrder: 1},
		11: {ID: 3, Name: "Dispensa", SortOrder: 3},
		14: {ID: 2, Name: "Latticini e uova", SortOrder: 2},
	}

	v := buildShoppingView(items, cats)

	var names []string
	for _, g := range v.Groups {
		names = append(names, g.Name)
	}
	want := []string{"Frutta e verdura", "Latticini e uova", "Dispensa", "Altro"}
	if len(names) != len(want) {
		t.Fatalf("groups = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("groups = %v, want %v", names, want)
		}
	}
	if got := v.Groups[0].Items; len(got) != 1 || got[0].Item.ID != 1 {
		t.Errorf("Frutta e verdura = %+v, want only the banana (the apple is checked)", got)
	}
	if got := v.Groups[3].Items; len(got) != 1 || got[0].Item.ID != 4 {
		t.Errorf("Altro = %+v, want the salt", got)
	}
	if len(v.Cart) != 1 || v.Cart[0].Item.ID != 3 {
		t.Errorf("cart = %+v, want the apple", v.Cart)
	}
	// On the printed sheet every item has its own box, checked or not.
	if len(v.PrintGroups) != 4 || len(v.PrintGroups[0].Items) != 2 {
		t.Errorf("print groups = %+v, want 4 with the apple in the first", v.PrintGroups)
	}
	if v.Done != 1 || v.Total != 5 || v.Percent != 20 {
		t.Errorf("progress = %d of %d (%d%%), want 1 of 5 (20%%)", v.Done, v.Total, v.Percent)
	}
}

// A category with every item checked disappears from the groups.
func TestBuildShoppingViewSkipsEmptyGroups(t *testing.T) {
	items := []domain.ShoppingListItem{shopItem(1, 10, "Mela", 300, true, 0)}
	cats := map[int64]domain.IngredientCategory{10: {ID: 1, Name: "Frutta e verdura"}}
	v := buildShoppingView(items, cats)
	if len(v.Groups) != 0 {
		t.Errorf("groups = %+v, want none", v.Groups)
	}
	if v.Percent != 100 {
		t.Errorf("percentage = %d, want 100", v.Percent)
	}
	if empty := buildShoppingView(nil, nil); empty.Percent != 0 || empty.Total != 0 {
		t.Errorf("empty list = %+v", empty)
	}
}

// Ingredients with a weight per piece show the pieces too: whole pieces are
// bought, so a fraction rounds up (but 2.02 stays 2).
func TestShoppingPieces(t *testing.T) {
	cases := []struct {
		qty, unitWeight float64
		want            int
	}{
		{240, 120, 2},
		{242, 120, 2},
		{300, 120, 3},
		{50, 120, 1},
		{240, 0, 0},
		{0, 120, 0},
	}
	for _, c := range cases {
		it := shopItem(1, 1, "x", c.qty, false, c.unitWeight)
		if got := shoppingPieces(it); got != c.want {
			t.Errorf("shoppingPieces(%v g, %v g/pz) = %d, want %d", c.qty, c.unitWeight, got, c.want)
		}
	}
	ml := shopItem(1, 1, "latte", 500, false, 250)
	ml.Ingredient.Unit = domain.UnitMilliliter
	if got := shoppingPieces(ml); got != 2 {
		t.Errorf("pieces count in ml too: %d", got)
	}
}
