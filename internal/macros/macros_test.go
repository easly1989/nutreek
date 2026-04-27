package macros

import (
	"math"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestAddAndScale(t *testing.T) {
	a := Macros{Kcal: 100, Protein: 10, Carbs: 20, Fat: 5}
	b := Macros{Kcal: 50, Protein: 4, Carbs: 8, Fat: 2}
	got := a.Add(b)
	want := Macros{Kcal: 150, Protein: 14, Carbs: 28, Fat: 7}
	if got != want {
		t.Fatalf("Add: got %+v want %+v", got, want)
	}

	// Scale 1.5x
	scaled := want.Scale(1.5)
	if !almostEqual(scaled.Kcal, 225) || !almostEqual(scaled.Protein, 21) {
		t.Errorf("Scale: got %+v", scaled)
	}

	// Add non-mutating
	if a.Kcal != 100 {
		t.Error("Add ha mutato il ricevitore")
	}
}

func TestIsZero(t *testing.T) {
	if !(Macros{}).IsZero() {
		t.Error("zero value !IsZero()")
	}
	if (Macros{Kcal: 1}).IsZero() {
		t.Error("non-zero IsZero()")
	}
}

func TestFromIngredient(t *testing.T) {
	ing := domain.Ingredient{
		KcalPer100:    240,
		ProteinPer100: 8,
		CarbsPer100:   44,
		FatPer100:     2,
		Unit:          domain.UnitGram,
	}
	cases := []struct {
		name string
		qty  float64
		want Macros
	}{
		{"50g", 50, Macros{Kcal: 120, Protein: 4, Carbs: 22, Fat: 1}},
		{"100g identita'", 100, Macros{Kcal: 240, Protein: 8, Carbs: 44, Fat: 2}},
		{"0g zero", 0, Macros{}},
		{"negativi zero", -10, Macros{}},
		{"250g", 250, Macros{Kcal: 600, Protein: 20, Carbs: 110, Fat: 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromIngredient(ing, tc.qty)
			if got != tc.want {
				t.Errorf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestForRecipeAndPerServing(t *testing.T) {
	pane := domain.Ingredient{KcalPer100: 240, ProteinPer100: 8, CarbsPer100: 44, FatPer100: 2}
	olio := domain.Ingredient{KcalPer100: 900, ProteinPer100: 0, CarbsPer100: 0, FatPer100: 100}
	items := []IngredientItem{
		{Ingredient: pane, Quantity: 200}, // 480 kcal
		{Ingredient: olio, Quantity: 10},  // 90 kcal
	}
	total := ForRecipe(items)
	if !almostEqual(total.Kcal, 570) {
		t.Errorf("kcal totali: got %v want 570", total.Kcal)
	}
	if !almostEqual(total.Fat, 14) { // 200g pane = 4g + 10g olio = 10g
		t.Errorf("fat totali: got %v want 14", total.Fat)
	}

	per := PerServing(total, 2)
	if !almostEqual(per.Kcal, 285) {
		t.Errorf("per serving kcal: got %v want 285", per.Kcal)
	}

	// servings=0 → zero, no panic
	if !PerServing(total, 0).IsZero() {
		t.Error("servings=0 dovrebbe ritornare Macros{}")
	}
	if !PerServing(total, -1).IsZero() {
		t.Error("servings<0 dovrebbe ritornare Macros{}")
	}
}

func TestForRecipeEmpty(t *testing.T) {
	if !ForRecipe(nil).IsZero() {
		t.Error("ricetta vuota dovrebbe essere zero")
	}
}
