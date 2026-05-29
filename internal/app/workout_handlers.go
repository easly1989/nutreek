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

type workoutPageData struct {
	authPageData
	Activities []domain.WorkoutActivity
	Logs       []domain.WorkoutLog
	TodayStr   string
	TodayKcal  float64
}

func (a *App) handleWorkoutPage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	activities, err := a.WorkoutLogs.ListActivities(ctx)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	to := time.Now()
	from := to.AddDate(0, 0, -30)
	logs, err := a.WorkoutLogs.ListWorkoutLogs(ctx, user.ID, from, to)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	todayKcal, _ := a.WorkoutLogs.WorkoutKcalForDay(ctx, user.ID, to)

	data := workoutPageData{
		authPageData: a.baseData(r),
		Activities:   activities,
		Logs:         logs,
		TodayStr:     to.Format("2006-01-02"),
		TodayKcal:    todayKcal,
	}
	if err := a.Render.Render(w, http.StatusOK, "workout", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleWorkoutLog(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())

	activityIDStr := strings.TrimSpace(r.PostFormValue("activity_id"))
	dateStr := strings.TrimSpace(r.PostFormValue("logged_at"))
	minutesStr := strings.TrimSpace(r.PostFormValue("duration_minutes"))
	noteStr := strings.TrimSpace(r.PostFormValue("note"))

	renderErr := func(msg string) {
		activities, _ := a.WorkoutLogs.ListActivities(r.Context())
		to := time.Now()
		from := to.AddDate(0, 0, -30)
		logs, _ := a.WorkoutLogs.ListWorkoutLogs(r.Context(), user.ID, from, to)
		data := workoutPageData{
			authPageData: a.baseData(r),
			Activities:   activities,
			Logs:         logs,
			TodayStr:     to.Format("2006-01-02"),
		}
		data.Error = msg
		if err := a.Render.Render(w, http.StatusUnprocessableEntity, "workout", data); err != nil {
			a.serverError(w, r, err)
		}
	}

	activityID, err := strconv.ParseInt(activityIDStr, 10, 64)
	if err != nil || activityID <= 0 {
		renderErr("Seleziona un'attività.")
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		renderErr("Data non valida.")
		return
	}

	minutes, err := strconv.Atoi(minutesStr)
	if err != nil || minutes <= 0 || minutes > 720 {
		renderErr("Durata non valida (1–720 minuti).")
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

	a.Sessions.Put(r.Context(), "flash", "Allenamento registrato.")
	http.Redirect(w, r, "/allenamento", http.StatusSeeOther)
}

func (a *App) handleWorkoutLogDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := a.WorkoutLogs.DeleteWorkoutLog(r.Context(), id, user.ID); err != nil {
		if errors.Is(err, db.ErrWorkoutLogNotFound) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}

	a.Sessions.Put(r.Context(), "flash", "Allenamento eliminato.")
	http.Redirect(w, r, "/allenamento", http.StatusSeeOther)
}
