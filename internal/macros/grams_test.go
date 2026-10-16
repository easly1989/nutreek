package macros

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestGrams(t *testing.T) {
	peso := 150.0
	zero := 0.0
	mela := &domain.Ingredient{UnitWeightG: &peso}
	senzaPeso := &domain.Ingredient{}
	pesoZero := &domain.Ingredient{UnitWeightG: &zero}
	cases := []struct {
		name string
		qty  float64
		unit string
		ing  *domain.Ingredient
		want float64
	}{
		{"pieces with a unit weight", 2, "pz", mela, 300},
		{"grammi restano grammi", 80, "g", mela, 80},
		{"millilitri restano millilitri", 200, "ml", mela, 200},
		{"pieces without a unit weight", 1, "pz", senzaPeso, 1},
		{"peso unitario zero ignorato", 1, "pz", pesoZero, 1},
		{"nil ingredient", 3, "pz", nil, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Grams(tc.qty, tc.unit, tc.ing); got != tc.want {
				t.Errorf("Grams(%v, %q) = %v, want %v", tc.qty, tc.unit, got, tc.want)
			}
		})
	}
}
