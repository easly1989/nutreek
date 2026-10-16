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

// slotAdherence aggrega l'aderenza media per tipo di pasto (es. "Colazione").
type slotAdherence struct {
	SlotName string
	AvgScore float64
	Count    int // numero di giorni-pasto con sgarro osservato
}

type statsPageData struct {
	authPageData
	View          statsView
	From, To      time.Time // primo lunedi' e ultima domenica mostrati
	SlotAdherence []slotAdherence
	TDEEReport    *service.TDEEReport
	TargetKcal    float64 // 0 se l'utente non ha un obiettivo
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	const numWeeks = 12
	weekStart := db.MondayOf(time.Now())
	from := weekStart.AddDate(0, 0, -7*(numWeeks-1))

	// Aderenza piano vs effettivo: streak giornaliero e breakdown per tipo di
	// pasto, oltre al punteggio di ogni settimana.
	type datedScore struct {
		date  time.Time
		score float64
	}
	var dailyAdh []datedScore // solo giorni pianificati fino a oggi, per lo streak
	slotAgg := map[string]*slotAdherence{}
	var slotOrder []string
	today := todayDate()

	weeks := make([]statsWeek, 0, numWeeks)
	for i := 0; i < numWeeks; i++ {
		ws := from.AddDate(0, 0, 7*i)
		we := ws.AddDate(0, 0, 6)
		sw := statsWeek{Start: ws}
		plan, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, user.ID, ws)
		if err != nil {
			weeks = append(weeks, sw)
			continue
		}
		full, err := a.WeeklyPlans.LoadFull(ctx, plan)
		if err != nil {
			weeks = append(weeks, sw)
			continue
		}

		sw.Total = macros.ForWeek(full.Days)
		for _, dayMeals := range full.Days {
			if len(dayMeals) > 0 {
				sw.DaysLogged++
			}
			for _, meal := range dayMeals {
				if meal.HasCheats() {
					sw.Cheats++
				}
			}
		}

		sw.WorkoutKcal, _ = a.WorkoutLogs.SumKcalForWeek(ctx, user.ID, ws, we)
		wlogs, _ := a.WorkoutLogs.ListWorkoutLogs(ctx, user.ID, ws, we)
		sw.Workouts = len(wlogs)

		dayScores := make([]macros.AdherenceScore, 0, 7)
		for d := 0; d < 7; d++ {
			dayMeals := full.Days[d]
			adh := macros.ScoreDay(dayMeals)
			dayScores = append(dayScores, adh)
			if !adh.PlannedMacros.IsZero() {
				sw.Planned = true
				// Streak: solo giorni con pasti pianificati, non futuri.
				if dayDate := ws.AddDate(0, 0, d); !dayDate.After(today) {
					dailyAdh = append(dailyAdh, datedScore{date: dayDate, score: adh.Score})
				}
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
		sw.Adherence = macros.ScoreWeek(dayScores)

		if a.WaterLogs != nil {
			totals, _ := a.WaterLogs.ForDateRange(ctx, user.ID, ws, we)
			for _, ml := range totals {
				sw.WaterML += ml
			}
		}
		weeks = append(weeks, sw)
	}

	// Streak: giorni pianificati in ordine, conta i piu' recenti
	// consecutivi con score >= 80.
	sort.Slice(dailyAdh, func(i, j int) bool { return dailyAdh[i].date.Before(dailyAdh[j].date) })
	dailyScores := make([]float64, len(dailyAdh))
	for i, ds := range dailyAdh {
		dailyScores[i] = ds.score
	}

	// Breakdown per slot: media e ordinamento per aderenza decrescente.
	slotAdherences := make([]slotAdherence, 0, len(slotOrder))
	for _, name := range slotOrder {
		agg := slotAgg[name]
		agg.AvgScore /= float64(agg.Count)
		slotAdherences = append(slotAdherences, *agg)
	}
	sort.Slice(slotAdherences, func(i, j int) bool { return slotAdherences[i].AvgScore > slotAdherences[j].AvgScore })

	weights, _ := a.WeightLogs.ForRange(ctx, user.ID, from, weekStart.AddDate(0, 0, 6))
	tdeeReport, _ := a.TDEE.Report(ctx, user, numWeeks, weekStart)

	var target float64
	if user.TargetKcal != nil {
		target = *user.TargetKcal
	}

	data := statsPageData{
		authPageData: a.baseData(r),
		View: buildStatsView(statsInput{
			Weeks:         weeks,
			Weights:       weights,
			TDEE:          tdeeReport,
			TargetKcal:    target,
			WaterTargetML: user.WaterTargetML,
			StreakDays:    macros.Streak(dailyScores, 80),
		}),
		From:          from,
		To:            weekStart.AddDate(0, 0, 6),
		SlotAdherence: slotAdherences,
		TDEEReport:    tdeeReport,
		TargetKcal:    target,
	}
	if err := a.Render.Render(w, http.StatusOK, "stats", data); err != nil {
		a.serverError(w, r, err)
	}
}
