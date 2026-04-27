// Package macros calcola i macronutrienti (kcal/proteine/carboidrati/grassi)
// a partire da ingredienti, ricette, pasti e giornate. Modulo puro: zero
// dipendenze fuori dalla stdlib + internal/domain, completamente testabile.
package macros

import "github.com/amon2126/nutreek/internal/domain"

// Macros aggrega i 4 valori standard. Tutti per la stessa porzione (es.
// "totale del pasto", non "per 100g").
type Macros struct {
	Kcal    float64
	Protein float64
	Carbs   float64
	Fat     float64
}

// Add ritorna m + o senza mutare i ricevitori.
func (m Macros) Add(o Macros) Macros {
	return Macros{
		Kcal:    m.Kcal + o.Kcal,
		Protein: m.Protein + o.Protein,
		Carbs:   m.Carbs + o.Carbs,
		Fat:     m.Fat + o.Fat,
	}
}

// Scale moltiplica tutti i campi per f. Usato per servings o per quantita'.
func (m Macros) Scale(f float64) Macros {
	return Macros{
		Kcal:    m.Kcal * f,
		Protein: m.Protein * f,
		Carbs:   m.Carbs * f,
		Fat:     m.Fat * f,
	}
}

// IsZero indica se tutti e 4 i campi sono 0. Utile per skip rendering.
func (m Macros) IsZero() bool {
	return m.Kcal == 0 && m.Protein == 0 && m.Carbs == 0 && m.Fat == 0
}

// FromIngredient calcola i macro per `quantity` (in g o ml a seconda dell'unita'
// dell'ingrediente). I valori `_per_100` vengono divisi per 100 e moltiplicati
// per la quantita'. Quantity negative o zero ritornano Macros{}.
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

// IngredientItem e' una vista minimale per ForRecipe: l'ingrediente
// + la quantita' usata. Le ricette aggregano N di questi.
type IngredientItem struct {
	Ingredient domain.Ingredient
	Quantity   float64 // in g o ml
}

// ForRecipe somma i macro di tutti gli items. Restituisce il totale della
// ricetta (non per-serving, vedi PerServing).
func ForRecipe(items []IngredientItem) Macros {
	var total Macros
	for _, it := range items {
		total = total.Add(FromIngredient(it.Ingredient, it.Quantity))
	}
	return total
}

// PerServing divide i macro totali per il numero di porzioni. servings <= 0
// ritorna Macros{} per evitare division-by-zero.
func PerServing(total Macros, servings int) Macros {
	if servings <= 0 {
		return Macros{}
	}
	return total.Scale(1.0 / float64(servings))
}
