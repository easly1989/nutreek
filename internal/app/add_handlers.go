package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// The «Add» button of the week (#251): one place to start a meal, a reminder or
// a workout on the day being looked at. The chooser is a sheet on the phone and
// a dialog on a desktop (modal_dialog); each choice opens its own form in it.

// addFormData is the view model of the chooser and of the meal and workout
// forms it leads to.
type addFormData struct {
	authPageData
	Day        int
	Date       time.Time
	WeekStart  string
	Activities []domain.WorkoutActivity

	// The workout form: shortcuts, what to start from, how many workouts the day
	// before has ("As yesterday"), and the workout being edited (0 for a new one).
	Recents    []db.WorkoutRecent
	ActivityID int64
	Minutes    int
	AtTime     string  // "HH:MM" or ""
	WeightKg   float64 // the profile's, for the estimate shown while typing (0 = unknown)
	YesterdayN int
	EditID     int64
}

// addDay reads the week (start) and the day of an add request, which names the
// day by its date (the button of the page: the shown day is known only to the
// browser) or by its index in the week (day, from the chooser's own links). A
// bad day answers 400.
func (a *App) addDay(w http.ResponseWriter, r *http.Request) (weekStart time.Time, day int, ok bool) {
	weekStart = parseWeekStart(r)
	q := r.URL.Query()
	if s := q.Get("date"); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err == nil {
			day = int(d.Sub(weekStart).Hours() / 24)
			if day >= 0 && day <= 6 {
				return weekStart, day, true
			}
		}
	} else if n, err := strconv.Atoi(q.Get("day")); err == nil && n >= 0 && n <= 6 {
		return weekStart, n, true
	}
	http.Error(w, a.t(r, "week.error.missing_day"), http.StatusBadRequest)
	return weekStart, 0, false
}

// renderAddDialog answers an add dialog: the dialog is a fragment, so a plain
// visit goes back to the week.
func (a *App) renderAddDialog(w http.ResponseWriter, r *http.Request, title, inner string, extra func(*addFormData)) {
	weekStart, day, ok := a.addDay(w, r)
	if !ok {
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToDay(w, r, weekStart, day)
		return
	}
	data := addFormData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Day:          day,
		Date:         weekStart.AddDate(0, 0, day),
		WeekStart:    formatDate(weekStart),
	}
	if extra != nil {
		extra(&data)
	}
	mdata := modalDialogData{Title: a.t(r, title), Inner: inner, InnerData: data}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "modal_dialog", mdata); err != nil {
		a.serverError(w, r, err)
	}
}

// handleAddChooser GET /week/add?date=YYYY-MM-DD&start=YYYY-MM-DD.
func (a *App) handleAddChooser(w http.ResponseWriter, r *http.Request) {
	a.renderAddDialog(w, r, "week.add_title", "add_chooser", nil)
}

// handleAddMealForm GET /week/meal?day=N&start=YYYY-MM-DD.
func (a *App) handleAddMealForm(w http.ResponseWriter, r *http.Request) {
	a.renderAddDialog(w, r, "week.add_meal_title", "meal_form", nil)
}

// handleAddWorkoutForm GET /week/workout?day=N&start=YYYY-MM-DD, with the
// activity and minutes of a recent workout (activity, minutes) to start from.
func (a *App) handleAddWorkoutForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	acts, _ := a.WorkoutLogs.ListActivitiesFor(r.Context(), user.ID)
	recents, _ := a.WorkoutLogs.RecentWorkouts(r.Context(), user.ID, 3)
	activityID, _ := strconv.ParseInt(r.URL.Query().Get("activity"), 10, 64)
	minutes, _ := strconv.Atoi(r.URL.Query().Get("minutes"))
	a.renderAddDialog(w, r, "week.add_workout_title", "workout_form", func(d *addFormData) {
		d.Activities, d.Recents, d.ActivityID, d.Minutes = acts, recents, activityID, minutes
		d.WeightKg = weightKgOf(user)
		yesterday := d.Date.AddDate(0, 0, -1)
		if logs, err := a.WorkoutLogs.ListWorkoutLogs(r.Context(), user.ID, yesterday, yesterday); err == nil {
			d.YesterdayN = len(logs)
		}
	})
}
