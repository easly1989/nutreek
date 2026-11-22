package macros

import (
	"math"
	"sort"

	"github.com/amon2126/nutreek/internal/domain"
)

// ReplacementQuantity is the quantity and unit a row keeps when its ingredient
// is replaced (#227): the same weight. A row in pieces becomes grams, because
// the new ingredient may weigh a piece differently or have no piece weight; a
// row in ml stays in ml.
func ReplacementQuantity(quantity float64, unit string, old *domain.Ingredient) (float64, string) {
	grams := Grams(quantity, unit, old)
	if unit == "ml" {
		return grams, "ml"
	}
	return grams, "g"
}

// Replacement is one ingredient suggested in place of the current one, with
// what the row would be at the same weight.
type Replacement struct {
	Ingredient   *domain.Ingredient
	SameCategory bool
	Macros       Macros // the row's macros with this ingredient
	Diff         Macros // Macros minus the current row's
	Distance     float64
}

// ReplacementDistance says how far two rows are, in kcal: the difference in
// energy plus the difference of each macro weighed by its energy per gram (4, 4
// and 9), so a swap that moves 10 g of fat counts more than one that moves 10 g
// of carbs.
func ReplacementDistance(diff Macros) float64 {
	return math.Abs(diff.Kcal) + 4*math.Abs(diff.Protein) + 4*math.Abs(diff.Carbs) + 9*math.Abs(diff.Fat)
}

// DiffOf is the difference of an ingredient at grams against the current row.
func DiffOf(cur Macros, ing domain.Ingredient, grams float64) (Macros, Macros) {
	m := FromIngredient(ing, grams)
	return m, Macros{Kcal: m.Kcal - cur.Kcal, Protein: m.Protein - cur.Protein, Carbs: m.Carbs - cur.Carbs, Fat: m.Fat - cur.Fat}
}

// RankReplacements puts the candidates in the order to suggest them in place of
// a row that weighs grams and has the macros cur: the ones in the same category
// first (sameCategory is a set of ingredient ids), then the nearest by
// ReplacementDistance, then by name. The ingredient with id curID is left out,
// and so is anything past limit.
func RankReplacements(cur Macros, grams float64, curID int64, cands []*domain.Ingredient, sameCategory map[int64]bool, limit int) []Replacement {
	out := make([]Replacement, 0, len(cands))
	for _, c := range cands {
		if c == nil || c.ID == curID {
			continue
		}
		m, d := DiffOf(cur, *c, grams)
		out = append(out, Replacement{Ingredient: c, SameCategory: sameCategory[c.ID], Macros: m, Diff: d, Distance: ReplacementDistance(d)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.SameCategory != b.SameCategory {
			return a.SameCategory
		}
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		return a.Ingredient.Name < b.Ingredient.Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
