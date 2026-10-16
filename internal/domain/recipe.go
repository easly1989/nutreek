package domain

import "time"

// Recipe is a user's recipe, possibly public (visible to others).
type Recipe struct {
	ID          int64
	UserID      int64
	Name        string
	Description *string
	Servings    int
	IsPublic    bool
	// IsDraft = true for drafts created by Duplicate and not changed yet.
	// Drafts are hidden from lists and plans, and saving the metadata is
	// blocked while they stay drafts. They are promoted on the first content
	// change (add/remove ingredient) that makes them differ from the others.
	IsDraft   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RecipeItem is an ingredient that makes up a recipe.
type RecipeItem struct {
	ID           int64
	RecipeID     int64
	IngredientID int64
	Quantity     float64 // in g or ml depending on the ingredient's unit
	SortOrder    int
}

// RecipeItemFull joins RecipeItem + Ingredient for macro calculations / rendering.
type RecipeItemFull struct {
	RecipeItem
	Ingredient Ingredient
}

// RecipeWithItems joins recipe + items + author (display name) for
// rendering the detail page.
type RecipeWithItems struct {
	Recipe
	AuthorName string
	Items      []RecipeItemFull
}
