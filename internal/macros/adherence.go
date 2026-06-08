package macros

import (
	"math"

	"github.com/amon2126/nutreek/internal/domain"
)

// AdherenceScore descrive quanto un giorno pianificato e' stato rispettato:
// confronta i macro "pianificati" (gli item ufficiali dei pasti) con quelli
// "effettivi" (sgarri al posto degli item dove presenti). Senza sgarri il
// giorno e' perfettamente aderente (Score 100).
type AdherenceScore struct {
	HasCheats      bool
	PlannedMacros  Macros
	ActualMacros   Macros
	KcalDiffPct    float64 // (actual - planned) / planned * 100
	ProteinDiffPct float64
	Score          float64 // 0–100: 100 = nessuno sgarro; scende con la diff kcal
}

// ScoreDay calcola l'aderenza di un giorno dato i pasti pianificati.
// PlannedMacros somma sempre gli item ufficiali; ActualMacros usa gli sgarri
// al posto degli item nei pasti che ne hanno. Score = 100 - |KcalDiffPct|,
// clampato a [0,100]. Un giorno senza pasti pianificati ha PlannedMacros zero
// e Score 100 (nessuno scostamento da misurare).
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

// diffPct ritorna la differenza percentuale (actual - planned) / planned * 100.
// Se planned e' 0 ma actual no, ritorna 100 (deviazione massima convenzionale);
// se entrambi 0, ritorna 0.
func diffPct(planned, actual float64) float64 {
	if planned == 0 {
		if actual == 0 {
			return 0
		}
		return 100
	}
	return (actual - planned) / planned * 100
}

// ScoreWeek media i punteggi dei soli giorni con almeno un pasto pianificato.
// I giorni vuoti non diluiscono la media. Ritorna 0 se nessun giorno ha pasti.
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

// Streak conta i giorni consecutivi piu' recenti con Score >= threshold.
// dailyScores e' atteso in ordine cronologico (dal piu' vecchio al piu'
// recente): il conteggio parte dalla fine e si ferma al primo giorno sotto
// soglia.
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
