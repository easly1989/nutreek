package app

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
)

// shoppingView is the shopping list as the page shows it: one list with the
// items from the meals and the ones added by hand, sorted by name; the ones
// already in the cart apart; and the progress. The printed sheet has every row.
type shoppingView struct {
	Rows    []shoppingRow // still to buy
	Cart    []shoppingRow // checked
	Print   []shoppingRow // every row, checked or not
	Done    int
	Total   int
	Percent int
}

// shoppingRow is a line of the list: either an item computed from the meals
// (Auto) or one added by hand (User).
type shoppingRow struct {
	Auto     *shoppingItemView
	User     *shoppingUserItemView
	Category string // the ingredient's category; empty = «Altro» (also every manual item)
	Pieces   int    // 0 = not counted in pieces
	Amount   string // manual items: the quantity in g or ml, empty when it is in pieces
	name     string
	id       int64
}

type shoppingItemView struct {
	Item domain.ShoppingListItem
}

// buildShoppingView merges the computed and the manual items in one list
// sorted by name, and sets the checked ones apart.
func buildShoppingView(items []domain.ShoppingListItem, user []shoppingUserItemView, cats map[int64]domain.IngredientCategory) shoppingView {
	var rows []shoppingRow
	for _, it := range items {
		name := ""
		if it.Ingredient != nil {
			name = it.Ingredient.Name
		}
		rows = append(rows, shoppingRow{Auto: &shoppingItemView{Item: it}, Category: cats[it.IngredientID].Name, Pieces: shoppingPieces(it), name: name, id: it.ID})
	}
	for i := range user {
		u := user[i]
		pieces, amount := userItemQuantity(u.Item.QuantityText)
		rows = append(rows, shoppingRow{User: &u, Pieces: pieces, Amount: amount, name: u.Item.Description, id: u.Item.ID})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := strings.ToLower(rows[i].name), strings.ToLower(rows[j].name)
		if a != b {
			return a < b
		}
		return rows[i].id < rows[j].id
	})

	v := shoppingView{Total: len(rows), Print: rows}
	for _, r := range rows {
		if r.checked() {
			v.Cart = append(v.Cart, r)
			v.Done++
			continue
		}
		v.Rows = append(v.Rows, r)
	}
	if v.Total > 0 {
		v.Percent = v.Done * 100 / v.Total
	}
	return v
}

func (r shoppingRow) checked() bool {
	if r.Auto != nil {
		return r.Auto.Item.Checked
	}
	return r.User.Item.Checked
}

// userItemQuantity splits a manual item's «3 pz» / «250 g» text: pieces go in
// the pieces column, anything else stays as typed in the amount column.
func userItemQuantity(text string) (pieces int, amount string) {
	num, unit, ok := strings.Cut(strings.TrimSpace(text), " ")
	if ok && unit == "pz" {
		if q, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", "."), 64); err == nil && q > 0 {
			return int(math.Ceil(q - 0.05)), ""
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
