// Package macros computes the macronutrients (kcal/protein/carbs/fat) from
// ingredients, recipes, meals and days. A pure module: no dependencies
// outside the stdlib + internal/domain, fully testable.
package macros

import "github.com/amon2126/nutreek/internal/domain"

// Macros holds the 4 standard values. All for the same portion (e.g. "the
// meal's total", not "per 100g").
type Macros struct {
	Kcal    float64
	Protein float64
	Carbs   float64
	Fat     float64
}

// Add returns m + o without mutating the receivers.
func (m Macros) Add(o Macros) Macros {
	return Macros{
		Kcal:    m.Kcal + o.Kcal,
		Protein: m.Protein + o.Protein,
		Carbs:   m.Carbs + o.Carbs,
		Fat:     m.Fat + o.Fat,
	}
}

// Scale multiplies every field by f. Used for servings or quantities.
func (m Macros) Scale(f float64) Macros {
	return Macros{
		Kcal:    m.Kcal * f,
		Protein: m.Protein * f,
		Carbs:   m.Carbs * f,
		Fat:     m.Fat * f,
	}
}

// IsZero says whether all 4 fields are 0. Useful to skip rendering.
func (m Macros) IsZero() bool {
	return m.Kcal == 0 && m.Protein == 0 && m.Carbs == 0 && m.Fat == 0
}

// Shares is the percentage split of the energy among the three macros.
type Shares struct {
	Protein, Carbs, Fat float64
}

// EnergyShares splits the macros' energy (4 kcal/g for protein and carbs,
// 9 kcal/g for fat) into percentages that add up to 100. Without macros it
// returns Shares{}.
func (m Macros) EnergyShares() Shares {
	p, c, f := m.Protein*4, m.Carbs*4, m.Fat*9
	tot := p + c + f
	if tot <= 0 {
		return Shares{}
	}
	return Shares{Protein: p / tot * 100, Carbs: c / tot * 100, Fat: f / tot * 100}
}

// FromIngredient computes the macros for `quantity` (in g or ml depending on
// the ingredient's unit). The `_per_100` values are divided by 100 and
// multiplied by the quantity. Negative or zero quantities return Macros{}.
func FromIngredient(ing domain.Ingredient, quantity float64) Macros {
	if quantity <= 0 {
		return Macros{}
	}
	factor := quantity / 100.0
	return Macros{
		Kcal:    ing.KcalPer100 * factor,
		Protein: ing.ProteinPer100 * factor,
		Carbs:   ing.CarbsPer100 * factor,
		Fat:     ing.FatPer100 * factor,
	}
}

// IngredientItem is a minimal view for ForRecipe: the ingredient + the
// quantity used. Recipes aggregate N of these.
type IngredientItem struct {
	Ingredient domain.Ingredient
	Quantity   float64 // in g o ml
}

// ForRecipe sums the macros of every item. It returns the recipe's total
// (not per serving, see PerServing).
func ForRecipe(items []IngredientItem) Macros {
	var total Macros
	for _, it := range items {
		total = total.Add(FromIngredient(it.Ingredient, it.Quantity))
	}
	return total
}

// PerServing divides the total macros by the number of servings. servings <= 0
// returns Macros{} to avoid a division by zero.
func PerServing(total Macros, servings int) Macros {
	if servings <= 0 {
		return Macros{}
	}
	return total.Scale(1.0 / float64(servings))
}

// ForMealItem computes the macros of a single meal item. The quantity in
// MealItem is read differently depending on the type:
//   - an ingredient: g/ml -> FromIngredient.
//   - a recipe without variations: servings × PerServing(total).
//   - a recipe with variations (non-empty Customizations): a direct sum of
//     the absolute quantities in the variations (they already represent what
//     was actually eaten, with no further scaling by servings).
func ForMealItem(item domain.MealItemFull) Macros {
	if item.Ingredient != nil {
		return FromIngredient(*item.Ingredient, Grams(item.Quantity, item.QuantityUnit, item.Ingredient))
	}
	if item.Recipe != nil {
		if len(item.Customizations) > 0 {
			var out Macros
			for _, c := range item.Customizations {
				if c.Ingredient != nil {
					out = out.Add(FromIngredient(*c.Ingredient, c.Quantity))
				}
			}
			return out
		}
		per := PerServing(ForRecipeItems(item.RecipeItems), item.Recipe.Servings)
		return per.Scale(item.Quantity) // quantity = porzioni
	}
	return Macros{}
}

// Grams converts an item's quantity into grams/ml: items in pieces («pz»)
// count quantity × the ingredient's unit weight. Without a known unit weight
// the quantity stays as entered.
func Grams(quantity float64, unit string, ing *domain.Ingredient) float64 {
	if unit == "pz" && ing != nil && ing.PieceWeight() > 0 {
		return quantity * ing.PieceWeight()
	}
	return quantity
}

// RecipeItemGrams is a recipe row's quantity in grams/ml (see Grams).
func RecipeItemGrams(ri domain.RecipeItemFull) float64 {
	return Grams(ri.Quantity, ri.QuantityUnit, &ri.Ingredient)
}

// ForRecipeItems sums the macros of a recipe's rows, rows in pieces counted
// through the unit weight. It returns the recipe's total (see PerServing).
func ForRecipeItems(items []domain.RecipeItemFull) Macros {
	var total Macros
	for _, ri := range items {
		total = total.Add(FromIngredient(ri.Ingredient, RecipeItemGrams(ri)))
	}
	return total
}

// ForNutritionistVariation computes the total macros of the items of a
// nutrition plan variation. Since LoadPlanFull does not load the ingredients
// of the referenced recipes, the recipes' per-serving macros must be
// precomputed by the caller and passed in recipePerServing (key = recipe
// ID). Ingredient items are computed directly, with «pz» items converted
// to grams through the unit weight (see Grams).
func ForNutritionistVariation(items []domain.NutritionistItem, recipePerServing map[int64]Macros) Macros {
	var out Macros
	for _, it := range items {
		switch {
		case it.Ingredient != nil:
			out = out.Add(FromIngredient(*it.Ingredient, Grams(it.Quantity, it.QuantityUnit, it.Ingredient)))
		case it.RecipeID != nil:
			if per, ok := recipePerServing[*it.RecipeID]; ok {
				out = out.Add(per.Scale(it.Quantity)) // quantity = porzioni
			}
		}
	}
	return out
}

// ForMeal sums the macros of every item of a meal.
func ForMeal(items []domain.MealItemFull) Macros {
	var out Macros
	for _, it := range items {
		out = out.Add(ForMealItem(it))
	}
	return out
}

// ForMealEffective returns the meal's "actual" macros: when cheats are
// logged, they fully replace the official items (the user ate the cheat
// instead). Without cheats, it equals ForMeal(Items).
func ForMealEffective(meal domain.MealWithItems) Macros {
	if meal.HasCheats() {
		return ForMeal(meal.Cheats)
	}
	return ForMeal(meal.Items)
}

// ForDay sums the actual macros of every meal of a day (cheats included in
// place of the items when present).
func ForDay(meals []domain.MealWithItems) Macros {
	var out Macros
	for _, m := range meals {
		out = out.Add(ForMealEffective(m))
	}
	return out
}

// ForWeek sums the actual macros of every day of the week.
func ForWeek(days [7][]domain.MealWithItems) Macros {
	var out Macros
	for _, day := range days {
		out = out.Add(ForDay(day))
	}
	return out
}
