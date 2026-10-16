package app

import (
	"fmt"
	"math"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/service"
	"github.com/amon2126/nutreek/internal/web"
)

// statsWeek raccoglie i numeri di una settimana delle Statistiche.
type statsWeek struct {
	Start       time.Time
	Total       macros.Macros // pasti della settimana (esclusi i giorni vuoti)
	DaysLogged  int
	WorkoutKcal float64
	Workouts    int
	Cheats      int
	WaterML     int     // totale della settimana
	Adherence   float64 // 0-100, valida solo se Planned
	Planned     bool
}

// statsInput e' tutto quello che serve per costruire la pagina.
type statsInput struct {
	Weeks         []statsWeek        // dalla piu' vecchia alla piu' recente
	Weights       []domain.WeightLog // in ordine di data
	TDEE          *service.TDEEReport
	TargetKcal    float64
	WaterTargetML int
	StreakDays    int
}

// statsTile e' una card di sintesi; i numeri arrivano gia' formattati.
type statsTile struct {
	Label, Value, Unit, Sub string
}

// statsWeekRow e' una riga della tabella settimana per settimana.
type statsWeekRow struct {
	Start        time.Time
	DaysLogged   int
	PerDay       macros.Macros // medie sui giorni registrati
	Adherence    float64
	HasAdherence bool
}

// statsCharts sono le serie dei grafici, una voce per settimana (null dove
// il dato manca). Passano alla pagina come JSON per static/js/charts.js.
type statsCharts struct {
	Labels       []string   `json:"labels"`
	Target       float64    `json:"target"`
	TDEE         []*float64 `json:"tdee"`
	Prot         []float64  `json:"prot"`
	Carb         []float64  `json:"carb"`
	Fat          []float64  `json:"fat"`
	Adherence    []*float64 `json:"adherence"`
	Workout      []float64  `json:"workout"`
	Cheats       []float64  `json:"cheats"`
	Water        []float64  `json:"water"`
	WaterTarget  float64    `json:"waterTarget"`
	WeightLabels []string   `json:"weightLabels"`
	Weight       []float64  `json:"weight"`
}

type statsView struct {
	AvgDayKcal float64
	Tiles      []statsTile
	Rows       []statsWeekRow // dalla piu' recente
	Charts     statsCharts

	HasMacro, HasWeight, HasTDEEChart, HasAdherence bool
	HasWorkout, HasCheats, HasWater                 bool
}

var monthsShortIT = [12]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"}

// shortDateIT scrive una data come sugli assi dei grafici: «12 ott».
func shortDateIT(t time.Time) string {
	return fmt.Sprintf("%d %s", t.Day(), monthsShortIT[t.Month()-1])
}

func buildStatsView(in statsInput) statsView {
	var v statsView
	n := len(in.Weeks)
	c := statsCharts{
		Target:      in.TargetKcal,
		WaterTarget: float64(in.WaterTargetML) / 1000,
		Labels:      make([]string, n), TDEE: make([]*float64, n), Adherence: make([]*float64, n),
		Prot: make([]float64, n), Carb: make([]float64, n), Fat: make([]float64, n),
		Workout: make([]float64, n), Cheats: make([]float64, n), Water: make([]float64, n),
	}
	tdeeByWeek := map[time.Time]float64{}
	if in.TDEE != nil {
		for _, wd := range in.TDEE.WeeklyData {
			if k := wd.EstimatedKcal(); k > 0 {
				tdeeByWeek[wd.WeekStart] = k
			}
		}
	}

	var kcal float64
	var days, weeksWithMeals, workouts, cheats, waterWeeks, tdeePoints int
	var workoutKcal, waterL float64
	for i, w := range in.Weeks {
		c.Labels[i] = shortDateIT(w.Start)
		row := statsWeekRow{Start: w.Start, DaysLogged: w.DaysLogged, Adherence: w.Adherence, HasAdherence: w.Planned}
		if w.DaysLogged > 0 {
			d := float64(w.DaysLogged)
			row.PerDay = macros.Macros{Kcal: w.Total.Kcal / d, Protein: w.Total.Protein / d, Carbs: w.Total.Carbs / d, Fat: w.Total.Fat / d}
			c.Prot[i], c.Carb[i], c.Fat[i] = row.PerDay.Protein*4, row.PerDay.Carbs*4, row.PerDay.Fat*9
			kcal += w.Total.Kcal
			days += w.DaysLogged
			weeksWithMeals++
			workouts += w.Workouts
			workoutKcal += w.WorkoutKcal
			cheats += w.Cheats
		}
		v.Rows = append([]statsWeekRow{row}, v.Rows...)
		if k, ok := tdeeByWeek[w.Start]; ok {
			c.TDEE[i] = &k
			tdeePoints++
		}
		if w.Planned {
			a := w.Adherence
			c.Adherence[i] = &a
			v.HasAdherence = true
		}
		c.Workout[i] = w.WorkoutKcal
		c.Cheats[i] = float64(w.Cheats)
		c.Water[i] = float64(w.WaterML) / 7 / 1000
		if w.WaterML > 0 {
			waterWeeks++
			waterL += c.Water[i]
		}
		v.HasWorkout = v.HasWorkout || w.WorkoutKcal > 0
		v.HasCheats = v.HasCheats || w.Cheats > 0
	}
	v.HasWater = waterWeeks > 0
	v.HasTDEEChart = in.TDEE != nil && in.TDEE.HasEnoughData && tdeePoints >= 2

	// Peso: solo le pesate nella finestra delle settimane mostrate.
	var weights []domain.WeightLog
	for _, wl := range in.Weights {
		if n > 0 && wl.MeasuredAt.Before(in.Weeks[0].Start) {
			continue
		}
		weights = append(weights, wl)
		c.WeightLabels = append(c.WeightLabels, shortDateIT(wl.MeasuredAt))
		c.Weight = append(c.Weight, wl.WeightKg)
	}
	v.HasWeight = len(weights) >= 2
	v.Charts = c

	dash := "—"
	day := statsTile{Label: "Media al giorno", Value: dash, Unit: "kcal"}
	if days > 0 {
		v.AvgDayKcal = kcal / float64(days)
		v.HasMacro = true
		day.Value = web.FormatNum(v.AvgDayKcal)
	}
	if in.TargetKcal > 0 {
		day.Sub = "obiettivo " + web.FormatNum(in.TargetKcal)
	}

	weight := statsTile{Label: "Peso", Value: dash, Unit: "kg", Sub: "nessuna pesata"}
	if len(weights) > 0 {
		last := weights[len(weights)-1]
		weight.Value = web.FormatDec(last.WeightKg, 1)
		weight.Sub = "una sola pesata"
		if len(weights) >= 2 {
			weight.Sub = weightTrend(last.WeightKg-weights[0].WeightKg, last.MeasuredAt.Sub(weights[0].MeasuredAt))
		}
	}

	work := statsTile{Label: "Allenamenti", Value: dash, Unit: "a settimana"}
	cheat := statsTile{Label: "Sgarri", Value: dash, Unit: "a settimana"}
	if weeksWithMeals > 0 {
		wm := float64(weeksWithMeals)
		work.Value = web.FormatDec(float64(workouts)/wm, 1)
		if workoutKcal > 0 {
			work.Sub = web.FormatNum(workoutKcal/wm) + " kcal in media"
		}
		cheat.Value = web.FormatDec(float64(cheats)/wm, 1)
	}
	switch {
	case in.StreakDays == 1:
		cheat.Sub = "1 giorno senza"
	case in.StreakDays > 1:
		cheat.Sub = fmt.Sprintf("%d giorni senza", in.StreakDays)
	}

	water := statsTile{Label: "Acqua", Value: dash, Unit: "l al giorno"}
	if waterWeeks > 0 {
		water.Value = web.FormatDec(waterL/float64(waterWeeks), 1)
	}
	if in.WaterTargetML > 0 {
		water.Sub = "obiettivo " + web.FormatDec(c.WaterTarget, 1) + " l"
	}

	v.Tiles = []statsTile{day, weight, work, cheat, water}
	return v
}

// weightTrend descrive la variazione di peso: «−1,6 kg in 2 settimane».
func weightTrend(delta float64, span time.Duration) string {
	if math.Abs(delta) < 0.05 {
		return "stabile"
	}
	s := web.FormatDec(math.Abs(delta), 1) + " kg"
	if delta < 0 {
		s = "−" + s
	} else {
		s = "+" + s
	}
	d := int(math.Round(span.Hours() / 24))
	switch {
	case d < 2:
		return s
	case d < 14:
		return fmt.Sprintf("%s in %d giorni", s, d)
	default:
		return fmt.Sprintf("%s in %d settimane", s, int(math.Round(float64(d)/7)))
	}
}
