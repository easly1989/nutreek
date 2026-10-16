package app

import (
	"math"
	"sort"

	"github.com/amon2126/nutreek/internal/domain"
)

// shoppingOtherGroup collects the items whose ingredient has no category.
const shoppingOtherGroup = "Altro"

// shoppingView is the shopping list as the page shows it: items to buy
// grouped by category, items already in the cart apart, and the progress.
// PrintGroups has every item, for the printed sheet.
type shoppingView struct {
	Groups      []shoppingGroup
	PrintGroups []shoppingGroup
	Cart        []shoppingItemView
	Done        int
	Total       int
	Percent     int
}

// shoppingGroupKey orders the groups: categories by sort_order, «Altro» last.
type shoppingGroupKey struct {
	order int
	name  string
	other bool
}

type shoppingGroup struct {
	Name  string
	Items []shoppingItemView
}

type shoppingItemView struct {
	Item   domain.ShoppingListItem
	Pieces int // 0 = the ingredient is not counted in pieces
}

// buildShoppingView groups the items by the ingredient's category (in
// category order, «Altro» last) and sets the checked ones apart.
// Within every group the order of items stays (alphabetical, from LoadFull).
func buildShoppingView(items []domain.ShoppingListItem, cats map[int64]domain.IngredientCategory) shoppingView {
	v := shoppingView{Total: len(items)}
	todo := map[shoppingGroupKey][]shoppingItemView{}
	all := map[shoppingGroupKey][]shoppingItemView{}
	for _, it := range items {
		iv := shoppingItemView{Item: it, Pieces: shoppingPieces(it)}
		k := shoppingGroupKey{name: shoppingOtherGroup, other: true}
		if c, ok := cats[it.IngredientID]; ok {
			k = shoppingGroupKey{order: c.SortOrder, name: c.Name}
		}
		all[k] = append(all[k], iv)
		if it.Checked {
			v.Cart = append(v.Cart, iv)
			v.Done++
			continue
		}
		todo[k] = append(todo[k], iv)
	}
	v.Groups = sortedShoppingGroups(todo)
	v.PrintGroups = sortedShoppingGroups(all)
	if v.Total > 0 {
		v.Percent = v.Done * 100 / v.Total
	}
	return v
}

func sortedShoppingGroups(m map[shoppingGroupKey][]shoppingItemView) []shoppingGroup {
	keys := make([]shoppingGroupKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.other != b.other {
			return b.other
		}
		if a.order != b.order {
			return a.order < b.order
		}
		return a.name < b.name
	})
	out := make([]shoppingGroup, 0, len(keys))
	for _, k := range keys {
		out = append(out, shoppingGroup{Name: k.name, Items: m[k]})
	}
	return out
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
