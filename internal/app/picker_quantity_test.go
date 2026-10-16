package app

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

// The quantity proposed in the picker comes only from real data (#136): an
// OFF product never opens on «1 pz», which is the whole package.
func TestPickerQuantity(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name     string
		row      ingredientRow
		qty      float64
		unit     string
		servingG float64
	}{
		{"OFF with a serving", ingredientRow{OFF: &off.Product{UnitWeightG: f(500), ServingG: f(40)}}, 40, "g", 40},
		{"OFF without a serving", ingredientRow{OFF: &off.Product{UnitWeightG: f(500)}}, 0, "g", 0},
		{"OFF piccola porzione resta in g", ingredientRow{OFF: &off.Product{UnitWeightG: f(1000), ServingG: f(5)}}, 5, "g", 5},
		{"imported OFF with a serving", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceOFF, Unit: domain.UnitGram, UnitWeightG: f(500), DefaultQuantity: f(40)}}, 40, "g", 40},
		{"imported OFF without a serving", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceOFF, Unit: domain.UnitGram, UnitWeightG: f(375)}}, 0, "g", 0},
		{"by hand with a piece", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitGram, UnitWeightG: f(150)}}, 1, "pz", 0},
		{"a mano, 2 pezzi predefiniti", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitGram, UnitWeightG: f(60), DefaultQuantity: f(2)}}, 2, "pz", 0},
		{"a mano, 120 g predefiniti", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitGram, UnitWeightG: f(60), DefaultQuantity: f(120)}}, 120, "g", 0},
		{"by hand without a piece", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitMilliliter}}, 0, "ml", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.row.PickQuantity(); got != c.qty {
				t.Errorf("PickQuantity = %v, want %v", got, c.qty)
			}
			if got := c.row.PickUnit(); got != c.unit {
				t.Errorf("PickUnit = %q, want %q", got, c.unit)
			}
			if got := c.row.ServingG(); got != c.servingG {
				t.Errorf("ServingG = %v, want %v", got, c.servingG)
			}
		})
	}
}
