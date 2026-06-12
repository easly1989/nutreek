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
)

type weightPageData struct {
	authPageData
	Logs          []domain.WeightLog
	TodayStr      string // YYYY-MM-DD per il default del campo data
	HasTrend      bool
	TrendKg       float64 // differenza vs entry di confronto
	TrendPositive bool
	TrendDays     int
}

func (a *App) handleWeightPage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	logs, err := a.WeightLogs.ListWeightLogs(r.Context(), user.ID, 90)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	data := weightPageData{
		authPageData: a.baseData(r),
		Logs:         logs,
		TodayStr:     time.Now().Format("2006-01-02"),
	}

	// Calcola trend: confronto con l'entry più vicina a ≥6 giorni fa.
	if len(logs) >= 2 {
		latest := logs[0]
		for _, l := range logs[1:] {
			days := int(latest.MeasuredAt.Sub(l.MeasuredAt).Hours() / 24)
			if days >= 6 {
				diff := latest.WeightKg - l.WeightKg
				data.HasTrend = true
				data.TrendKg = diff
				data.TrendPositive = diff > 0
				data.TrendDays = days
				break
			}
		}
	}

	if err := a.Render.Render(w, http.StatusOK, "weight", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleWeightLog(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())

	dateStr := strings.TrimSpace(r.PostFormValue("measured_at"))
	weightStr := strings.TrimSpace(r.PostFormValue("weight_kg"))
	noteStr := strings.TrimSpace(r.PostFormValue("note"))

	renderErr := func(msg string) {
		logs, _ := a.WeightLogs.ListWeightLogs(r.Context(), user.ID, 90)
		data := weightPageData{
			authPageData: a.baseData(r),
			Logs:         logs,
		}
		data.Error = msg
		if err := a.Render.Render(w, http.StatusUnprocessableEntity, "weight", data); err != nil {
			a.serverError(w, r, err)
		}
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		renderErr("Data non valida.")
		return
	}

	weightStr = strings.ReplaceAll(weightStr, ",", ".")
	weightKg, err := strconv.ParseFloat(weightStr, 64)
	if err != nil || weightKg <= 0 || weightKg >= 500 {
		renderErr("Peso non valido (range: 0.1–499 kg).")
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

	a.Sessions.Put(r.Context(), "flash", "Misurazione salvata.")
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

	a.Sessions.Put(r.Context(), "flash", "Misurazione eliminata.")
	http.Redirect(w, r, "/profile/peso", http.StatusSeeOther)
}
