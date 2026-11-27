package app

import (
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/db"
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

	v := buildShoppingView(items, user, cats, db.ShoppingGroupNone, nil)

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
	if empty := buildShoppingView(nil, nil, nil, db.ShoppingGroupNone, nil); empty.Percent != 0 || empty.Total != 0 {
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
		{"0.04 pz", 1, ""},
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

// The quantity editor opens in pieces when the quantity is a whole number of
// them, as the row shows, and in the ingredient's unit otherwise.
func TestAutoEditor(t *testing.T) {
	cases := []struct {
		name            string
		qty, unitWeight float64
		wantQty         float64
		wantUnit        string
		wantWeight      float64
	}{
		{"whole pieces", 1000, 200, 5, "pz", 200},
		{"not whole", 1190, 120, 1190, "g", 120},
		{"less than one piece", 50, 120, 50, "g", 120},
		{"no piece weight", 400, 0, 400, "g", 0},
	}
	for _, c := range cases {
		qty, unit, weight := autoEditor(shopItem(1, 1, "x", c.qty, false, c.unitWeight))
		if qty != c.wantQty || unit != c.wantUnit || weight != c.wantWeight {
			t.Errorf("%s: got %v %s (piece %v), want %v %s (piece %v)", c.name, qty, unit, weight, c.wantQty, c.wantUnit, c.wantWeight)
		}
	}
	ml := shopItem(1, 1, "latte", 500, false, 250)
	ml.Ingredient.Unit = domain.UnitMilliliter
	if _, unit, _ := autoEditor(ml); unit != "pz" {
		t.Errorf("500 ml of a 250 ml piece: unit %q, want pz", unit)
	}
}

// Grouping by category puts the rows under their category, in category order
// and «Altro» last, manual items included; without it there are no groups and
// the rows name their category.
func TestShoppingViewGrouping(t *testing.T) {
	items := []domain.ShoppingListItem{
		shopItem(1, 10, "Zucchine", 400, false, 0),
		shopItem(2, 11, "Pasta", 500, false, 0),
		shopItem(3, 12, "Mela", 450, false, 0),
	}
	cats := map[int64]domain.IngredientCategory{
		10: {ID: 1, Name: "Frutta e verdura", SortOrder: 1},
		12: {ID: 1, Name: "Frutta e verdura", SortOrder: 1},
		11: {ID: 3, Name: "Dispensa", SortOrder: 3},
	}
	user := []shoppingUserItemView{{Item: domain.ShoppingUserItem{ID: 1, Description: "Candele"}}}

	v := buildShoppingView(items, user, cats, db.ShoppingGroupCategory, nil)
	var names []string
	for _, g := range v.Groups {
		names = append(names, g.Name)
	}
	if want := "Frutta e verdura,Dispensa,"; strings.Join(names, ",") != want {
		t.Errorf("groups = %q, want %q", strings.Join(names, ","), want)
	}
	if !v.Groups[2].Other || len(v.Groups[2].Rows) != 1 || v.Groups[2].Rows[0].name != "Candele" {
		t.Errorf("Altro = %+v, want the manual candles", v.Groups[2])
	}
	if got := v.Groups[0].Rows; len(got) != 2 || got[0].name != "Mela" || got[1].name != "Zucchine" {
		t.Errorf("Frutta e verdura rows = %+v, want Mela then Zucchine", got)
	}
	if v.Rows[0].ShowCat {
		t.Error("a grouped row must not repeat its category")
	}

	flat := buildShoppingView(items, user, cats, db.ShoppingGroupNone, nil)
	if len(flat.Groups) != 0 || flat.Rows[0].ShowCat {
		t.Errorf("not grouped: groups %d, ShowCat %v; a category under each row says nothing", len(flat.Groups), flat.Rows[0].ShowCat)
	}

	// The cart has no group headings: its rows name their category unless the list is not grouped.
	cart := []domain.ShoppingListItem{shopItem(1, 10, "Zucchine", 400, true, 0)}
	for grouping, want := range map[string]bool{db.ShoppingGroupNone: false, db.ShoppingGroupCategory: true, db.ShoppingGroupCustom: true} {
		if v := buildShoppingView(cart, nil, cats, grouping, nil); len(v.Cart) != 1 || v.Cart[0].ShowCat != want {
			t.Errorf("%s: the cart row's ShowCat = %v, want %v", grouping, v.Cart[0].ShowCat, want)
		}
	}
}

// In the user's own order the rows the user placed come first, in their order,
// and the ones never placed follow by name; the rows can be dragged.
func TestShoppingViewCustomOrder(t *testing.T) {
	items := []domain.ShoppingListItem{
		shopItem(1, 10, "Zucchine", 400, false, 0),
		shopItem(2, 11, "Pasta", 500, false, 0),
		shopItem(3, 12, "Mela", 450, false, 0),
	}
	user := []shoppingUserItemView{{Item: domain.ShoppingUserItem{ID: 7, Description: "Candele"}}}
	order := map[string]int{"i10": 0, "u7": 1, "i11": 2} // Zucchine, Candele, Pasta; Mela unplaced

	v := buildShoppingView(items, user, nil, db.ShoppingGroupCustom, order)
	var names []string
	for _, r := range v.Rows {
		names = append(names, r.name)
	}
	if got, want := strings.Join(names, ","), "Zucchine,Candele,Pasta,Mela"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if !v.Rows[0].Reorder || v.Rows[0].Key != "i10" || v.Rows[1].Key != "u7" {
		t.Errorf("rows = %+v", v.Rows[:2])
	}
	if flat := buildShoppingView(items, user, nil, db.ShoppingGroupNone, order); flat.Rows[0].Reorder || flat.Rows[0].name != "Candele" {
		t.Errorf("not custom: first %q, Reorder %v", flat.Rows[0].name, flat.Rows[0].Reorder)
	}
}

// Only the rows still to buy can be dragged; the ones in the cart cannot.
func TestShoppingViewCustomCartNotDraggable(t *testing.T) {
	items := []domain.ShoppingListItem{shopItem(1, 10, "Mela", 100, true, 0), shopItem(2, 11, "Pasta", 100, false, 0)}
	v := buildShoppingView(items, nil, nil, db.ShoppingGroupCustom, nil)
	if len(v.Cart) != 1 || v.Cart[0].Reorder || !v.Rows[0].Reorder {
		t.Errorf("cart %+v rows %+v", v.Cart, v.Rows)
	}
}

// An ignored row stays in its place, in the list, and does not count in the
// progress or on the printed sheet, checked or not.
func TestShoppingViewIgnored(t *testing.T) {
	a := shopItem(1, 10, "Mela", 100, false, 0)
	b := shopItem(2, 11, "Pasta", 100, true, 0)
	b.Ignored = true // ignored while checked: it still stays in the list, not in the cart
	c := shopItem(3, 12, "Sale", 100, true, 0)
	v := buildShoppingView([]domain.ShoppingListItem{a, b, c}, nil, nil, db.ShoppingGroupNone, nil)
	if len(v.Rows) != 2 || v.Rows[1].name != "Pasta" || !v.Rows[1].Ignored {
		t.Fatalf("rows = %+v, want Mela and the ignored Pasta", v.Rows)
	}
	if len(v.Cart) != 1 || v.Cart[0].name != "Sale" {
		t.Errorf("cart = %+v, want only Sale", v.Cart)
	}
	if v.Total != 2 || v.Done != 1 || len(v.Print) != 2 {
		t.Errorf("total %d, done %d, print %d: want 2, 1, 2", v.Total, v.Done, len(v.Print))
	}
}
