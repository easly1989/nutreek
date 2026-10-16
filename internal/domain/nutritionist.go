package domain

import "time"

type NutritionistPlan struct {
	ID        int64
	UserID    int64
	Name      string
	Notes     *string
	IsActive  bool // a single active plan per user (enforced by a partial index)
	CreatedAt time.Time
	UpdatedAt time.Time
}

type NutritionistMealType struct {
	ID        int64
	PlanID    int64
	Name      string
	SortOrder int
}

type NutritionistVariation struct {
	ID         int64
	MealTypeID int64
	Name       *string
	SortOrder  int
}

type NutritionistItem struct {
	ID           int64
	VariationID  int64
	RecipeID     *int64
	IngredientID *int64
	Quantity     float64
	QuantityUnit string
	SortOrder    int
	Recipe       *Recipe
	Ingredient   *Ingredient
}

type NutritionistVariationFull struct {
	NutritionistVariation
	Items []NutritionistItem
}

type NutritionistMealTypeFull struct {
	NutritionistMealType
	Variations []NutritionistVariationFull
}

type NutritionistPlanFull struct {
	NutritionistPlan
	MealTypes []NutritionistMealTypeFull
}
