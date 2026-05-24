package domain

import "time"

// WeeklyPlan rappresenta il piano settimanale di un utente per la settimana
// che inizia il lunedi' WeekStart. TemplateID puo' essere nil (piano vuoto
// non derivato) o riferirsi al template usato.
type WeeklyPlan struct {
	ID         int64
	UserID     int64
	WeekStart  time.Time // sempre un lunedi'
	TemplateID *int64
}

// Meal e' un pasto (es. "Colazione") in un giorno specifico del piano.
type Meal struct {
	ID           int64
	WeeklyPlanID int64
	DayOfWeek    int // 0 = lunedi', 6 = domenica
	Name         string
	SortOrder    int
}

// MealItem e' una riga di pasto: o una ricetta (Quantity = porzioni) o un
// ingrediente puro (Quantity = g/ml). Lo schema impone XOR via CHECK.
type MealItem struct {
	ID           int64
	MealID       int64
	RecipeID     *int64
	IngredientID *int64
	Quantity     float64
	SortOrder    int
}

// IsRecipe ritorna true se l'item punta a una ricetta.
func (m MealItem) IsRecipe() bool { return m.RecipeID != nil }

// IsIngredient ritorna true se l'item punta a un ingrediente puro.
func (m MealItem) IsIngredient() bool { return m.IngredientID != nil }

// MealItemCustomization e' una riga di personalizzazione per un singolo
// meal_item ricetta. Sostituisce gli ingredienti base della ricetta nel
// calcolo dei macro per quell'occorrenza specifica.
type MealItemCustomization struct {
	ID           int64
	MealItemID   int64
	IngredientID int64
	Quantity     float64
	SortOrder    int
	Ingredient   *Ingredient // popolato al caricamento per i calcoli macro
}

// MealItemFull e' MealItem con il riferimento risolto. Solo uno fra
// Recipe e Ingredient e' valorizzato.
type MealItemFull struct {
	MealItem
	Recipe         *Recipe                  // popolato se IsRecipe
	RecipeItems    []RecipeItemFull         // popolato se IsRecipe (per macro)
	Customizations []MealItemCustomization  // popolato se l'item ha variazioni
	Ingredient     *Ingredient              // popolato se IsIngredient
}

// IsCustomized ritorna true se questo item ricetta ha variazioni personalizzate
// che sostituiscono gli ingredienti base per il calcolo dei macro.
func (m MealItemFull) IsCustomized() bool { return len(m.Customizations) > 0 }

// MealWithItems e' Meal + items risolti.
// Cheats e' la lista degli sgarri associati al pasto: quando ne contiene
// almeno uno, i macro effettivi del pasto sono dati da Cheats invece che
// da Items (gli Items "ufficiali" restano presenti per riferimento UI).
type MealWithItems struct {
	Meal
	Items  []MealItemFull
	Cheats []MealItemFull
}

// HasCheats e' true quando il pasto ha sgarri registrati.
func (m MealWithItems) HasCheats() bool { return len(m.Cheats) > 0 }

// WeeklyPlanFull e' la struttura completa renderizzata in dashboard:
// piano + tutti i meals raggruppati per giorno (slice 7 lunghezza).
type WeeklyPlanFull struct {
	WeeklyPlan
	// Days[0..6] = lista pasti per quel giorno della settimana.
	Days [7][]MealWithItems
}
