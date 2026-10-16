package app

import (
	"errors"
	"net/http"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/planner"
	"github.com/amon2126/nutreek/internal/service"
)

// handleGeneratePlan POST /week/generate — generates the week's plan
// automatically, drawing from the pool of variations of the nutrition plan
// and aiming at the user's macro targets.
func (a *App) handleGeneratePlan(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	if v := r.PostFormValue("start"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}

	target := planner.DayTarget{}
	if user.TargetKcal != nil {
		target.Kcal = *user.TargetKcal
	}
	if user.TargetProtein != nil {
		target.Protein = *user.TargetProtein
	}
	if user.TargetCarbs != nil {
		target.Carbs = *user.TargetCarbs
	}
	if user.TargetFat != nil {
		target.Fat = *user.TargetFat
	}

	err := a.Planner.GenerateWeek(r.Context(), user.ID, weekStart, target)
	switch {
	case errors.Is(err, service.ErrNoTarget):
		a.flashAndBack(w, r, weekStart, "Imposta un target calorico nel profilo per generare il piano.")
		return
	case errors.Is(err, service.ErrNoNutritionistPool):
		a.flashAndBack(w, r, weekStart, "Aggiungi varianti al piano nutrizionista prima di generare.")
		return
	case errors.Is(err, service.ErrWeekNotEmpty):
		a.flashAndBack(w, r, weekStart, "La settimana ha gia' dei pasti: svuotala prima di generare.")
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Piano generato automaticamente per la settimana.")
	a.redirectToWeek(w, r, weekStart)
}
