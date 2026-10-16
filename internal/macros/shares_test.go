package macros

import (
	"math"
	"testing"
)

func TestEnergyShares(t *testing.T) {
	// 15 g P (60 kcal), 61 g C (244 kcal), 12 g G (108 kcal) su 412 kcal.
	s := Macros{Protein: 15, Carbs: 61, Fat: 12}.EnergyShares()
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.05 }
	if !near(s.Protein, 14.56) || !near(s.Carbs, 59.22) || !near(s.Fat, 26.21) {
		t.Errorf("EnergyShares = %+v, atteso 14,56 / 59,22 / 26,21", s)
	}
	if z := (Macros{Kcal: 10}).EnergyShares(); z != (Shares{}) {
		t.Errorf("senza macro = %+v, atteso zero", z)
	}
}
