package app

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/stores"
)

// shoppingView is the shopping list as the page shows it: one list with the
// items from the meals and the ones added by hand, sorted by name; the ones
// already in the cart apart; and the progress. The printed sheet has every row.
type shoppingView struct {
	Rows []shoppingRow // still to buy
	// Skipped is what the user set aside («not needed»): out of the list, in its own
	// section, not counted.
	Skipped []shoppingRow
	// Groups is Rows grouped by category, in category order and «Altro» last;
	// it is only filled when the user chose to group by category.
	Grouping string
	Groups   []shoppingGroup
	Cart     []shoppingRow // checked
	// SkippedPreview and CartPreview are the first names of each section, shown under its
	// closed heading.
	SkippedPreview shoppingPreview
	CartPreview    shoppingPreview
	Print          []shoppingRow // every row, checked or not
	Done           int
	Total          int
	Percent        int
	Rest           int // 100 - Percent: what the progress bar still has to fill
	Prices         shoppingPrices
}

// shoppingPreview is what a closed section shows of its rows: the first names and
// how many more there are.
type shoppingPreview struct {
	Names string
	More  int
}

const shoppingPreviewNames = 3

func previewOf(rows []shoppingRow) shoppingPreview {
	var names []string
	for i, r := range rows {
		if i == shoppingPreviewNames {
			break
		}
		names = append(names, r.name)
	}
	return shoppingPreview{Names: strings.Join(names, ", "), More: max(0, len(rows)-shoppingPreviewNames)}
}

// shoppingPrice is the estimate of a row from the store product an ingredient was
// made from (#239): whole packages of it, priced when the store was last asked.
type shoppingPrice struct {
	Cents       int
	Currency    string
	Packs       int
	Pack        string // the package, "700 g"
	Store       string
	Unavailable bool
}

// shoppingPrices is the estimate of the whole list: the rows that count (not the
// ones set aside) with a price, out of those that could have one.
type shoppingPrices struct {
	Total    int
	Currency string
	Priced   int
	Rows     int
	Stores   string // the stores the prices come from, "A, B"
	Any      bool   // at least one ingredient is linked to a store product: show the estimate
}

// withPrices fills the price of the rows computed from the meals that have a store
// product, and the estimate of the list. links is by ingredient id.
func (v shoppingView) withPrices(links map[int64]db.StoreLink) shoppingView {
	if len(links) == 0 {
		return v
	}
	price := func(row *shoppingRow) {
		if row.Auto == nil || row.Auto.Item.Ingredient == nil {
			return
		}
		l, ok := links[row.Auto.Item.IngredientID]
		if !ok || !l.HasPrice {
			return
		}
		ing := row.Auto.Item.Ingredient
		var pieceG float64
		if ing.UnitWeightG != nil {
			pieceG = *ing.UnitWeightG
		}
		cents, packs, ok := stores.Estimate(row.Auto.Item.TotalQuantity, string(ing.Unit), pieceG, l.SizeQty, l.SizeUnit, l.PriceCents)
		if !ok {
			return
		}
		pack := strconv.FormatFloat(l.SizeQty, 'f', -1, 64) + " " + l.SizeUnit
		row.Price = &shoppingPrice{Cents: cents, Currency: l.Currency, Packs: packs, Pack: pack, Store: l.PluginName, Unavailable: !l.Available}
	}
	fill := func(rows []shoppingRow) {
		for i := range rows {
			price(&rows[i])
		}
	}
	fill(v.Rows)
	fill(v.Skipped)
	fill(v.Cart)
	fill(v.Print)
	for g := range v.Groups {
		fill(v.Groups[g].Rows)
	}
	var names []string
	seen := map[string]bool{}
	for _, row := range v.Print {
		if row.Auto == nil || row.Ignored {
			continue
		}
		v.Prices.Rows++
		if row.Price == nil || (v.Prices.Currency != "" && row.Price.Currency != v.Prices.Currency) {
			continue
		}
		v.Prices.Currency = row.Price.Currency
		v.Prices.Total += row.Price.Cents
		v.Prices.Priced++
		if !seen[row.Price.Store] {
			seen[row.Price.Store] = true
			names = append(names, row.Price.Store)
		}
	}
	v.Prices.Stores = strings.Join(names, ", ")
	v.Prices.Any = v.Prices.Priced > 0
	return v
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
	Legacy      bool             // manual item with a free-text quantity, from before units
	ShowCat     bool             // the row names its category (in the user's own order, and in the cart when the list is grouped; never when «Altro» is the only one)
	Reorder     bool             // the row can be dragged (the user's own order)
	Ignored     bool             // greyed out and not counted (#230)
	Similar     *shoppingSimilar // the notice under the row: it looks like another row (#233)
	Key         string           // the row's key in the stored order: «i<ingredient id>» or «u<id>»
	Shares      []shoppingShare  // who needs how much of it, when the account has a household (#238)
	Price       *shoppingPrice   // what the linked store product would cost, when there is one (#239)
	name        string
	id          int64
	catOrder    int
}

type shoppingItemView struct {
	Item domain.ShoppingListItem
}

// shoppingShare is what one person needs of an ingredient.
type shoppingShare struct {
	Name  string
	Color int
	Qty   float64
	Unit  string
}

// withShares fills the per-person detail of the rows computed from the meals: the
// account owner first, then the household in order; people who need none are left out.
func (v shoppingView) withShares(people []db.HouseholdPerson, ownerName string, shares map[int64]map[int64]float64) shoppingView {
	if len(people) == 0 || len(shares) == 0 {
		return v
	}
	fill := func(rows []shoppingRow) {
		for i := range rows {
			if rows[i].Auto == nil || rows[i].Auto.Item.Ingredient == nil {
				continue
			}
			byPerson := shares[rows[i].Auto.Item.IngredientID]
			// The details are as old as the last generation: when they no longer add up to
			// the row (its quantity was edited, a person came or went), show none.
			var sum float64
			for _, q := range byPerson {
				sum += q
			}
			if math.Abs(sum-rows[i].Auto.Item.TotalQuantity) > 0.01 {
				continue
			}
			unit := string(rows[i].Auto.Item.Ingredient.Unit)
			rows[i].Shares = rows[i].Shares[:0:0]
			if q := byPerson[0]; q > 0 {
				rows[i].Shares = append(rows[i].Shares, shoppingShare{Name: ownerName, Qty: q, Unit: unit})
			}
			for k, p := range people {
				if q := byPerson[p.ID]; q > 0 {
					rows[i].Shares = append(rows[i].Shares, shoppingShare{Name: p.Name, Color: k + 1, Qty: q, Unit: unit})
				}
			}
		}
	}
	fill(v.Rows)
	fill(v.Skipped)
	fill(v.Cart)
	fill(v.Print)
	for g := range v.Groups {
		fill(v.Groups[g].Rows)
	}
	return v
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
		row := shoppingRow{Auto: &shoppingItemView{Item: it}, Ignored: it.Ignored, Category: cats[it.IngredientID].Name, catOrder: cats[it.IngredientID].SortOrder, Key: db.RowKeyIngredient(it.IngredientID), Pieces: shoppingPieces(it), name: name, id: it.ID}
		row.EditQty, row.EditUnit, row.PieceWeight = autoEditor(it)
		rows = append(rows, row)
	}
	for i := range user {
		u := user[i]
		pieces, amount := userItemQuantity(u.Item.QuantityText)
		row := shoppingRow{User: &u, Ignored: u.Item.Ignored, Pieces: pieces, Amount: amount, name: u.Item.Description, id: u.Item.ID, Key: db.RowKeyUserItem(u.Item.ID),
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

	v := shoppingView{Grouping: grouping}
	// A category under a row says something only when the list has a category besides «Altro».
	named := false
	for _, r := range rows {
		named = named || r.Category != ""
	}
	for i := range rows {
		rows[i].ShowCat = named && grouping == db.ShoppingGroupCustom      // with no grouping a category under each row says nothing
		rows[i].Reorder = custom && !rows[i].checked() && !rows[i].Ignored // the rows in the cart and the ones set aside are not draggable
	}
	for _, r := range rows {
		if r.Ignored {
			v.Skipped = append(v.Skipped, r) // set aside in its own section; not counted
			continue
		}
		v.Total++
		v.Print = append(v.Print, r)
		if r.checked() {
			r.ShowCat = named && grouping != db.ShoppingGroupNone // the cart has no group headings: its rows name their category
			v.Cart = append(v.Cart, r)
			v.Done++
			continue
		}
		v.Rows = append(v.Rows, r)
	}
	v.SkippedPreview, v.CartPreview = previewOf(v.Skipped), previewOf(v.Cart)
	if v.Total > 0 {
		v.Percent = v.Done * 100 / v.Total
		v.Rest = 100 - v.Percent
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
