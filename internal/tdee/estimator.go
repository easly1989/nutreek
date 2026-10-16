package tdee

import (
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

const kcalPerKg = 7700.0 // kcal ≈ 1 kg grasso corporeo

// WeekData collects the data of a single week needed for the TDEE estimate.
type WeekData struct {
	WeekStart   time.Time
	AvgKcal     float64 // average kcal/day from the meal planning
	StartWeight float64 // weight interpolated at the start of the week (0 = not available)
	EndWeight   float64 // weight interpolated at the end of the week (0 = not available)
}

// WeightDelta returns the weight change in kg for the week.
func (w WeekData) WeightDelta() float64 { return w.EndWeight - w.StartWeight }

// EstimatedKcal is the TDEE estimated for this week (0 when the data is not enough).
// Exposed as a method so Go templates can call it directly.
func (w WeekData) EstimatedKcal() float64 { return Estimate(w) }

// WeekLabel formats the week's label (dd/mm) for the charts.
func (w WeekData) WeekLabel() string { return w.WeekStart.Format("02/01") }

// Estimate estimates the daily TDEE from a single week.
// It returns 0 when the data is not enough.
func Estimate(d WeekData) float64 {
	if d.AvgKcal == 0 || d.StartWeight == 0 || d.EndWeight == 0 {
		return 0
	}
	weeklyKcal := d.AvgKcal * 7
	// TDEE = kcal_assunte - kcal_nette_da_variazione_peso
	// Δpeso > 0 → surplus calorico → TDEE < kcal assunte
	// Δpeso < 0 → deficit calorico → TDEE > kcal assunte
	tdee := (weeklyKcal - d.WeightDelta()*kcalPerKg) / 7
	return tdee
}

// EstimateFromHistory computes the weighted average TDEE over the weeks with
// complete data. The most recent weeks weigh more (an exponential average).
// It returns (0, 0) when there are no valid weeks.
func EstimateFromHistory(weeks []WeekData) (tdee float64, confidence int) {
	var valid []float64
	// the input weeks are ordered from the oldest to the most recent
	for _, w := range weeks {
		t := Estimate(w)
		if t > 500 && t < 8000 { // sanity check: TDEE umano plausibile
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		return 0, 0
	}
	var sum, totalWeight float64
	for i, v := range valid {
		w := float64(i + 1) // growing weight: the latest weeks count more
		sum += v * w
		totalWeight += w
	}
	n := len(valid)
	if n >= 4 {
		confidence = 100
	} else {
		confidence = n * 25
	}
	return sum / totalWeight, confidence
}

// InterpolateWeight estimates the weight at date `at` by interpolating
// linearly between the available measurements. The logs need not be sorted
// by date (the function sorts them internally).
// It returns 0 when there are not enough measurements to interpolate.
func InterpolateWeight(logs []domain.WeightLog, at time.Time) float64 {
	if len(logs) == 0 {
		return 0
	}
	// Look for the exact measurement
	for _, l := range logs {
		if sameDay(l.MeasuredAt, at) {
			return l.WeightKg
		}
	}
	// Find the previous and the next neighbor
	var before, after *domain.WeightLog
	for i := range logs {
		l := &logs[i]
		if l.MeasuredAt.Before(at) {
			if before == nil || l.MeasuredAt.After(before.MeasuredAt) {
				before = l
			}
		} else {
			if after == nil || l.MeasuredAt.Before(after.MeasuredAt) {
				after = l
			}
		}
	}
	if before == nil && after == nil {
		return 0
	}
	if before == nil {
		return after.WeightKg // extrapolation: use the first available
	}
	if after == nil {
		return before.WeightKg // extrapolation: use the last available
	}
	// Interpolazione lineare
	span := after.MeasuredAt.Sub(before.MeasuredAt).Hours()
	elapsed := at.Sub(before.MeasuredAt).Hours()
	if span == 0 {
		return before.WeightKg
	}
	frac := elapsed / span
	return before.WeightKg + frac*(after.WeightKg-before.WeightKg)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
