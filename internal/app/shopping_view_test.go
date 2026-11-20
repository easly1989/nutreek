package app

import (
	"strings"
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

// Computed and manual items form one list sorted by name; the checked ones
// leave it and go to the cart, whatever their kind.
func TestBuildShoppingView(t *testing.T) {
	items := []domain.ShoppingListItem{
		shopItem(1, 10, "Banana", 240, false, 120),
		shopItem(2, 11, "ceci", 400, false, 0),
		shopItem(3, 12, "Mela", 450, true, 150),
	}
	cats := map[int64]domain.IngredientCategory{
		10: {ID: 1, Name: "Frutta e verdura"},
		11: {ID: 3, Name: "Dispensa"},
	}
	user := []shoppingUserItemView{
		{Item: domain.ShoppingUserItem{ID: 1, Description: "Carta da forno", QuantityText: "2 pz"}},
		{Item: domain.ShoppingUserItem{ID: 2, Description: "Sale grosso", QuantityText: "1 kg", Checked: true}},
	}

	v := buildShoppingView(items, user, cats)

	var names []string
	for _, r := range v.Rows {
		names = append(names, r.name)
	}
	if want := []string{"Banana", "Carta da forno", "ceci"}; strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v, want %v", names, want)
	}
	if len(v.Cart) != 2 || v.Cart[0].name != "Mela" || v.Cart[1].name != "Sale grosso" {
		t.Errorf("cart = %+v, want the apple and the salt", v.Cart)
	}
	if len(v.Print) != 5 {
		t.Errorf("print = %d rows, want every one of the 5", len(v.Print))
	}
	if v.Rows[0].Category != "Frutta e verdura" || v.Rows[2].Category != "Dispensa" {
		t.Errorf("categories = %q, %q", v.Rows[0].Category, v.Rows[2].Category)
	}
	if v.Rows[0].Pieces != 2 {
		t.Errorf("banana pieces = %d, want 2", v.Rows[0].Pieces)
	}
	if v.Rows[1].Pieces != 2 || v.Rows[1].Amount != "" {
		t.Errorf("manual in pieces = %d pz, amount %q, want 2 and none", v.Rows[1].Pieces, v.Rows[1].Amount)
	}
	if v.Done != 2 || v.Total != 5 || v.Percent != 40 {
		t.Errorf("progress = %d of %d (%d%%), want 2 of 5 (40%%)", v.Done, v.Total, v.Percent)
	}
	if empty := buildShoppingView(nil, nil, nil); empty.Percent != 0 || empty.Total != 0 {
		t.Errorf("empty list = %+v", empty)
	}
}

// A manual quantity in pieces goes to the pieces column; anything else stays
// as typed in the amount column.
func TestUserItemQuantity(t *testing.T) {
	cases := []struct {
		text       string
		wantPieces int
		wantAmount string
	}{
		{"3 pz", 3, ""},
		{"2,5 pz", 3, ""},
		{"250 g", 0, "250 g"},
		{"1 kg", 0, "1 kg"},
		{"", 0, ""},
	}
	for _, c := range cases {
		if p, a := userItemQuantity(c.text); p != c.wantPieces || a != c.wantAmount {
			t.Errorf("userItemQuantity(%q) = %d, %q; want %d, %q", c.text, p, a, c.wantPieces, c.wantAmount)
		}
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
