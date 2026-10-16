package app

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

// La quantita' proposta nel picker viene solo da dati veri (#136): un
// prodotto OFF non si apre mai su «1 pz», che vale l'intera confezione.
func TestPickerQuantity(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name     string
		row      ingredientRow
		qty      float64
		unit     string
		servingG float64
	}{
		{"OFF con porzione", ingredientRow{OFF: &off.Product{UnitWeightG: f(500), ServingG: f(40)}}, 40, "g", 40},
		{"OFF senza porzione", ingredientRow{OFF: &off.Product{UnitWeightG: f(500)}}, 0, "g", 0},
		{"OFF piccola porzione resta in g", ingredientRow{OFF: &off.Product{UnitWeightG: f(1000), ServingG: f(5)}}, 5, "g", 5},
		{"OFF importato con porzione", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceOFF, Unit: domain.UnitGram, UnitWeightG: f(500), DefaultQuantity: f(40)}}, 40, "g", 40},
		{"OFF importato senza porzione", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceOFF, Unit: domain.UnitGram, UnitWeightG: f(375)}}, 0, "g", 0},
		{"a mano con il pezzo", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitGram, UnitWeightG: f(150)}}, 1, "pz", 0},
		{"a mano, 2 pezzi predefiniti", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitGram, UnitWeightG: f(60), DefaultQuantity: f(2)}}, 2, "pz", 0},
		{"a mano, 120 g predefiniti", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitGram, UnitWeightG: f(60), DefaultQuantity: f(120)}}, 120, "g", 0},
		{"a mano senza pezzo", ingredientRow{Local: &domain.Ingredient{Source: domain.IngredientSourceLocal, Unit: domain.UnitMilliliter}}, 0, "ml", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.row.PickQuantity(); got != c.qty {
				t.Errorf("PickQuantity = %v, atteso %v", got, c.qty)
			}
			if got := c.row.PickUnit(); got != c.unit {
				t.Errorf("PickUnit = %q, atteso %q", got, c.unit)
			}
			if got := c.row.ServingG(); got != c.servingG {
				t.Errorf("ServingG = %v, atteso %v", got, c.servingG)
			}
		})
	}
}
