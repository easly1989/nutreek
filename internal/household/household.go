// Package household scales the account owner's plan to the other people of the
// household (#238). The owner's quantities are the base; a person's are those
// quantities times the ratio of the two calorie goals, rounded to a step that makes
// sense for the unit, unless the person has a quantity of their own for an item or
// skips the whole meal. Everything here is pure.
package household

import (
	"math"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// The rounding steps: grams and millilitres to 5, pieces to half a piece, recipe
// servings to a quarter.
const (
	StepGrams    = 5.0
	StepPieces   = 0.5
	StepServings = 0.25
)

// Ratio is a person's goal over the base goal; 1 when either is unknown, so a plan
// without goals shows the same quantities for everyone rather than nothing.
func Ratio(person, base float64) float64 {
	if person <= 0 || base <= 0 {
		return 1
	}
	return person / base
}

// Step is the rounding step of an item's quantity.
func Step(it domain.MealItemFull) float64 {
	switch {
	case it.Recipe != nil:
		return StepServings
	case it.QuantityUnit == "pz":
		return StepPieces
	}
	return StepGrams
}

// RoundTo rounds q to the nearest multiple of step, never to zero for a positive q
// (a person who eats a little still eats something).
func RoundTo(q, step float64) float64 {
	if q <= 0 || step <= 0 {
		return 0
	}
	return math.Max(math.Round(q/step), 1) * step
}

// Person is one person of the household as the calculations see them.
type Person struct {
	ID    int64
	Ratio float64
	// Quantities are the quantities set by hand, by meal item id, in the item's unit
	// (0 = not eaten). Skipped holds the ids of the meals the person does not eat.
	Quantities map[int64]float64
	Skipped    map[int64]bool
}

// Quantity is what the person eats of a meal item, in the item's own unit.
func (p Person) Quantity(it domain.MealItemFull) float64 {
	if p.Skipped[it.MealID] {
		return 0
	}
	if q, ok := p.Quantities[it.ID]; ok {
		return q
	}
	return p.Auto(it)
}

// Manual says whether the quantity of the item was set by hand for this person.
func (p Person) Manual(it domain.MealItemFull) bool {
	_, ok := p.Quantities[it.ID]
	return ok
}

// Auto is the quantity the person would eat without a hand-set one, for the
// «auto 75 g» hint next to a changed quantity.
func (p Person) Auto(it domain.MealItemFull) float64 {
	if p.Ratio == 1 { // the owner, or the same goal: the very same quantity, not rounded
		return it.Quantity
	}
	return RoundTo(it.Quantity*p.Ratio, Step(it))
}

// Factor is the person's quantity over the owner's: every macro is linear in the
// quantity, so it scales the owner's macros to the person's.
func (p Person) Factor(it domain.MealItemFull) float64 {
	if it.Quantity <= 0 {
		return 0
	}
	return p.Quantity(it) / it.Quantity
}

// ItemMacros are the macros of what the person eats of an item.
func (p Person) ItemMacros(it domain.MealItemFull) macros.Macros {
	return macros.ForMealItem(it).Scale(p.Factor(it))
}

// MealMacros are the macros of what the person eats of a meal: nothing for a
// reminder or a skipped meal; a meal with cheats counts what was really eaten,
// scaled by the ratio (a cheat has no per-person quantity).
func (p Person) MealMacros(m domain.MealWithItems) macros.Macros {
	if m.IsReminder || p.Skipped[m.ID] {
		return macros.Macros{}
	}
	if m.HasCheats() {
		return macros.ForMeal(m.Cheats).Scale(p.Ratio)
	}
	var out macros.Macros
	for _, it := range m.Items {
		out = out.Add(p.ItemMacros(it))
	}
	return out
}

// DayMacros are the macros of what the person eats in a day.
func (p Person) DayMacros(meals []domain.MealWithItems) macros.Macros {
	var out macros.Macros
	for _, m := range meals {
		out = out.Add(p.MealMacros(m))
	}
	return out
}

// Needs adds to into what the person needs of each ingredient (in grams or
// millilitres) for the meals, the way a shopping list counts them: a meal with cheats
// has been eaten and needs nothing, a recipe expands into its ingredients.
func (p Person) Needs(meals []domain.MealWithItems, into map[int64]float64) {
	for _, m := range meals {
		if m.HasCheats() || m.IsReminder {
			continue
		}
		for _, it := range m.Items {
			q := p.Quantity(it)
			if q <= 0 {
				continue
			}
			if it.Ingredient != nil {
				into[it.Ingredient.ID] += macros.Grams(q, it.QuantityUnit, it.Ingredient)
			}
			if it.Recipe != nil && it.Recipe.Servings > 0 {
				scale := q / float64(it.Recipe.Servings)
				for _, ri := range it.RecipeItems {
					into[ri.IngredientID] += macros.RecipeItemGrams(ri) * scale
				}
			}
		}
	}
}
