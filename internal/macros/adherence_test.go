package macros

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// pane: 240 kcal/100g, 8 prot, 44 carbs, 2 fat.
var paneAdh = domain.Ingredient{
	KcalPer100:    240,
	ProteinPer100: 8,
	CarbsPer100:   44,
	FatPer100:     2,
	Unit:          domain.UnitGram,
}

// mealWith builds a meal with an ingredient item of `itemQty` grams and,
// optionally, a cheat of `cheatQty` grams (cheatQty <= 0 = none).
func mealWith(name string, itemQty, cheatQty float64) domain.MealWithItems {
	m := domain.MealWithItems{
		Meal: domain.Meal{Name: name},
		Items: []domain.MealItemFull{
			{MealItem: domain.MealItem{Quantity: itemQty}, Ingredient: &paneAdh},
		},
	}
	if cheatQty > 0 {
		m.Cheats = []domain.MealItemFull{
			{MealItem: domain.MealItem{Quantity: cheatQty}, Ingredient: &paneAdh},
		}
	}
	return m
}

func TestScoreDayNoCheats(t *testing.T) {
	day := []domain.MealWithItems{
		mealWith("Colazione", 100, 0),
		mealWith("Pranzo", 200, 0),
	}
	got := ScoreDay(day)
	if got.HasCheats {
		t.Error("HasCheats must be false without cheats")
	}
	if got.Score != 100 {
		t.Errorf("Score: got %v want 100", got.Score)
	}
	// Planned e actual coincidono: 300g pane = 720 kcal.
	if !almostEqual(got.PlannedMacros.Kcal, 720) {
		t.Errorf("PlannedMacros.Kcal: got %v want 720", got.PlannedMacros.Kcal)
	}
	if !almostEqual(got.ActualMacros.Kcal, 720) {
		t.Errorf("ActualMacros.Kcal: got %v want 720", got.ActualMacros.Kcal)
	}
}

func TestScoreDayEmpty(t *testing.T) {
	// A day with no meals: no deviation, Score 100, planned zero.
	got := ScoreDay(nil)
	if got.HasCheats {
		t.Error("HasCheats must be false")
	}
	if got.Score != 100 {
		t.Errorf("Score: got %v want 100", got.Score)
	}
	if !got.PlannedMacros.IsZero() {
		t.Errorf("PlannedMacros not zero: %+v", got.PlannedMacros)
	}
}

func TestScoreDayWithCheat(t *testing.T) {
	// Lunch planned 200g (480 kcal), cheat 220g (528 kcal): +10% kcal.
	// Breakfast 100g (240 kcal) with no cheat.
	// Planned = 720, Actual = 240 + 528 = 768 → +6.66% → Score ~93.3.
	day := []domain.MealWithItems{
		mealWith("Colazione", 100, 0),
		mealWith("Pranzo", 200, 220),
	}
	got := ScoreDay(day)
	if !got.HasCheats {
		t.Fatal("HasCheats must be true")
	}
	if !almostEqual(got.PlannedMacros.Kcal, 720) {
		t.Errorf("PlannedMacros.Kcal: got %v want 720", got.PlannedMacros.Kcal)
	}
	if !almostEqual(got.ActualMacros.Kcal, 768) {
		t.Errorf("ActualMacros.Kcal: got %v want 768", got.ActualMacros.Kcal)
	}
	wantDiff := (768.0 - 720.0) / 720.0 * 100
	if !almostEqual(got.KcalDiffPct, wantDiff) {
		t.Errorf("KcalDiffPct: got %v want %v", got.KcalDiffPct, wantDiff)
	}
	wantScore := 100 - wantDiff
	if !almostEqual(got.Score, wantScore) {
		t.Errorf("Score: got %v want %v", got.Score, wantScore)
	}
}

func TestScoreDayCheatClampedToZero(t *testing.T) {
	// Sgarro enorme: deviazione > 100% → Score clampato a 0.
	day := []domain.MealWithItems{
		mealWith("Pranzo", 100, 500), // planned 240, actual 1200 → +400%
	}
	got := ScoreDay(day)
	if got.Score != 0 {
		t.Errorf("Score: got %v want 0 (clamp)", got.Score)
	}
}

func TestScoreWeek(t *testing.T) {
	days := []AdherenceScore{
		{PlannedMacros: Macros{Kcal: 700}, Score: 100},
		{PlannedMacros: Macros{Kcal: 700}, Score: 80},
		{PlannedMacros: Macros{}, Score: 100}, // empty day: excluded
	}
	got := ScoreWeek(days)
	if !almostEqual(got, 90) {
		t.Errorf("ScoreWeek: got %v want 90 (the average of 100 and 80)", got)
	}

	// No day with meals.
	if v := ScoreWeek([]AdherenceScore{{PlannedMacros: Macros{}}}); v != 0 {
		t.Errorf("empty ScoreWeek: got %v want 0", v)
	}
}

func TestStreak(t *testing.T) {
	cases := []struct {
		name      string
		scores    []float64
		threshold float64
		want      int
	}{
		{"all above", []float64{100, 90, 85, 100}, 80, 4},
		{"interrotto a meta'", []float64{100, 50, 90, 100}, 80, 2},
		{"ultimo sotto soglia", []float64{100, 100, 70}, 80, 0},
		{"empty", nil, 80, 0},
		{"esattamente soglia conta", []float64{80, 80}, 80, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Streak(c.scores, c.threshold); got != c.want {
				t.Errorf("Streak: got %d want %d", got, c.want)
			}
		})
	}
}
