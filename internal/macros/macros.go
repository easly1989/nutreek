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

// ForMealItem calcola i macro di una singola voce di pasto. La quantita'
// in MealItem e' interpretata diversamente a seconda del tipo:
//   - se e' un ingrediente: g/ml -> FromIngredient.
//   - se e' una ricetta senza variazioni: porzioni × PerServing(totale).
//   - se e' una ricetta con variazioni (Customizations non vuote): somma diretta
//     delle quantita' assolute nelle variazioni (gia' rappresentano il consumo
//     reale, senza ulteriore scaling per porzioni).
func ForMealItem(item domain.MealItemFull) Macros {
	if item.Ingredient != nil {
		grams := item.Quantity
		if item.QuantityUnit == "pz" && item.Ingredient.UnitWeightG != nil {
			grams = item.Quantity * *item.Ingredient.UnitWeightG
		}
		return FromIngredient(*item.Ingredient, grams)
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
		ingredients := make([]IngredientItem, 0, len(item.RecipeItems))
		for _, ri := range item.RecipeItems {
			ingredients = append(ingredients, IngredientItem{
				Ingredient: ri.Ingredient,
				Quantity:   ri.Quantity,
			})
		}
		total := ForRecipe(ingredients)
		per := PerServing(total, item.Recipe.Servings)
		return per.Scale(item.Quantity) // quantity = porzioni
	}
	return Macros{}
}

// ForMeal somma i macro di tutti gli items di un pasto.
func ForMeal(items []domain.MealItemFull) Macros {
	var out Macros
	for _, it := range items {
		out = out.Add(ForMealItem(it))
	}
	return out
}

// ForMealEffective ritorna i macro "effettivi" del pasto: se ci sono sgarri
// registrati, sostituiscono completamente gli items ufficiali (l'utente ha
// mangiato lo sgarro al loro posto). Senza sgarri, equivale a ForMeal(Items).
func ForMealEffective(meal domain.MealWithItems) Macros {
	if meal.HasCheats() {
		return ForMeal(meal.Cheats)
	}
	return ForMeal(meal.Items)
}

// ForDay somma i macro effettivi di tutti i pasti di un giorno (sgarri
// inclusi al posto degli items quando presenti).
func ForDay(meals []domain.MealWithItems) Macros {
	var out Macros
	for _, m := range meals {
		out = out.Add(ForMealEffective(m))
	}
	return out
}

// ForWeek somma i macro effettivi di tutti i giorni della settimana.
func ForWeek(days [7][]domain.MealWithItems) Macros {
	var out Macros
	for _, day := range days {
		out = out.Add(ForDay(day))
	}
	return out
}
