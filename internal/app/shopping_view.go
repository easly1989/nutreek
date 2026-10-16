package app

import (
	"math"
	"sort"

	"github.com/amon2126/nutreek/internal/domain"
)

// shoppingOtherGroup raccoglie le voci il cui ingrediente non ha categoria.
const shoppingOtherGroup = "Altro"

// shoppingView e' la lista della spesa come la mostra la pagina: voci da
// prendere raggruppate per categoria, voci gia' nel carrello a parte, e
// l'avanzamento. PrintGroups ha tutte le voci, per il foglio stampato.
type shoppingView struct {
	Groups      []shoppingGroup
	PrintGroups []shoppingGroup
	Cart        []shoppingItemView
	Done        int
	Total       int
	Percent     int
}

// shoppingGroupKey ordina i gruppi: categorie per sort_order, «Altro» in fondo.
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
	Pieces int // 0 = l'ingrediente non si conta a pezzi
}

// buildShoppingView raggruppa le voci per categoria dell'ingrediente
// (nell'ordine delle categorie, «Altro» in fondo) e separa le spuntate.
// Dentro ogni gruppo resta l'ordine di items (alfabetico, da LoadFull).
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

// shoppingPieces converte la quantita' in pezzi interi per gli ingredienti
// che hanno un peso per pezzo: si comprano pezzi interi, quindi si arrotonda
// per eccesso, tollerando i residui di arrotondamento (2,02 pz restano 2).
func shoppingPieces(it domain.ShoppingListItem) int {
	if it.Ingredient == nil || it.Ingredient.UnitWeightG == nil || *it.Ingredient.UnitWeightG <= 0 || it.TotalQuantity <= 0 {
		return 0
	}
	return int(math.Ceil(it.TotalQuantity / *it.Ingredient.UnitWeightG - 0.05))
}
