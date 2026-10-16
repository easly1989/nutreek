package domain

import "time"

// WeeklyPlan is a user's weekly plan for the week starting on Monday
// WeekStart. TemplateID can be nil (an empty plan not derived from a
// template) or point to the template used.
type WeeklyPlan struct {
	ID         int64
	UserID     int64
	WeekStart  time.Time // always a Monday
	TemplateID *int64
}

// Meal is a meal (e.g. "Colazione") on a specific day of the plan.
type Meal struct {
	ID           int64
	WeeklyPlanID int64
	DayOfWeek    int // 0 = lunedi', 6 = domenica
	Name         string
	SortOrder    int
}

// MealItem is a meal row: either a recipe (Quantity = servings) or a plain
// ingredient (Quantity = g/ml/pz). The schema enforces the XOR via CHECK.
type MealItem struct {
	ID           int64
	MealID       int64
	RecipeID     *int64
	IngredientID *int64
	Quantity     float64
	SortOrder    int
	// QuantityUnit is the unit the user picked: 'g', 'ml', or 'pz'.
	// 'pz' needs ingredient.UnitWeightG for the macro calculation.
	QuantityUnit string
}

// IsRecipe returns true when the item points to a recipe.
func (m MealItem) IsRecipe() bool { return m.RecipeID != nil }

// IsIngredient returns true when the item points to a plain ingredient.
func (m MealItem) IsIngredient() bool { return m.IngredientID != nil }

// MealItemCustomization is a customization row for a single recipe
// meal_item. It replaces the recipe's base ingredients in the macro
// calculation for that specific occurrence.
type MealItemCustomization struct {
	ID           int64
	MealItemID   int64
	IngredientID int64
	Quantity     float64
	SortOrder    int
	Ingredient   *Ingredient // filled on load for the macro calculations
}

// MealItemFull is MealItem with the reference resolved. Only one of Recipe
// and Ingredient is set.
type MealItemFull struct {
	MealItem
	Recipe         *Recipe                 // filled when IsRecipe
	RecipeItems    []RecipeItemFull        // filled when IsRecipe (for macros)
	Customizations []MealItemCustomization // filled when the item has variations
	Ingredient     *Ingredient             // filled when IsIngredient
}

// IsCustomized returns true when this recipe item has custom variations that
// replace the base ingredients in the macro calculation.
func (m MealItemFull) IsCustomized() bool { return len(m.Customizations) > 0 }

// MealWithItems is Meal + resolved items.
// Cheats is the list of cheats attached to the meal: when it holds at least
// one, the meal's actual macros come from Cheats instead of Items (the
// "official" Items stay there for the UI's reference).
type MealWithItems struct {
	Meal
	Items  []MealItemFull
	Cheats []MealItemFull
}

// HasCheats is true when the meal has cheats logged.
func (m MealWithItems) HasCheats() bool { return len(m.Cheats) > 0 }

// WeeklyPlanFull is the complete structure rendered in the dashboard:
// plan + every meal grouped by day (a slice of length 7).
type WeeklyPlanFull struct {
	WeeklyPlan
	// Days[0..6] = the list of meals for that day of the week.
	Days [7][]MealWithItems
}
