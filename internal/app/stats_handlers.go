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

type statsPageData struct {
	authPageData
	Weeks    []weekStatsRow
	Averages macros.Macros
	HasData  bool
	MaxKcal  float64 // per la sparkbar CSS
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	// Calcola le ultime 12 settimane (dalla corrente inclusa).
	now := time.Now()
	weekStart := mondayOf(now)

	var rows []weekStatsRow
	var sumTotal macros.Macros
	var countNonEmpty int

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

	data := statsPageData{
		authPageData: a.baseData(r),
		Weeks:        rows,
		Averages:     averages,
		HasData:      countNonEmpty > 0,
		MaxKcal:      maxKcal,
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
