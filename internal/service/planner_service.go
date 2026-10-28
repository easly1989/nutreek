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

// Domain errors of the automatic generator: the caller (handler) tells them
// apart to show the user the right message.
var (
	// ErrNoTarget: no calorie target set, generation is impossible.
	ErrNoTarget = errors.New("target calorico non impostato")
	// ErrNoNutritionistPool: no usable variation in the nutrition plan.
	ErrNoNutritionistPool = errors.New("nessuna variante nel piano nutrizionista")
	// ErrWeekNotEmpty: the week already has meals.
	ErrWeekNotEmpty = errors.New("settimana gia' popolata")
)

// PlannerService generates the weekly plan automatically, drawing from the
// pool of variations of the user's effective nutrition plan (the one assigned
// by the coach, when present, otherwise their own active plan).
type PlannerService struct {
	WeeklyPlans       *db.WeeklyPlans
	NutritionistPlans *db.NutritionistPlans
	Recipes           *db.Recipes
	Coach             *db.CoachRepository
}

// EffectivePlanFull resolves the user's effective nutrition plan: the plan
// assigned by the coach when present, otherwise the user's active plan
// (created when missing).
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

// GenerateWeek fills the week weekStart with meals generated from the
// nutrition plan, approaching the daily macro targets. It refuses when the
// week already has meals or the pool/targets are missing. Every meal type of
// the nutrition plan becomes a slot repeated on all 7 days.
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

	// Per-serving macros of the referenced recipes: LoadPlanFull does not load
	// the recipes' ingredients, so we resolve them here once.
	recipePerServing, err := s.recipePerServing(ctx, userID, full)
	if err != nil {
		return err
	}

	// Build the slots (one meal type = one slot) and index the items by
	// variation so they can be copied to the DB after the selection.
	var slots []planner.Slot
	itemsByVariation := make(map[int64][]domain.NutritionistItem)
	for _, mt := range full.MealTypes {
		var vars []planner.Variation
		for _, v := range mt.Variations {
			if len(v.Items) == 0 {
				continue // empty variations produce no meals
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

	// The same slots apply to all 7 days of the week.
	daySlots := make([][]planner.Slot, 7)
	for d := range daySlots {
		daySlots[d] = slots
	}
	days := planner.Generate(daySlots, target, weekStart.Unix())

	// Turn the selections into meals + items to insert.
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
	// Historical freeze: record which nutrition plan generated the week.
	if err := s.WeeklyPlans.SetSourcePlan(ctx, plan.ID, full.ID); err != nil {
		return err
	}
	return nil
}

// recipePerServing resolves, for every recipe referenced in the plan, the
// macros of a single serving. A recipe that cannot be accessed counts 0 (it
// does not block the generation).
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
				out[*it.RecipeID] = macros.PerServing(macros.ForRecipeItems(rec.Items), rec.Servings)
			}
		}
	}
	return out, nil
}
