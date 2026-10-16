package app

import (
	"math"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

func TestPlanKcal(t *testing.T) {
	peso := 150.0
	mela := &domain.Ingredient{KcalPer100: 52, UnitWeightG: &peso}
	ricetta := int64(9)
	plan := &domain.NutritionistPlanFull{MealTypes: []domain.NutritionistMealTypeFull{{
		Variations: []domain.NutritionistVariationFull{{
			NutritionistVariation: domain.NutritionistVariation{ID: 1},
			Items: []domain.NutritionistItem{
				{ID: 10, Ingredient: mela, Quantity: 1, QuantityUnit: "pz"},  // 78
				{ID: 11, RecipeID: &ricetta, Quantity: 2, QuantityUnit: "g"}, // 2 × 420
				{ID: 12, Ingredient: mela, Quantity: 100, QuantityUnit: "g"}, // 52
			},
		}},
	}}}
	vars, items := planKcal(plan, map[int64]macros.Macros{9: {Kcal: 420}})
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.01 }
	if !near(items[10], 78) || !near(items[11], 840) || !near(items[12], 52) {
		t.Errorf("kcal voci = %v, attese 78 / 840 / 52", items)
	}
	if !near(vars[1], 970) {
		t.Errorf("kcal variante = %.2f, attese 970", vars[1])
	}
	if v, i := planKcal(nil, nil); len(v) != 0 || len(i) != 0 {
		t.Error("piano nil: mappe non vuote")
	}
}
