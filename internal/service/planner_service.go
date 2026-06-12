package service

import (
	"context"
	"errors"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/planner"
)

// Errori di dominio del generatore automatico: il chiamante (handler) li
// distingue per mostrare il messaggio giusto all'utente.
var (
	// ErrNoTarget: target calorico non impostato, impossibile generare.
	ErrNoTarget = errors.New("target calorico non impostato")
	// ErrNoNutritionistPool: nessuna variante utilizzabile nel piano nutrizionista.
	ErrNoNutritionistPool = errors.New("nessuna variante nel piano nutrizionista")
	// ErrWeekNotEmpty: la settimana ha gia' dei pasti.
	ErrWeekNotEmpty = errors.New("settimana gia' popolata")
)

// PlannerService genera automaticamente il piano settimanale pescando dal
// pool di varianti del piano nutrizionista effettivo dell'utente (quello
// assegnato dal coach, se presente, altrimenti il proprio piano attivo).
type PlannerService struct {
	WeeklyPlans       *db.WeeklyPlans
	NutritionistPlans *db.NutritionistPlans
	Recipes           *db.Recipes
	Coach             *db.CoachRepository
}

// EffectivePlanFull risolve il piano nutrizionista effettivo dell'utente:
// il piano assegnato dal coach se presente, altrimenti il piano attivo
// dell'utente (creato se mancante).
func (s *PlannerService) EffectivePlanFull(ctx context.Context, userID int64) (*domain.NutritionistPlanFull, error) {
	if s.Coach != nil {
		assignment, err := s.Coach.GetAssignment(ctx, userID)
		if err != nil {
			return nil, err
		}
		if assignment != nil && assignment.AssignedPlanID != nil {
			return s.NutritionistPlans.LoadPlanFullByID(ctx, *assignment.AssignedPlanID)
		}
	}
	return s.NutritionistPlans.LoadPlanFull(ctx, userID)
}

// GenerateWeek riempie la settimana weekStart con pasti generati dal piano
// nutrizionista, avvicinandosi ai target macro giornalieri. Rifiuta se la
// settimana ha gia' pasti o se mancano pool/target. Ogni tipo pasto del piano
// nutrizionista diventa uno slot replicato su tutti e 7 i giorni.
func (s *PlannerService) GenerateWeek(ctx context.Context, userID int64, weekStart time.Time, target planner.DayTarget) error {
	if target.Kcal <= 0 {
		return ErrNoTarget
	}
	plan, err := s.WeeklyPlans.GetOrCreateForWeek(ctx, userID, weekStart)
	if err != nil {
		return err
	}
	hasMeals, err := s.WeeklyPlans.HasMeals(ctx, plan.ID)
	if err != nil {
		return err
	}
	if hasMeals {
		return ErrWeekNotEmpty
	}

	full, err := s.EffectivePlanFull(ctx, userID)
	if err != nil {
		return err
	}

	// Macro per-porzione delle ricette referenziate: LoadPlanFull non carica
	// gli ingredienti delle ricette, quindi li risolviamo qui una volta sola.
	recipePerServing, err := s.recipePerServing(ctx, userID, full)
	if err != nil {
		return err
	}

	// Costruisce gli slot (un tipo pasto = uno slot) e indicizza gli item per
	// variante per poterli replicare a DB dopo la selezione.
	var slots []planner.Slot
	itemsByVariation := make(map[int64][]domain.NutritionistItem)
	for _, mt := range full.MealTypes {
		var vars []planner.Variation
		for _, v := range mt.Variations {
			if len(v.Items) == 0 {
				continue // varianti vuote non producono pasti
			}
			itemsByVariation[v.ID] = v.Items
			vars = append(vars, planner.Variation{
				ID:     v.ID,
				Macros: macros.ForNutritionistVariation(v.Items, recipePerServing),
			})
		}
		if len(vars) == 0 {
			continue
		}
		slots = append(slots, planner.Slot{Name: mt.Name, Variations: vars})
	}
	if len(slots) == 0 {
		return ErrNoNutritionistPool
	}

	// Gli stessi slot valgono per tutti e 7 i giorni della settimana.
	daySlots := make([][]planner.Slot, 7)
	for d := range daySlots {
		daySlots[d] = slots
	}
	days := planner.Generate(daySlots, target, weekStart.Unix())

	// Traduce le selezioni in pasti + voci da inserire.
	var meals []db.GeneratedMeal
	for _, day := range days {
		for sortOrder, sel := range day.Selections {
			gm := db.GeneratedMeal{
				DayOfWeek: day.DayOfWeek,
				Name:      sel.SlotName,
				SortOrder: sortOrder,
			}
			for _, it := range itemsByVariation[sel.VariationID] {
				gm.Items = append(gm.Items, db.GeneratedMealItem{
					RecipeID:     it.RecipeID,
					IngredientID: it.IngredientID,
					Quantity:     it.Quantity,
					QuantityUnit: it.QuantityUnit,
				})
			}
			meals = append(meals, gm)
		}
	}

	if err := s.WeeklyPlans.CreateMeals(ctx, plan.ID, userID, meals); err != nil {
		return err
	}
	// Freeze storico: registra quale piano nutrizionista ha generato la settimana.
	if err := s.WeeklyPlans.SetSourcePlan(ctx, plan.ID, full.ID); err != nil {
		return err
	}
	return nil
}

// recipePerServing risolve, per ogni ricetta referenziata nel piano, i macro
// di una singola porzione. Una ricetta non accessibile contribuisce 0 (non
// blocca la generazione).
func (s *PlannerService) recipePerServing(ctx context.Context, userID int64, full *domain.NutritionistPlanFull) (map[int64]macros.Macros, error) {
	out := make(map[int64]macros.Macros)
	for _, mt := range full.MealTypes {
		for _, v := range mt.Variations {
			for _, it := range v.Items {
				if it.RecipeID == nil {
					continue
				}
				if _, done := out[*it.RecipeID]; done {
					continue
				}
				rec, err := s.Recipes.GetWithItems(ctx, *it.RecipeID, userID)
				if err != nil {
					out[*it.RecipeID] = macros.Macros{}
					continue
				}
				ingredients := make([]macros.IngredientItem, 0, len(rec.Items))
				for _, ri := range rec.Items {
					ingredients = append(ingredients, macros.IngredientItem{
						Ingredient: ri.Ingredient,
						Quantity:   ri.Quantity,
					})
				}
				out[*it.RecipeID] = macros.PerServing(macros.ForRecipe(ingredients), rec.Servings)
			}
		}
	}
	return out, nil
}
