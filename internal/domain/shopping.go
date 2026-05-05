package domain

import "time"

// ShoppingList rappresenta una lista della spesa per una settimana
// specifica (o piu' settimane consecutive). Una sola lista per
// (utente, weekStart). week_count >= 1 indica per quante settimane la
// lista aggrega gli ingredienti.
type ShoppingList struct {
	ID        int64
	UserID    int64
	WeekStart time.Time
	WeekCount int
	CreatedAt time.Time
}

// ShoppingListItem e' una riga generata dall'aggregazione automatica del
// piano: un ingrediente specifico con la quantita' totale richiesta.
// Ingredient viene popolato lato repo nel caricamento full.
type ShoppingListItem struct {
	ID             int64
	ShoppingListID int64
	IngredientID   int64
	TotalQuantity  float64
	Checked        bool
	SortOrder      int
	Ingredient     *Ingredient
}

// ShoppingUserItem e' una riga manuale aggiunta dall'utente. Testo
// libero, persistente fra rigenerazioni.
type ShoppingUserItem struct {
	ID             int64
	ShoppingListID int64
	Description    string
	QuantityText   string
	Checked        bool
	SortOrder      int
	CreatedAt      time.Time
}

// ShoppingListFull aggrega lista + items auto + items utente per il
// rendering della pagina spesa.
type ShoppingListFull struct {
	ShoppingList
	Items     []ShoppingListItem
	UserItems []ShoppingUserItem
}
