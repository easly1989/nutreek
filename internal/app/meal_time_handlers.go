package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
)

// Meal times and reminders (#236): a time per meal, the reminders (meals with no
// food), and the time each meal name gets by default.

// reminderFormData is the view model of the «New reminder» dialog.
type reminderFormData struct {
	authPageData
	Day       int
	Date      time.Time
	WeekStart string
	Error     string
}

// handleReminderForm GET /week/reminder?day=N&start=YYYY-MM-DD: the dialog.
func (a *App) handleReminderForm(w http.ResponseWriter, r *http.Request) {
	weekStart := parseWeekStart(r)
	day, err := strconv.Atoi(r.URL.Query().Get("day"))
	if err != nil || day < 0 || day > 6 {
		http.Error(w, a.t(r, "week.error.missing_day"), http.StatusBadRequest)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToDay(w, r, weekStart, day)
		return
	}
	date := weekStart.AddDate(0, 0, day)
	data := reminderFormData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Day:          day,
		Date:         date,
		WeekStart:    formatDate(weekStart),
	}
	mdata := modalDialogData{Title: a.t(r, "meal_times.reminder_title"), Inner: "reminder_form", InnerData: data}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "modal_dialog", mdata); err != nil {
		a.serverError(w, r, err)
	}
}

// handleAddReminder POST /reminders?start=YYYY-MM-DD, fields day_of_week, name,
// note, start_time, end_time.
func (a *App) handleAddReminder(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	day, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("day_of_week")))
	if err != nil || day < 0 || day > 6 {
		a.flashAndBack(w, r, weekStart, "week.flash.invalid_day")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		a.flashAndBack(w, r, weekStart, "week.flash.meal_name_required")
		return
	}
	if !withinLen(name, MaxNameLen) {
		a.flashAndBack(w, r, weekStart, "week.flash.name_too_long")
		return
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	_, err = a.WeeklyPlans.AddReminder(r.Context(), plan.ID, user.ID, day, name,
		r.PostFormValue("note"), strings.TrimSpace(r.PostFormValue("start_time")), strings.TrimSpace(r.PostFormValue("end_time")))
	if errors.Is(err, db.ErrInvalidTime) {
		a.flashAndBack(w, r, weekStart, "meal_times.flash.invalid_time")
		return
	}
	if errors.Is(err, db.ErrTooLong) {
		a.flashAndBack(w, r, weekStart, "week.flash.name_too_long")
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleMealTime POST /meals/{mealID}/time?start=YYYY-MM-DD, fields start_time and
// end_time: changes a meal's time; the day comes back sorted by it.
func (a *App) handleMealTime(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	day, err := a.WeeklyPlans.SetMealTime(r.Context(), mealID, user.ID,
		strings.TrimSpace(r.PostFormValue("start_time")), strings.TrimSpace(r.PostFormValue("end_time")))
	switch {
	case errors.Is(err, db.ErrMealNotFound):
		http.Error(w, a.t(r, "week.error.meal_not_found"), http.StatusNotFound)
		return
	case errors.Is(err, db.ErrInvalidTime):
		a.flashAndBack(w, r, weekStart, "meal_times.flash.invalid_time")
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleSettingsMealTime POST /settings/meal-times, fields name, start_time and
// end_time: the time a meal name gets when it is created (also changes one).
func (a *App) handleSettingsMealTime(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	err := a.WeeklyPlans.SetMealTimeDefault(r.Context(), user.ID, r.PostFormValue("name"),
		strings.TrimSpace(r.PostFormValue("start_time")), strings.TrimSpace(r.PostFormValue("end_time")))
	if errors.Is(err, db.ErrInvalidTime) {
		a.Sessions.Put(r.Context(), "flash", "meal_times.flash.invalid_time")
	} else if errors.Is(err, db.ErrTooLong) {
		a.Sessions.Put(r.Context(), "flash", "week.flash.name_too_long")
	} else if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings#orari", http.StatusSeeOther)
}

// handleSettingsMealTimeDelete POST /settings/meal-times/delete, field name.
func (a *App) handleSettingsMealTimeDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := a.WeeklyPlans.DeleteMealTimeDefault(r.Context(), user.ID, r.PostFormValue("name")); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings#orari", http.StatusSeeOther)
}

// handleSettingsShowMealTimes POST /settings/show-meal-times: a hidden 0 and, when
// the switch is on, a 1 in show_meal_times.
func (a *App) handleSettingsShowMealTimes(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_ids"), http.StatusBadRequest)
		return
	}
	on := false
	for _, v := range r.PostForm["show_meal_times"] {
		on = on || v == "1"
	}
	if err := a.Users.SetShowMealTimes(r.Context(), user.ID, on); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings#orari", http.StatusSeeOther)
}
