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
		a.flashAndBack(w, r, weekStart, "week.flash.no_target")
		return
	case errors.Is(err, service.ErrNoNutritionistPool):
		a.flashAndBack(w, r, weekStart, "week.flash.no_pool")
		return
	case errors.Is(err, service.ErrWeekNotEmpty):
		a.flashAndBack(w, r, weekStart, "week.flash.has_meals_generate")
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "week.flash.generated")
	a.redirectToWeek(w, r, weekStart)
}
