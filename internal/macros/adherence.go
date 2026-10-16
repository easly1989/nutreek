package macros

import (
	"math"

	"github.com/amon2126/nutreek/internal/domain"
)

// AdherenceScore describes how well a planned day was followed: it compares
// the "planned" macros (the meals' official items) with the "actual" ones
// (cheats in place of the items where present). Without cheats the day is
// perfectly adherent (Score 100).
type AdherenceScore struct {
	HasCheats      bool
	PlannedMacros  Macros
	ActualMacros   Macros
	KcalDiffPct    float64 // (actual - planned) / planned * 100
	ProteinDiffPct float64
	Score          float64 // 0–100: 100 = no cheat; drops with the kcal difference
}

// ScoreDay computes a day's adherence from its planned meals.
// PlannedMacros always sums the official items; ActualMacros uses the cheats
// in place of the items in the meals that have them. Score = 100 -
// |KcalDiffPct|, clamped to [0,100]. A day with no planned meals has zero
// PlannedMacros and Score 100 (no deviation to measure).
func ScoreDay(meals []domain.MealWithItems) AdherenceScore {
	var planned, actual Macros
	hasCheats := false
	for _, m := range meals {
		planned = planned.Add(ForMeal(m.Items))
		if m.HasCheats() {
			hasCheats = true
			actual = actual.Add(ForMeal(m.Cheats))
		} else {
			actual = actual.Add(ForMeal(m.Items))
		}
	}

	score := AdherenceScore{
		HasCheats:     hasCheats,
		PlannedMacros: planned,
		ActualMacros:  actual,
	}

	if !hasCheats {
		score.Score = 100
		return score
	}

	score.KcalDiffPct = diffPct(planned.Kcal, actual.Kcal)
	score.ProteinDiffPct = diffPct(planned.Protein, actual.Protein)

	dev := math.Abs(score.KcalDiffPct)
	score.Score = math.Max(0, 100-dev)
	return score
}

// diffPct returns the percentage difference (actual - planned) / planned * 100.
// When planned is 0 but actual is not, it returns 100 (the conventional
// maximum deviation); when both are 0, it returns 0.
func diffPct(planned, actual float64) float64 {
	if planned == 0 {
		if actual == 0 {
			return 0
		}
		return 100
	}
	return (actual - planned) / planned * 100
}

// ScoreWeek averages the scores of only the days with at least one planned
// meal. Empty days do not dilute the average. Returns 0 when no day has meals.
func ScoreWeek(days []AdherenceScore) float64 {
	var sum float64
	var n int
	for _, d := range days {
		if d.PlannedMacros.IsZero() {
			continue
		}
		sum += d.Score
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// Streak counts the most recent consecutive days with Score >= threshold.
// dailyScores is expected in chronological order (from the oldest to the
// most recent): the count starts from the end and stops at the first day
// under the threshold.
func Streak(dailyScores []float64, threshold float64) int {
	n := 0
	for i := len(dailyScores) - 1; i >= 0; i-- {
		if dailyScores[i] < threshold {
			break
		}
		n++
	}
	return n
}
