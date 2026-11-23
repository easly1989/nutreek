package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// handleWorkoutLog POST /allenamento — adds a workout to the day.
// The form comes from the day card; after creation it updates the card via
// an HTMX swap or a redirect to the week.
func (a *App) handleWorkoutLog(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())

	activityIDStr := strings.TrimSpace(r.PostFormValue("activity_id"))
	dateStr := strings.TrimSpace(r.PostFormValue("logged_at"))
	minutesStr := strings.TrimSpace(r.PostFormValue("duration_minutes"))
	noteStr := strings.TrimSpace(r.PostFormValue("note"))
	weekStart := parseWeekStart(r)

	activityID, err := strconv.ParseInt(activityIDStr, 10, 64)
	if err != nil || activityID <= 0 {
		a.Sessions.Put(r.Context(), "flash", "week.flash.select_activity")
		a.redirectToWeek(w, r, weekStart)
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_date")
		a.redirectToWeek(w, r, weekStart)
		return
	}

	minutes, err := strconv.Atoi(minutesStr)
	if err != nil || minutes <= 0 || minutes > 720 {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_duration")
		a.redirectToWeek(w, r, weekStart)
		return
	}

	atTime, ok := parseAtTime(r.PostFormValue("at_time"))
	if !ok {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_time")
		a.redirectToWeek(w, r, weekStart)
		return
	}

	var notePtr *string
	if noteStr != "" {
		notePtr = &noteStr
	}

	if _, err := a.WorkoutLogs.CreateWorkoutLog(r.Context(), user.ID, activityID, date, minutes, atTime, notePtr); err != nil {
		if errors.Is(err, db.ErrActivityNotFound) {
			a.Sessions.Put(r.Context(), "flash", "week.flash.select_activity")
			a.redirectToWeek(w, r, weekStart)
			return
		}
		a.serverError(w, r, err)
		return
	}

	// Update the card of the day the workout was logged on.
	dayIndex := int(date.Sub(db.MondayOf(date)).Hours() / 24)
	a.respondMutationOrRedirect(w, r, db.MondayOf(date), dayIndex)
}

// handleWorkoutLogDelete POST /allenamento/{id}/delete — deletes a workout.
func (a *App) handleWorkoutLogDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Read logged_at before deleting to know which card to update.
	var dateStr string
	err = a.Store.DB.QueryRowContext(r.Context(),
		`SELECT logged_at FROM workout_logs WHERE id = ? AND user_id = ?`, id, user.ID,
	).Scan(&dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	date, _ := time.Parse("2006-01-02", dateStr)
	weekStart := db.MondayOf(date)
	dayIndex := int(date.Sub(weekStart).Hours() / 24)

	if err := a.WorkoutLogs.DeleteWorkoutLog(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, db.ErrWorkoutLogNotFound) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}

	a.respondMutationOrRedirect(w, r, weekStart, dayIndex)
}

// handleWorkoutEditForm GET /allenamento/{id}/edit?start=YYYY-MM-DD: the dialog
// that changes the activity and the minutes of a workout.
func (a *App) handleWorkoutEditForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	log, err := a.WorkoutLogs.GetWorkoutLog(r.Context(), id, user.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	weekStart := db.MondayOf(log.LoggedAt)
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToDay(w, r, weekStart, int(log.LoggedAt.Sub(weekStart).Hours()/24))
		return
	}
	acts, _ := a.WorkoutLogs.ListActivitiesFor(r.Context(), user.ID)
	data := addFormData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Date:         log.LoggedAt,
		WeekStart:    formatDate(weekStart),
		Activities:   acts,
		ActivityID:   log.ActivityID,
		Minutes:      log.DurationMinutes,
		AtTime:       log.AtTime,
		WeightKg:     weightKgOf(user),
		EditID:       log.ID,
	}
	mdata := modalDialogData{Title: a.t(r, "week.edit_workout_title"), Inner: "workout_form", InnerData: data}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "modal_dialog", mdata); err != nil {
		a.serverError(w, r, err)
	}
}

// handleWorkoutLogUpdate POST /allenamento/{id} — changes the activity and the
// minutes of a workout; the kcal are worked out again.
func (a *App) handleWorkoutLogUpdate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	log, err := a.WorkoutLogs.GetWorkoutLog(r.Context(), id, user.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	weekStart := db.MondayOf(log.LoggedAt)
	dayIndex := int(log.LoggedAt.Sub(weekStart).Hours() / 24)

	activityID, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("activity_id")), 10, 64)
	if err != nil || activityID <= 0 {
		a.Sessions.Put(r.Context(), "flash", "week.flash.select_activity")
		a.redirectToWeek(w, r, weekStart)
		return
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("duration_minutes")))
	if err != nil || minutes <= 0 || minutes > 720 {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_duration")
		a.redirectToWeek(w, r, weekStart)
		return
	}
	atTime, ok := parseAtTime(r.PostFormValue("at_time"))
	if !ok {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_time")
		a.redirectToWeek(w, r, weekStart)
		return
	}
	if err := a.WorkoutLogs.UpdateWorkoutLog(r.Context(), id, user.ID, activityID, minutes, atTime); err != nil {
		if errors.Is(err, db.ErrActivityNotFound) {
			a.Sessions.Put(r.Context(), "flash", "week.flash.select_activity")
			a.redirectToWeek(w, r, weekStart)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, dayIndex)
}

// handleWorkoutCopy POST /allenamento/copy — repeats the workouts of the day
// before the date logged_at on that date («As yesterday»).
func (a *App) handleWorkoutCopy(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	date, err := time.Parse("2006-01-02", strings.TrimSpace(r.PostFormValue("logged_at")))
	if err != nil {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_date")
		a.redirectToWeek(w, r, parseWeekStart(r))
		return
	}
	if _, err := a.WorkoutLogs.CopyWorkoutLogs(r.Context(), user.ID, date.AddDate(0, 0, -1), date); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.respondMutationOrRedirect(w, r, db.MondayOf(date), int(date.Sub(db.MondayOf(date)).Hours()/24))
}

// parseAtTime reads the optional time of a workout: "" is fine, otherwise it has to
// be HH:MM.
func parseAtTime(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return "", false
	}
	return t.Format("15:04"), true
}

// weightKgOf is the weight in the profile, 0 when none was given.
func weightKgOf(u *domain.User) float64 {
	if u == nil || u.WeightKg == nil {
		return 0
	}
	return *u.WeightKg
}

// handleWorkoutActivityCreate POST /allenamento/attivita — adds an activity of the
// user's own from the workout dialog («New activity…») and gives the dialog back
// with it selected, keeping the minutes and the time already typed.
func (a *App) handleWorkoutActivityCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	name := strings.TrimSpace(r.PostFormValue("new_name"))
	kcal, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.PostFormValue("new_kcal")), ",", "."), 64)
	if name == "" || !withinLen(name, 60) || err != nil || kcal < 1 || kcal > 2000 {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_activity")
		a.redirectToWeek(w, r, weekStart)
		return
	}
	id, err := a.WorkoutLogs.CreateActivity(r.Context(), user.ID, name, kcal)
	if errors.Is(err, db.ErrActivityExists) {
		a.Sessions.Put(r.Context(), "flash", "week.flash.activity_exists")
		a.redirectToWeek(w, r, weekStart)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(r.PostFormValue("logged_at")))
	if err != nil || r.Header.Get("HX-Request") != "true" {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	acts, _ := a.WorkoutLogs.ListActivitiesFor(r.Context(), user.ID)
	minutes, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("duration_minutes")))
	atTime, _ := parseAtTime(r.PostFormValue("at_time"))
	data := addFormData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Day:          int(date.Sub(weekStart).Hours() / 24),
		Date:         date,
		WeekStart:    formatDate(weekStart),
		Activities:   acts,
		ActivityID:   id,
		Minutes:      minutes,
		AtTime:       atTime,
		WeightKg:     weightKgOf(user),
	}
	mdata := modalDialogData{Title: a.t(r, "week.add_workout_title"), Inner: "workout_form", InnerData: data}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "modal_dialog", mdata); err != nil {
		a.serverError(w, r, err)
	}
}

// handleSettingsWorkoutsInGoal POST /settings/workouts-in-goal: a hidden 0 and, when
// the switch is on, a 1 in workouts_in_goal.
func (a *App) handleSettingsWorkoutsInGoal(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_ids"), http.StatusBadRequest)
		return
	}
	on := false
	for _, v := range r.PostForm["workouts_in_goal"] {
		on = on || v == "1"
	}
	if err := a.Users.SetWorkoutsInGoal(r.Context(), user.ID, on); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings#allenamenti", http.StatusSeeOther)
}
