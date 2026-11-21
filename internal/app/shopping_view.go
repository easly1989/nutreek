package app

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// shoppingView is the shopping list as the page shows it: one list with the
// items from the meals and the ones added by hand, sorted by name; the ones
// already in the cart apart; and the progress. The printed sheet has every row.
type shoppingView struct {
	Rows []shoppingRow // still to buy
	// Groups is Rows grouped by category, in category order and «Altro» last;
	// it is only filled when the user chose to group by category.
	Grouping string
	Groups   []shoppingGroup
	Cart     []shoppingRow // checked
	Print    []shoppingRow // every row, checked or not
	Done     int
	Total    int
	Percent  int
}

// shoppingGroup is a category of the list.
type shoppingGroup struct {
	Name  string // the category's name; for Other the template shows a translated label
	Other bool
	Rows  []shoppingRow
}

// shoppingRow is a line of the list: either an item computed from the meals
// (Auto) or one added by hand (User).
type shoppingRow struct {
	Auto     *shoppingItemView
	User     *shoppingUserItemView
	Category string // the ingredient's category; empty = «Altro» (also every manual item)
	Pieces   int    // 0 = not counted in pieces
	Amount   string // manual items: the quantity in g or ml, empty when it is in pieces
	// The quantity editor opens on EditQty in EditUnit; PieceWeight is the
	// weight of a piece of an item from the meals (0 = no pieces offered).
	EditQty     float64
	EditUnit    string
	PieceWeight float64
	Legacy      bool   // manual item with a free-text quantity, from before units
	ShowCat     bool   // the row names its category (only when the list is not grouped)
	Reorder     bool   // the row can be dragged (the user's own order)
	Key         string // the row's key in the stored order: «i<ingredient id>» or «u<id>»
	name        string
	id          int64
	catOrder    int
}

type shoppingItemView struct {
	Item domain.ShoppingListItem
}

// buildShoppingView merges the computed and the manual items in one list
// sorted by name, and sets the checked ones apart.
func buildShoppingView(items []domain.ShoppingListItem, user []shoppingUserItemView, cats map[int64]domain.IngredientCategory, grouping string, order map[string]int) shoppingView {
	var rows []shoppingRow
	for _, it := range items {
		name := ""
		if it.Ingredient != nil {
			name = it.Ingredient.Name
		}
		row := shoppingRow{Auto: &shoppingItemView{Item: it}, Category: cats[it.IngredientID].Name, catOrder: cats[it.IngredientID].SortOrder, Key: db.RowKeyIngredient(it.IngredientID), Pieces: shoppingPieces(it), name: name, id: it.ID}
		row.EditQty, row.EditUnit, row.PieceWeight = autoEditor(it)
		rows = append(rows, row)
	}
	for i := range user {
		u := user[i]
		pieces, amount := userItemQuantity(u.Item.QuantityText)
		row := shoppingRow{User: &u, Pieces: pieces, Amount: amount, name: u.Item.Description, id: u.Item.ID, Key: db.RowKeyUserItem(u.Item.ID),
			EditQty: u.Item.Quantity, EditUnit: u.Item.Unit, Legacy: u.Item.Unit == "" && u.Item.QuantityText != ""}
		if row.EditUnit == "" {
			row.EditUnit = "g"
		}
		rows = append(rows, row)
	}
	custom := grouping == db.ShoppingGroupCustom
	sort.SliceStable(rows, func(i, j int) bool {
		if custom {
			// The user's own order first; rows they never placed after them, by name.
			pi, oi := order[rows[i].Key]
			pj, oj := order[rows[j].Key]
			if oi != oj {
				return oi
			}
			if oi && pi != pj {
				return pi < pj
			}
		}
		a, b := strings.ToLower(rows[i].name), strings.ToLower(rows[j].name)
		if a != b {
			return a < b
		}
		return rows[i].id < rows[j].id
	})

	v := shoppingView{Total: len(rows), Print: rows, Grouping: grouping}
	for i := range rows {
		rows[i].ShowCat = grouping != db.ShoppingGroupCategory
		rows[i].Reorder = custom && !rows[i].checked() // the rows in the cart are not draggable
	}
	for _, r := range rows {
		if r.checked() {
			r.ShowCat = true // the cart has no group headings: its rows name their category
			v.Cart = append(v.Cart, r)
			v.Done++
			continue
		}
		v.Rows = append(v.Rows, r)
	}
	if v.Total > 0 {
		v.Percent = v.Done * 100 / v.Total
	}
	if grouping == db.ShoppingGroupCategory {
		v.Groups = groupRows(v.Rows)
	}
	return v
}

// groupRows groups rows by category: categories in their order, «Altro» (no
// category) last; the rows keep their order, by name.
func groupRows(rows []shoppingRow) []shoppingGroup {
	byName := map[string]*shoppingGroup{}
	var order []*shoppingGroup
	for _, r := range rows {
		g := byName[r.Category]
		if g == nil {
			g = &shoppingGroup{Name: r.Category, Other: r.Category == ""}
			byName[r.Category] = g
			order = append(order, g)
		}
		g.Rows = append(g.Rows, r)
	}
	pos := func(g *shoppingGroup) int {
		if g.Other {
			return math.MaxInt
		}
		return g.Rows[0].catOrder
	}
	sort.SliceStable(order, func(i, j int) bool {
		if pi, pj := pos(order[i]), pos(order[j]); pi != pj {
			return pi < pj
		}
		return order[i].Name < order[j].Name
	})
	out := make([]shoppingGroup, 0, len(order))
	for _, g := range order {
		out = append(out, *g)
	}
	return out
}

func (r shoppingRow) checked() bool {
	if r.Auto != nil {
		return r.Auto.Item.Checked
	}
	return r.User.Item.Checked
}

// autoEditor says what the quantity editor of an item from the meals opens
// on: pieces when the quantity is a whole number of them (as the row shows
// «5 pcs»), the ingredient's unit otherwise. It also returns the weight of a
// piece, 0 when the ingredient has none.
func autoEditor(it domain.ShoppingListItem) (qty float64, unit string, pieceWeight float64) {
	qty, unit = it.TotalQuantity, "g"
	if it.Ingredient == nil {
		return qty, unit, 0
	}
	unit = string(it.Ingredient.Unit)
	pieceWeight = it.Ingredient.PieceWeight()
	if pieceWeight > 0 && it.TotalQuantity > 0 {
		// Exact to the gram, so saving without a change keeps the quantity.
		if n := math.Round(it.TotalQuantity / pieceWeight); n >= 1 && math.Abs(it.TotalQuantity-n*pieceWeight) < 0.05 {
			return n, "pz", pieceWeight
		}
	}
	return qty, unit, pieceWeight
}

// userItemQuantity splits a manual item's «3 pz» / «250 g» text: pieces go in
// the pieces column, anything else stays as typed in the amount column.
func userItemQuantity(text string) (pieces int, amount string) {
	num, unit, ok := strings.Cut(strings.TrimSpace(text), " ")
	if ok && unit == "pz" {
		if q, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", "."), 64); err == nil && q > 0 {
			return max(1, int(math.Ceil(q-0.05))), ""
		}
	}
	return 0, text
}

// shoppingPieces turns the quantity into whole pieces for the ingredients
// that have a weight per piece: whole pieces are bought, so it rounds up,
// tolerating rounding residue (2.02 pz stays 2).
func shoppingPieces(it domain.ShoppingListItem) int {
	if it.Ingredient == nil || it.Ingredient.UnitWeightG == nil || *it.Ingredient.UnitWeightG <= 0 || it.TotalQuantity <= 0 {
		return 0
	}
	return int(math.Ceil(it.TotalQuantity / *it.Ingredient.UnitWeightG - 0.05))
}
