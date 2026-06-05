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

// TDEEReport contiene la stima adattiva del TDEE e i dati di supporto.
type TDEEReport struct {
	EstimatedTDEE    float64
	Confidence       int     // 0–100: quante settimane valide abbiamo
	FormulaBasedTDEE float64 // BMR × activity (dalla formula, 0 se BMR non calcolato)
	WeeklyData       []tdee.WeekData
	SuggestionDiff   float64 // EstimatedTDEE − target_kcal corrente (0 se nessun target)
	HasEnoughData    bool    // true se c'è almeno 1 settimana valida
}

// TDEEService calcola il TDEE adattivo dall'osservazione delle variazioni peso/intake.
type TDEEService struct {
	WeightLogs  *db.WeightLogs
	WeeklyPlans *db.WeeklyPlans
}

// Report calcola il TDEEReport per l'utente sulle ultime `numWeeks` settimane.
func (s *TDEEService) Report(ctx context.Context, user *domain.User, numWeeks int, weekStart time.Time) (*TDEEReport, error) {
	if numWeeks <= 0 {
		numWeeks = 12
	}
	from := weekStart.AddDate(0, 0, -7*(numWeeks-1))
	to := weekStart.AddDate(0, 0, 6)

	// Carica tutti i weight log nel range (+ 1 settimana buffer per l'interpolazione).
	weightLogs, err := s.WeightLogs.ForRange(ctx, user.ID, from.AddDate(0, 0, -7), to)
	if err != nil {
		return nil, err
	}

	// Costruisce WeekData per ogni settimana (dalla più vecchia alla più recente).
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
			continue // settimana senza dati pasto: salta
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

	// TDEE da formula (BMR × moltiplicatore attività).
	if user.BMR != nil && *user.BMR > 0 {
		mult := 1.2 // default sedentary
		if user.ActivityLevel != nil {
			if m, ok := activityMultipliers[*user.ActivityLevel]; ok {
				mult = m
			}
		}
		report.FormulaBasedTDEE = *user.BMR * mult
	}

	// Differenza rispetto al target attuale.
	if user.TargetKcal != nil && *user.TargetKcal > 0 && estimatedTDEE > 0 {
		report.SuggestionDiff = estimatedTDEE - *user.TargetKcal
	}

	return report, nil
}
