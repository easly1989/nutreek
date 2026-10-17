package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
)

type weightPageData struct {
	authPageData
	Logs     []domain.WeightLog // from the most recent
	TodayStr string             // YYYY-MM-DD for the date field's default
	Range    int                // days shown in the chart: 30, 90 or 365
	Trend    string             // «−0,8 kg in 30 days»; empty with fewer than two weigh-ins
	Charts   statsCharts        // only WeightLabels and Weight
	HasChart bool
}

// weightRanges are the chart's ranges; the first is the default.
var weightRanges = []int{90, 30, 365}

// parseWeightRange reads ?giorni=; a value not in the list falls back to the default.
func parseWeightRange(r *http.Request) int {
	if n, err := strconv.Atoi(r.URL.Query().Get("giorni")); err == nil {
		for _, v := range weightRanges {
			if n == v {
				return n
			}
		}
	}
	return weightRanges[0]
}

func (a *App) handleWeightPage(w http.ResponseWriter, r *http.Request) {
	data, err := a.weightPage(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Render.Render(w, r, http.StatusOK, "weight", data); err != nil {
		a.serverError(w, r, err)
	}
}

// weightPage prepares the page: the list of weigh-ins and the chart of the
// chosen range, with the change from the first to the last.
func (a *App) weightPage(r *http.Request) (weightPageData, error) {
	user := userFromContext(r.Context())
	data := weightPageData{
		authPageData: a.baseData(r),
		TodayStr:     time.Now().Format("2006-01-02"),
		Range:        parseWeightRange(r),
	}
	logs, err := a.WeightLogs.ListWeightLogs(r.Context(), user.ID, 365)
	if err != nil {
		return data, err
	}
	data.Logs = logs
	today := todayDate()
	inRange, err := a.WeightLogs.ForRange(r.Context(), user.ID, today.AddDate(0, 0, -data.Range), today)
	if err != nil {
		return data, err
	}
	for _, l := range inRange {
		data.Charts.WeightLabels = append(data.Charts.WeightLabels, shortDateIT(l.MeasuredAt))
		data.Charts.Weight = append(data.Charts.Weight, l.WeightKg)
	}
	data.HasChart = len(inRange) >= 2
	if data.HasChart {
		first, last := inRange[0], inRange[len(inRange)-1]
		data.Trend = weightTrend(i18n.FromContext(r.Context()), last.WeightKg-first.WeightKg, last.MeasuredAt.Sub(first.MeasuredAt))
	}
	return data, nil
}

func (a *App) handleWeightLog(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())

	dateStr := strings.TrimSpace(r.PostFormValue("measured_at"))
	weightStr := strings.TrimSpace(r.PostFormValue("weight_kg"))
	noteStr := strings.TrimSpace(r.PostFormValue("note"))

	renderErr := func(msg string) {
		data, err := a.weightPage(r)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Error = msg
		if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "weight", data); err != nil {
			a.serverError(w, r, err)
		}
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		renderErr(a.t(r, "weight.error.date"))
		return
	}

	weightStr = strings.ReplaceAll(weightStr, ",", ".")
	weightKg, err := strconv.ParseFloat(weightStr, 64)
	if err != nil || weightKg <= 0 || weightKg >= 500 {
		renderErr(a.t(r, "weight.error.weight"))
		return
	}

	var notePtr *string
	if noteStr != "" {
		notePtr = &noteStr
	}

	if _, err := a.WeightLogs.LogWeight(r.Context(), user.ID, date, weightKg, notePtr); err != nil {
		a.serverError(w, r, err)
		return
	}

	a.Sessions.Put(r.Context(), "flash", "weight.flash.saved")
	http.Redirect(w, r, "/profile/peso", http.StatusSeeOther)
}

func (a *App) handleWeightLogDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := a.WeightLogs.DeleteWeightLog(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, db.ErrWeightLogNotFound) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}

	a.Sessions.Put(r.Context(), "flash", "weight.flash.deleted")
	http.Redirect(w, r, "/profile/peso", http.StatusSeeOther)
}
