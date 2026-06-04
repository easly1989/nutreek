package app

import (
	"net/http"
	"time"

	"github.com/amon2126/nutreek/internal/macros"
)

type weekStatsRow struct {
	WeekStart  time.Time
	Total      macros.Macros
	DaysLogged int
	KcalBarPct float64 // 0-100, per sparkbar CSS
}

type weightPoint struct {
	Date   string
	Weight float64
}

type workoutWeekPoint struct {
	WeekLabel  string
	KcalBurned float64
}

type cheatWeekPoint struct {
	WeekLabel string
	Count     int
}

type waterWeekPoint struct {
	WeekLabel string
	AvgML     float64 // media ml/giorno (totale settimana / 7)
	BarPct    float64 // 0-100 per la sparkbar CSS
}

type statsPageData struct {
	authPageData
	Weeks    []weekStatsRow
	Averages macros.Macros
	HasData  bool
	MaxKcal  float64 // per la sparkbar CSS

	WeightPoints    []weightPoint
	WorkoutWeeks    []workoutWeekPoint
	CheatWeeks      []cheatWeekPoint
	WaterWeeks      []waterWeekPoint
	HasWeightData   bool
	HasWorkoutData  bool
	HasCheatData    bool
	HasWaterData    bool
	AvgWorkouts     float64
	AvgCheats       float64
	AvgWaterML      float64
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	now := time.Now()
	weekStart := mondayOf(now)

	var rows []weekStatsRow
	var sumTotal macros.Macros
	var countNonEmpty int
	var workoutWeeks []workoutWeekPoint
	var cheatWeeks []cheatWeekPoint
	var waterWeeks []waterWeekPoint
	var hasWorkoutData, hasCheatData, hasWaterData bool
	var totalWorkoutSessions, totalCheats int
	var sumWaterAvg float64
	var countWaterWeeks int

	for i := 0; i < 12; i++ {
		ws := weekStart.AddDate(0, 0, -7*i)
		plan, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, user.ID, ws)
		if err != nil {
			continue
		}
		full, err := a.WeeklyPlans.LoadFull(ctx, plan)
		if err != nil {
			continue
		}

		weekTotal := macros.ForWeek(full.Days)
		daysLogged := 0
		for _, dayMeals := range full.Days {
			if len(dayMeals) > 0 {
				daysLogged++
			}
		}

		rows = append(rows, weekStatsRow{
			WeekStart:  ws,
			Total:      weekTotal,
			DaysLogged: daysLogged,
		})

		if daysLogged > 0 {
			sumTotal = sumTotal.Add(weekTotal)
			countNonEmpty++
		}

		// Kcal e conteggio allenamenti per settimana.
		wkcal, _ := a.WorkoutLogs.SumKcalForWeek(ctx, user.ID, ws, ws.AddDate(0, 0, 6))
		wlogs, _ := a.WorkoutLogs.ListWorkoutLogs(ctx, user.ID, ws, ws.AddDate(0, 0, 6))
		label := ws.Format("02/01")
		workoutWeeks = append(workoutWeeks, workoutWeekPoint{WeekLabel: label, KcalBurned: wkcal})
		if wkcal > 0 {
			hasWorkoutData = true
		}
		totalWorkoutSessions += len(wlogs)

		// Conteggio sgarri per settimana.
		cheatCount := 0
		for _, dayMeals := range full.Days {
			for _, meal := range dayMeals {
				if meal.HasCheats() {
					cheatCount++
				}
			}
		}
		cheatWeeks = append(cheatWeeks, cheatWeekPoint{WeekLabel: label, Count: cheatCount})
		if cheatCount > 0 {
			hasCheatData = true
		}
		totalCheats += cheatCount

		// Idratazione: media ml/giorno della settimana (totale / 7).
		var weekWaterML int
		if a.WaterLogs != nil {
			totals, _ := a.WaterLogs.ForDateRange(ctx, user.ID, ws, ws.AddDate(0, 0, 6))
			for _, ml := range totals {
				weekWaterML += ml
			}
		}
		avgML := float64(weekWaterML) / 7.0
		waterWeeks = append(waterWeeks, waterWeekPoint{WeekLabel: label, AvgML: avgML})
		if weekWaterML > 0 {
			hasWaterData = true
			sumWaterAvg += avgML
			countWaterWeeks++
		}
	}

	var averages macros.Macros
	if countNonEmpty > 0 {
		averages = macros.Macros{
			Kcal:    sumTotal.Kcal / float64(countNonEmpty),
			Protein: sumTotal.Protein / float64(countNonEmpty),
			Carbs:   sumTotal.Carbs / float64(countNonEmpty),
			Fat:     sumTotal.Fat / float64(countNonEmpty),
		}
	}

	var maxKcal float64
	for _, row := range rows {
		if row.Total.Kcal > maxKcal {
			maxKcal = row.Total.Kcal
		}
	}
	if maxKcal > 0 {
		for i := range rows {
			rows[i].KcalBarPct = rows[i].Total.Kcal / maxKcal * 100
		}
	}

	// Andamento peso: ultime 90 misurazioni ordinate cronologicamente.
	weightLogs, _ := a.WeightLogs.ListWeightLogs(ctx, user.ID, 90)
	var weightPoints []weightPoint
	for i := len(weightLogs) - 1; i >= 0; i-- {
		l := weightLogs[i]
		weightPoints = append(weightPoints, weightPoint{
			Date:   l.MeasuredAt.Format("02/01"),
			Weight: l.WeightKg,
		})
	}

	// Sparkbar idratazione: percentuale rispetto al picco settimanale.
	var maxWaterAvg float64
	for _, p := range waterWeeks {
		if p.AvgML > maxWaterAvg {
			maxWaterAvg = p.AvgML
		}
	}
	if maxWaterAvg > 0 {
		for i := range waterWeeks {
			waterWeeks[i].BarPct = waterWeeks[i].AvgML / maxWaterAvg * 100
		}
	}

	// Inverti le settimane (cronologico: dalla più vecchia alla più recente).
	reverseWorkoutWeeks := make([]workoutWeekPoint, len(workoutWeeks))
	reverseCheatWeeks := make([]cheatWeekPoint, len(cheatWeeks))
	reverseWaterWeeks := make([]waterWeekPoint, len(waterWeeks))
	for i, v := range workoutWeeks {
		reverseWorkoutWeeks[len(workoutWeeks)-1-i] = v
	}
	for i, v := range cheatWeeks {
		reverseCheatWeeks[len(cheatWeeks)-1-i] = v
	}
	for i, v := range waterWeeks {
		reverseWaterWeeks[len(waterWeeks)-1-i] = v
	}

	var avgWorkouts, avgCheats, avgWaterML float64
	if countNonEmpty > 0 {
		avgWorkouts = float64(totalWorkoutSessions) / float64(countNonEmpty)
		avgCheats = float64(totalCheats) / float64(countNonEmpty)
	}
	if countWaterWeeks > 0 {
		avgWaterML = sumWaterAvg / float64(countWaterWeeks)
	}

	data := statsPageData{
		authPageData:   a.baseData(r),
		Weeks:          rows,
		Averages:       averages,
		HasData:        countNonEmpty > 0,
		MaxKcal:        maxKcal,
		WeightPoints:   weightPoints,
		WorkoutWeeks:   reverseWorkoutWeeks,
		CheatWeeks:     reverseCheatWeeks,
		WaterWeeks:     reverseWaterWeeks,
		HasWeightData:  len(weightPoints) > 0,
		HasWorkoutData: hasWorkoutData,
		HasCheatData:   hasCheatData,
		HasWaterData:   hasWaterData,
		AvgWorkouts:    avgWorkouts,
		AvgCheats:      avgCheats,
		AvgWaterML:     avgWaterML,
	}
	if err := a.Render.Render(w, http.StatusOK, "stats", data); err != nil {
		a.serverError(w, r, err)
	}
}

// mondayOf restituisce il lunedì della settimana contenente t.
// Duplica la funzione in db/weekly_plans.go ma evita di dipendere dal package db qui.
func mondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // domenica → 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, t.Location())
}
