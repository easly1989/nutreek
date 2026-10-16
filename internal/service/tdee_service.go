package service

import (
	"context"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/tdee"
)

// activityMultipliers mappa ActivityLevel → moltiplicatore TDEE formula.
var activityMultipliers = map[string]float64{
	"sedentary": 1.2,
	"light":     1.375,
	"moderate":  1.55,
	"active":    1.725,
	"athlete":   1.9,
}

// TDEEReport holds the adaptive TDEE estimate and the supporting data.
type TDEEReport struct {
	EstimatedTDEE    float64
	Confidence       int     // 0–100: quante settimane valide abbiamo
	FormulaBasedTDEE float64 // BMR × activity (from the formula, 0 when BMR is not computed)
	WeeklyData       []tdee.WeekData
	SuggestionDiff   float64 // EstimatedTDEE − the current target_kcal (0 when there is no target)
	HasEnoughData    bool    // true when there is at least 1 valid week
}

// TDEEService computes the adaptive TDEE by observing weight/intake changes.
type TDEEService struct {
	WeightLogs  *db.WeightLogs
	WeeklyPlans *db.WeeklyPlans
}

// Report computes the user's TDEEReport over the last `numWeeks` weeks.
func (s *TDEEService) Report(ctx context.Context, user *domain.User, numWeeks int, weekStart time.Time) (*TDEEReport, error) {
	if numWeeks <= 0 {
		numWeeks = 12
	}
	from := weekStart.AddDate(0, 0, -7*(numWeeks-1))
	to := weekStart.AddDate(0, 0, 6)

	// Load every weight log in the range (+ a 1-week buffer for the interpolation).
	weightLogs, err := s.WeightLogs.ForRange(ctx, user.ID, from.AddDate(0, 0, -7), to)
	if err != nil {
		return nil, err
	}

	// Build WeekData for every week (from the oldest to the most recent).
	weekData := make([]tdee.WeekData, 0, numWeeks)
	for i := numWeeks - 1; i >= 0; i-- {
		ws := weekStart.AddDate(0, 0, -7*i)
		we := ws.AddDate(0, 0, 6)

		plan, err := s.WeeklyPlans.GetOrCreateForWeek(ctx, user.ID, ws)
		if err != nil {
			continue
		}
		full, err := s.WeeklyPlans.LoadFull(ctx, plan)
		if err != nil {
			continue
		}

		weekMacros := macros.ForWeek(full.Days)
		daysLogged := 0
		for _, dayMeals := range full.Days {
			if len(dayMeals) > 0 {
				daysLogged++
			}
		}
		if daysLogged == 0 || weekMacros.Kcal == 0 {
			continue // a week without meal data: skip it
		}
		avgKcal := weekMacros.Kcal / float64(daysLogged)

		startW := tdee.InterpolateWeight(weightLogs, ws)
		endW := tdee.InterpolateWeight(weightLogs, we)

		weekData = append(weekData, tdee.WeekData{
			WeekStart:   ws,
			AvgKcal:     avgKcal,
			StartWeight: startW,
			EndWeight:   endW,
		})
	}

	estimatedTDEE, confidence := tdee.EstimateFromHistory(weekData)

	report := &TDEEReport{
		EstimatedTDEE: estimatedTDEE,
		Confidence:    confidence,
		WeeklyData:    weekData,
		HasEnoughData: estimatedTDEE > 0,
	}

	// TDEE from the formula (BMR × activity multiplier).
	if user.BMR != nil && *user.BMR > 0 {
		mult := 1.2 // default sedentary
		if user.ActivityLevel != nil {
			if m, ok := activityMultipliers[*user.ActivityLevel]; ok {
				mult = m
			}
		}
		report.FormulaBasedTDEE = *user.BMR * mult
	}

	// Difference from the current target.
	if user.TargetKcal != nil && *user.TargetKcal > 0 && estimatedTDEE > 0 {
		report.SuggestionDiff = estimatedTDEE - *user.TargetKcal
	}

	return report, nil
}
