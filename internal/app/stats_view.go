package app

import (
	"math"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/service"
	"github.com/amon2126/nutreek/internal/web"
)

// statsWeek collects the numbers of one week of the Statistics.
type statsWeek struct {
	Start       time.Time
	Total       macros.Macros // the week's meals (empty days excluded)
	DaysLogged  int
	WorkoutKcal float64
	Workouts    int
	Cheats      int
	WaterML     int     // the week's total
	Adherence   float64 // 0-100, valid only when Planned
	Planned     bool
}

// statsInput is everything needed to build the page.
type statsInput struct {
	Lang          string             // number and date formats and the texts of the tiles; empty = Italian
	Weeks         []statsWeek        // from the oldest to the most recent
	Weights       []domain.WeightLog // in date order
	TDEE          *service.TDEEReport
	TargetKcal    float64
	WaterTargetML int
	StreakDays    int
}

// statsTile is a summary card; the numbers arrive already formatted.
type statsTile struct {
	Label, Value, Unit, Sub string
}

// statsWeekRow is a row of the week-by-week table.
type statsWeekRow struct {
	Start        time.Time
	DaysLogged   int
	PerDay       macros.Macros // averages over the logged days
	Adherence    float64
	HasAdherence bool
}

// statsCharts are the chart series, one entry per week (null where the data
// is missing). They reach the page as JSON for static/js/charts.js.
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
	Rows       []statsWeekRow // from the most recent
	Charts     statsCharts

	HasMacro, HasWeight, HasTDEEChart, HasAdherence bool
	HasWorkout, HasCheats, HasWater                 bool
}

func buildStatsView(in statsInput) statsView {
	var v statsView
	tr := func(key string, args ...any) string { return i18n.T(in.Lang, key, args...) }
	n := len(in.Weeks)
	c := statsCharts{
		Target:      in.TargetKcal,
		WaterTarget: float64(in.WaterTargetML) / 1000,
		Labels:      make([]string, n), TDEE: make([]*float64, n), Adherence: make([]*float64, n),
		Prot: make([]float64, n), Carb: make([]float64, n), Fat: make([]float64, n),
		Workout: make([]float64, n), Cheats: make([]float64, n), Water: make([]float64, n),
	}
	// Key by date, not time.Time: two equal instants in different time zones
	// would not be the same key.
	tdeeByWeek := map[string]float64{}
	if in.TDEE != nil {
		for _, wd := range in.TDEE.WeeklyData {
			if k := wd.EstimatedKcal(); k > 0 {
				tdeeByWeek[wd.WeekStart.Format("2006-01-02")] = k
			}
		}
	}

	var kcal float64
	var days, weeksWithMeals, workouts, cheats, waterWeeks, tdeePoints int
	var workoutKcal, waterL float64
	for i, w := range in.Weeks {
		c.Labels[i] = web.ShortDate(in.Lang, w.Start)
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
		if k, ok := tdeeByWeek[w.Start.Format("2006-01-02")]; ok {
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

	// Weight: only the weigh-ins within the window of the weeks shown.
	var weights []domain.WeightLog
	for _, wl := range in.Weights {
		if n > 0 && wl.MeasuredAt.Before(in.Weeks[0].Start) {
			continue
		}
		weights = append(weights, wl)
		c.WeightLabels = append(c.WeightLabels, web.ShortDate(in.Lang, wl.MeasuredAt))
		c.Weight = append(c.Weight, wl.WeightKg)
	}
	v.HasWeight = len(weights) >= 2
	v.Charts = c

	dash := "—"
	day := statsTile{Label: tr("stats.tile.avg_day"), Value: dash, Unit: "kcal"}
	if days > 0 {
		v.AvgDayKcal = kcal / float64(days)
		v.HasMacro = true
		day.Value = web.FormatNum(in.Lang, v.AvgDayKcal)
	}
	if in.TargetKcal > 0 {
		day.Sub = tr("stats.tile.target", "Value", web.FormatNum(in.Lang, in.TargetKcal))
	}

	weight := statsTile{Label: tr("stats.weight.title"), Value: dash, Unit: "kg", Sub: tr("stats.tile.no_weigh_in")}
	if len(weights) > 0 {
		last := weights[len(weights)-1]
		weight.Value = web.FormatDec(in.Lang, last.WeightKg, 1)
		weight.Sub = tr("stats.tile.one_weigh_in")
		if len(weights) >= 2 {
			weight.Sub = weightTrend(in.Lang, last.WeightKg-weights[0].WeightKg, last.MeasuredAt.Sub(weights[0].MeasuredAt))
		}
	}

	work := statsTile{Label: tr("stats.workout.title"), Value: dash, Unit: tr("stats.tile.per_week")}
	cheat := statsTile{Label: tr("stats.cheats.title"), Value: dash, Unit: tr("stats.tile.per_week")}
	if weeksWithMeals > 0 {
		wm := float64(weeksWithMeals)
		work.Value = web.FormatDec(in.Lang, float64(workouts)/wm, 1)
		if workoutKcal > 0 {
			work.Sub = tr("stats.tile.kcal_avg", "Value", web.FormatNum(in.Lang, workoutKcal/wm))
		}
		cheat.Value = web.FormatDec(in.Lang, float64(cheats)/wm, 1)
	}
	if in.StreakDays > 0 {
		cheat.Sub = tr("stats.tile.days_clean", "Count", in.StreakDays)
	}

	water := statsTile{Label: tr("stats.water.title"), Value: dash, Unit: tr("stats.water.unit")}
	if waterWeeks > 0 {
		water.Value = web.FormatDec(in.Lang, waterL/float64(waterWeeks), 1)
	}
	if in.WaterTargetML > 0 {
		water.Sub = tr("stats.tile.target", "Value", web.FormatDec(in.Lang, c.WaterTarget, 1)+" l")
	}

	v.Tiles = []statsTile{day, weight, work, cheat, water}
	return v
}

// weightTrend describes the weight change in lang: «−1,6 kg in 2 settimane».
func weightTrend(lang string, delta float64, span time.Duration) string {
	if math.Abs(delta) < 0.05 {
		return i18n.T(lang, "stats.trend.steady")
	}
	s := web.FormatDec(lang, math.Abs(delta), 1) + " kg"
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
		return i18n.T(lang, "stats.trend.days", "Delta", s, "Count", d)
	default:
		return i18n.T(lang, "stats.trend.weeks", "Delta", s, "Count", int(math.Round(float64(d)/7)))
	}
}
