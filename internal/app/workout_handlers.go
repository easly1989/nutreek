package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
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

	var notePtr *string
	if noteStr != "" {
		notePtr = &noteStr
	}

	if _, err := a.WorkoutLogs.CreateWorkoutLog(r.Context(), user.ID, activityID, date, minutes, notePtr); err != nil {
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
