package app

import (
	"net/http"
	"sort"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/service"
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

type adherenceWeekPoint struct {
	WeekLabel string
	Score     float64 // 0-100 aderenza media della settimana
	HasData   bool    // false = settimana senza pasti pianificati (punto null nel grafico)
}

// slotAdherence aggrega l'aderenza media per tipo di pasto (es. "Colazione").
type slotAdherence struct {
	SlotName string
	AvgScore float64
	Count    int // numero di giorni-pasto con sgarro osservato
}

type statsPageData struct {
	authPageData
	Weeks    []weekStatsRow
	Averages macros.Macros
	HasData  bool
	MaxKcal  float64 // per la sparkbar CSS

	WeightPoints   []weightPoint
	WorkoutWeeks   []workoutWeekPoint
	CheatWeeks     []cheatWeekPoint
	WaterWeeks     []waterWeekPoint
	HasWeightData  bool
	HasWorkoutData bool
	HasCheatData   bool
	HasWaterData   bool
	AvgWorkouts    float64
	AvgCheats      float64
	AvgWaterML     float64

	TDEEReport     *service.TDEEReport
	UserTargetKcal float64 // valore dereferenziato di User.TargetKcal (0 se non impostato)

	StreakDays       int                  // giorni consecutivi recenti senza sgarri
	AdherenceWeeks   []adherenceWeekPoint // score aderenza per settimana (cronologico)
	SlotAdherence    []slotAdherence      // aderenza media per tipo di pasto
	HasAdherenceData bool                 // true se esiste almeno un giorno pianificato
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	now := time.Now()
	weekStart := db.MondayOf(now)

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

	// Aderenza piano vs effettivo: punteggio per settimana, streak giornaliero
	// e breakdown per tipo di pasto.
	var adherenceWeeks []adherenceWeekPoint
	type datedScore struct {
		date  time.Time
		score float64
	}
	var dailyAdh []datedScore // solo giorni pianificati fino a oggi, per lo streak
	slotAgg := map[string]*slotAdherence{}
	var slotOrder []string
	today := todayDate()

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

		// Aderenza: punteggio per ciascun giorno, media settimanale, streak e
		// breakdown per slot (nome pasto).
		dayScores := make([]macros.AdherenceScore, 0, 7)
		for d := 0; d < 7; d++ {
			dayMeals := full.Days[d]
			adh := macros.ScoreDay(dayMeals)
			dayScores = append(dayScores, adh)
			dayDate := ws.AddDate(0, 0, d)
			// Streak: solo giorni con pasti pianificati, non futuri.
			if !adh.PlannedMacros.IsZero() && !dayDate.After(today) {
				dailyAdh = append(dailyAdh, datedScore{date: dayDate, score: adh.Score})
			}
			// Breakdown per slot: aderenza del singolo pasto.
			for _, meal := range dayMeals {
				ms := macros.ScoreDay([]domain.MealWithItems{meal})
				if ms.PlannedMacros.IsZero() {
					continue
				}
				agg, ok := slotAgg[meal.Name]
				if !ok {
					agg = &slotAdherence{SlotName: meal.Name}
					slotAgg[meal.Name] = agg
					slotOrder = append(slotOrder, meal.Name)
				}
				agg.AvgScore += ms.Score // somma, mediata dopo il loop
				agg.Count++
			}
		}
		weekHasPlanned := false
		for _, ds := range dayScores {
			if !ds.PlannedMacros.IsZero() {
				weekHasPlanned = true
				break
			}
		}
		adherenceWeeks = append(adherenceWeeks, adherenceWeekPoint{
			WeekLabel: label,
			Score:     macros.ScoreWeek(dayScores),
			HasData:   weekHasPlanned,
		})

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
	reverseAdherenceWeeks := make([]adherenceWeekPoint, len(adherenceWeeks))
	for i, v := range adherenceWeeks {
		reverseAdherenceWeeks[len(adherenceWeeks)-1-i] = v
	}

	// Streak: ordina i giorni pianificati cronologicamente e conta i piu'
	// recenti consecutivi con score >= 80.
	sort.Slice(dailyAdh, func(i, j int) bool {
		return dailyAdh[i].date.Before(dailyAdh[j].date)
	})
	dailyScores := make([]float64, len(dailyAdh))
	for i, ds := range dailyAdh {
		dailyScores[i] = ds.score
	}
	streakDays := macros.Streak(dailyScores, 80)

	// Breakdown per slot: media e ordinamento per aderenza decrescente.
	slotAdherences := make([]slotAdherence, 0, len(slotOrder))
	for _, name := range slotOrder {
		agg := slotAgg[name]
		if agg.Count > 0 {
			agg.AvgScore /= float64(agg.Count)
		}
		slotAdherences = append(slotAdherences, *agg)
	}
	sort.Slice(slotAdherences, func(i, j int) bool {
		return slotAdherences[i].AvgScore > slotAdherences[j].AvgScore
	})

	var avgWorkouts, avgCheats, avgWaterML float64
	if countNonEmpty > 0 {
		avgWorkouts = float64(totalWorkoutSessions) / float64(countNonEmpty)
		avgCheats = float64(totalCheats) / float64(countNonEmpty)
	}
	if countWaterWeeks > 0 {
		avgWaterML = sumWaterAvg / float64(countWaterWeeks)
	}

	// TDEE adattivo: stima dal trend peso/intake.
	tdeeReport, _ := a.TDEE.Report(ctx, user, 12, weekStart)

	var userTargetKcal float64
	if user.TargetKcal != nil {
		userTargetKcal = *user.TargetKcal
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
		TDEEReport:     tdeeReport,
		UserTargetKcal: userTargetKcal,

		StreakDays:       streakDays,
		AdherenceWeeks:   reverseAdherenceWeeks,
		SlotAdherence:    slotAdherences,
		HasAdherenceData: len(dailyAdh) > 0,
	}
	if err := a.Render.Render(w, http.StatusOK, "stats", data); err != nil {
		a.serverError(w, r, err)
	}
}

