package tdee

import (
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestEstimate_Stable(t *testing.T) {
	// Peso stabile → TDEE ≈ kcal assunte
	d := WeekData{AvgKcal: 2000, StartWeight: 80, EndWeight: 80}
	got := Estimate(d)
	if got != 2000 {
		t.Errorf("stable weight: want 2000, got %.2f", got)
	}
}

func TestEstimate_Deficit(t *testing.T) {
	// -0.5 kg/week with 1800 kcal/day → TDEE = (1800*7 + 0.5*7700)/7 = 2350
	d := WeekData{AvgKcal: 1800, StartWeight: 80, EndWeight: 79.5}
	got := Estimate(d)
	want := 2350.0
	if abs(got-want) > 1 {
		t.Errorf("deficit: want %.1f, got %.1f", want, got)
	}
}

func TestEstimate_Surplus(t *testing.T) {
	// +0.3 kg/week with 2500 kcal/day → TDEE = (2500*7 - 0.3*7700)/7 = 2170
	d := WeekData{AvgKcal: 2500, StartWeight: 80, EndWeight: 80.3}
	got := Estimate(d)
	want := (2500*7 - 0.3*7700) / 7
	if abs(got-want) > 1 {
		t.Errorf("surplus: want %.1f, got %.1f", want, got)
	}
}

func TestEstimate_MissingData(t *testing.T) {
	if Estimate(WeekData{AvgKcal: 2000}) != 0 {
		t.Error("without weight it must return 0")
	}
	if Estimate(WeekData{StartWeight: 80, EndWeight: 80}) != 0 {
		t.Error("without kcal it must return 0")
	}
}

func TestEstimateFromHistory_Confidence(t *testing.T) {
	stable := func(i int) WeekData {
		return WeekData{
			WeekStart:   time.Now().AddDate(0, 0, -7*i),
			AvgKcal:     2000,
			StartWeight: 75,
			EndWeight:   75,
		}
	}
	// 1 week → 25%
	_, c1 := EstimateFromHistory([]WeekData{stable(0)})
	if c1 != 25 {
		t.Errorf("1 week: want confidence 25, got %d", c1)
	}
	// 4 settimane → 100%
	weeks := []WeekData{stable(3), stable(2), stable(1), stable(0)}
	tdee4, c4 := EstimateFromHistory(weeks)
	if c4 != 100 {
		t.Errorf("4 weeks: want confidence 100, got %d", c4)
	}
	if abs(tdee4-2000) > 1 {
		t.Errorf("stable weight: want TDEE 2000, got %.1f", tdee4)
	}
}

func TestEstimateFromHistory_SanityFilter(t *testing.T) {
	// A TDEE outside the range [500,8000] must be excluded
	weeks := []WeekData{
		{AvgKcal: 100, StartWeight: 80, EndWeight: 79}, // TDEE = (700+7700)/7 = 1200 — OK
		{AvgKcal: 10, StartWeight: 80, EndWeight: 80},  // TDEE = 10 — escluso (<500)
	}
	tdee, conf := EstimateFromHistory(weeks)
	if tdee == 0 || conf == 0 {
		t.Error("it should find at least one valid week")
	}
	_ = tdee
}

func TestInterpolateWeight(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	logs := []domain.WeightLog{
		{MeasuredAt: base, WeightKg: 80},
		{MeasuredAt: base.AddDate(0, 0, 6), WeightKg: 79.3},
	}

	// Inizio esatto
	if got := InterpolateWeight(logs, base); got != 80 {
		t.Errorf("start: want 80, got %.2f", got)
	}
	// Fine esatta
	if got := InterpolateWeight(logs, base.AddDate(0, 0, 6)); got != 79.3 {
		t.Errorf("end: want 79.3, got %.2f", got)
	}
	// Mid-week: linear interpolation → 79.65
	mid := InterpolateWeight(logs, base.AddDate(0, 0, 3))
	want := 80 + (79.3-80)*3.0/6.0
	if abs(mid-want) > 0.01 {
		t.Errorf("middle: want %.3f, got %.3f", want, mid)
	}
	// Out of range → extrapolation with the nearest end
	after := InterpolateWeight(logs, base.AddDate(0, 0, 10))
	if after != 79.3 {
		t.Errorf("after: want 79.3, got %.2f", after)
	}
}

func TestInterpolateWeight_Empty(t *testing.T) {
	if got := InterpolateWeight(nil, time.Now()); got != 0 {
		t.Errorf("empty: want 0, got %.2f", got)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
