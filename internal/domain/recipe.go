package domain

import "time"

// Recipe e' una ricetta dell'utente, eventualmente pubblica (visibile ad altri).
type Recipe struct {
	ID          int64
	UserID      int64
	Name        string
	Description *string
	Servings    int
	IsPublic    bool
	// IsDraft = true per bozze create da Duplicate non ancora modificate.
	// I draft sono nascosti dalle liste e dai piani, e il salva metadati è
	// bloccato finché restano draft. Vengono promossi al primo cambio di
	// contenuto (add/remove ingrediente) che li rende diversi dalle altre.
	IsDraft   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RecipeItem e' un ingrediente che compone una ricetta.
type RecipeItem struct {
	ID           int64
	RecipeID     int64
	IngredientID int64
	Quantity     float64 // in g o ml a seconda dell'unita' dell'ingrediente
	SortOrder    int
}

// RecipeItemFull joina RecipeItem + Ingredient per i calcoli macro / render.
type RecipeItemFull struct {
	RecipeItem
	Ingredient Ingredient
}

// RecipeWithItems aggrega ricetta + items + autore (display name) per il
// render della pagina di dettaglio.
type RecipeWithItems struct {
	Recipe
	AuthorName string
	Items      []RecipeItemFull
}
