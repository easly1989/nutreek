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
		{"pezzi con peso unitario", 2, "pz", mela, 300},
		{"grammi restano grammi", 80, "g", mela, 80},
		{"millilitri restano millilitri", 200, "ml", mela, 200},
		{"pezzi senza peso unitario", 1, "pz", senzaPeso, 1},
		{"peso unitario zero ignorato", 1, "pz", pesoZero, 1},
		{"ingrediente nil", 3, "pz", nil, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Grams(tc.qty, tc.unit, tc.ing); got != tc.want {
				t.Errorf("Grams(%v, %q) = %v, atteso %v", tc.qty, tc.unit, got, tc.want)
			}
		})
	}
}
