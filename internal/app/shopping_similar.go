package app

import (
	"strings"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/macros"
)

// nameStopwords are the words that do not say which product it is.
var nameStopwords = map[string]bool{
	"di": true, "del": true, "della": true, "dei": true, "al": true, "alla": true, "con": true, "senza": true,
	"e": true, "il": true, "la": true, "lo": true, "le": true, "i": true, "gli": true, "un": true, "una": true,
	"bio": true, "biologico": true, "the": true, "of": true, "and": true, "with": true,
	"g": true, "gr": true, "kg": true, "ml": true, "l": true, "cl": true, "pz": true,
}

// nameTokens reduces a product name to the words that identify it: lower case,
// no accents, no numbers, no sizes in brackets, no filler words.
func nameTokens(name string) map[string]bool {
	var b strings.Builder
	depth := 0
	for _, r := range strings.ToLower(name) {
		switch {
		case r == '(':
			depth++
			b.WriteByte(' ')
		case r == ')':
			if depth > 0 {
				depth--
			}
			b.WriteByte(' ')
		case depth > 0:
		case r == 'à' || r == 'á' || r == 'â' || r == 'ä':
			b.WriteByte('a')
		case r == 'è' || r == 'é' || r == 'ê' || r == 'ë':
			b.WriteByte('e')
		case r == 'ì' || r == 'í' || r == 'î' || r == 'ï':
			b.WriteByte('i')
		case r == 'ò' || r == 'ó' || r == 'ô' || r == 'ö':
			b.WriteByte('o')
		case r == 'ù' || r == 'ú' || r == 'û' || r == 'ü':
			b.WriteByte('u')
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	out := map[string]bool{}
	for _, w := range strings.Fields(b.String()) {
		if len(w) > 1 && !nameStopwords[w] {
			out[w] = true
		}
	}
	return out
}

// similarNames says whether two product names look like the same product:
// the same words, or all the words of the shorter one (at least two) are in
// the longer, or most of their words are shared. One shared word is not enough
// («Pasta» and «Pasta integrale» stay apart).
func similarNames(a, b string) bool {
	return similarTokens(nameTokens(a), nameTokens(b))
}

// similarTokens is similarNames on names already reduced by nameTokens.
func similarTokens(ta, tb map[string]bool) bool {
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	shared := 0
	for w := range ta {
		if tb[w] {
			shared++
		}
	}
	if shared == len(ta) {
		return len(ta) >= 2 || len(tb) == 1
	}
	union := len(ta) + len(tb) - shared
	return shared >= 2 && float64(shared)/float64(union) >= 0.67
}

// shoppingMergeOption is one way to merge a similar pair: keep one row, drop the other.
type shoppingMergeOption struct {
	Keep, Drop string // row keys
	Name       string // the name kept, when the user has a choice of two
	Primary    bool   // the proposal: the more used of two ingredients
}

// shoppingSimilar is the notice under a row: it looks like another row.
type shoppingSimilar struct {
	A, B       string // row keys, for «keep them apart»
	OtherName  string
	QtyA, QtyB float64 // quantities, in Unit, when they add up
	Sum        float64
	Unit       string
	HasSum     bool
	Options    []shoppingMergeOption
}

// rowQuantity is a row's quantity as a number and its unit; ok is false when
// the row has none (a free-text manual item).
func rowQuantity(r shoppingRow) (q float64, unit string, ok bool) {
	if r.Auto != nil {
		if r.Auto.Item.Ingredient == nil {
			return 0, "", false
		}
		return r.Auto.Item.TotalQuantity, string(r.Auto.Item.Ingredient.Unit), true
	}
	if r.User.Item.Quantity > 0 && r.User.Item.Unit != "" {
		return r.User.Item.Quantity, r.User.Item.Unit, true
	}
	return 0, "", false
}

// pairQuantities expresses two rows' quantities in one unit when they add up:
// g with g, ml with ml, and pieces of an item added by hand turned into grams
// with the weight per piece of the ingredient of the meals. ok is false when
// they do not add up, in which case the pair is not offered.
func pairQuantities(a, b shoppingRow) (qa, qb float64, unit string, ok bool) {
	ua, ub := "", ""
	var okA, okB bool
	qa, ua, okA = rowQuantity(a)
	qb, ub, okB = rowQuantity(b)
	if !okA || !okB {
		return 0, 0, "", false
	}
	if ua == ub && ua != "pz" {
		return qa, qb, ua, true
	}
	if ua == "pz" && ub == "pz" {
		return qa, qb, "pz", true
	}
	// A manual row in pieces next to a row from the meals that has a piece weight.
	conv := func(manual float64, auto shoppingRow) (float64, bool) {
		if auto.Auto == nil || auto.Auto.Item.Ingredient == nil || auto.Auto.Item.Ingredient.PieceWeight() <= 0 {
			return 0, false
		}
		return macros.Grams(manual, "pz", auto.Auto.Item.Ingredient), true
	}
	if a.Auto != nil && b.User != nil && ub == "pz" {
		if g, ok := conv(qb, a); ok {
			return qa, g, ua, true
		}
	}
	if b.Auto != nil && a.User != nil && ua == "pz" {
		if g, ok := conv(qa, b); ok {
			return g, qb, ub, true
		}
	}
	return 0, 0, "", false
}

// withSimilar adds, under the rows still to buy that look like another row, a
// notice that offers to merge them or keep them apart (#233). A row is in at
// most one notice; pairs the user kept apart are left out. usage says how many
// rows of the plan use each ingredient: of two ingredients, the more used is
// the one proposed to keep.
func (v shoppingView) withSimilar(dismissed map[string]bool, usage map[int64]int) shoppingView {
	used := map[int]bool{}
	if len(v.Rows) > 400 { // a list this long is not a shopping list; skip the pairs
		return v
	}
	tokens := make([]map[string]bool, len(v.Rows))
	for i, r := range v.Rows {
		tokens[i] = nameTokens(r.name)
	}
	for i := range v.Rows {
		if used[i] || v.Rows[i].Ignored {
			continue
		}
		for j := i + 1; j < len(v.Rows); j++ {
			a, b := v.Rows[i], v.Rows[j]
			if used[j] || b.Ignored || dismissed[db.PairKey(a.Key, b.Key)] || !similarTokens(tokens[i], tokens[j]) {
				continue
			}
			qa, qb, unit, ok := pairQuantities(a, b)
			if !ok {
				continue
			}
			n := &shoppingSimilar{A: a.Key, B: b.Key, OtherName: b.name, QtyA: qa, QtyB: qb, Sum: qa + qb, Unit: unit, HasSum: true}
			switch {
			case a.Auto != nil && b.Auto != nil:
				first := usage[a.Auto.Item.IngredientID] >= usage[b.Auto.Item.IngredientID]
				n.Options = []shoppingMergeOption{
					{Keep: a.Key, Drop: b.Key, Name: a.name, Primary: first},
					{Keep: b.Key, Drop: a.Key, Name: b.name, Primary: !first},
				}
			case a.Auto != nil && b.User != nil:
				n.Options = []shoppingMergeOption{{Keep: a.Key, Drop: b.Key, Primary: true}}
			case a.User != nil && b.Auto != nil:
				n.OtherName = b.name
				n.QtyA, n.QtyB = qa, qb
				n.Options = []shoppingMergeOption{{Keep: b.Key, Drop: a.Key, Primary: true}}
			default: // two items added by hand
				n.Options = []shoppingMergeOption{{Keep: a.Key, Drop: b.Key, Primary: true}}
			}
			v.Rows[i].Similar = n
			used[i], used[j] = true, true
			break
		}
	}
	if v.Grouping == db.ShoppingGroupCategory {
		v.Groups = groupRows(v.Rows)
	}
	return v
}
