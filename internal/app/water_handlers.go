package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/justinas/nosurf"
)

// maxWaterLogML limits a single quick log to 3 liters: beyond that it is
// nearly always a typo.
const maxWaterLogML = 3000

// handleWaterLog POST /water — logs a water intake for the date and answers
// with the updated widget (HTMX outerHTML on #water-widget-{date}).
func (a *App) handleWaterLog(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	date, ok := a.parseWaterDate(w, r)
	if !ok {
		return
	}
	amount, err := strconv.Atoi(r.PostFormValue("amount_ml"))
	if err != nil || amount <= 0 || amount > maxWaterLogML {
		http.Error(w, "quantita' non valida", http.StatusBadRequest)
		return
	}
	if err := a.WaterLogs.Log(r.Context(), user.ID, date, amount); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.renderWaterWidget(w, r, user.ID, date)
}

// handleWaterDelete POST /water/delete-last — undoes the day's last quick
// log. Idempotent: when there is nothing, it re-renders the widget anyway.
func (a *App) handleWaterDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	date, ok := a.parseWaterDate(w, r)
	if !ok {
		return
	}
	_ = a.WaterLogs.DeleteLast(r.Context(), user.ID, date)
	a.renderWaterWidget(w, r, user.ID, date)
}

// parseWaterDate reads the `date` field (YYYY-MM-DD) from the form; defaults to today.
func (a *App) parseWaterDate(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	raw := r.PostFormValue("date")
	if raw == "" {
		return todayDate(), true
	}
	date, err := time.Parse("2006-01-02", raw)
	if err != nil {
		http.Error(w, "data non valida", http.StatusBadRequest)
		return time.Time{}, false
	}
	return date, true
}

// renderWaterWidget writes the updated water_widget partial for the date.
func (a *App) renderWaterWidget(w http.ResponseWriter, r *http.Request, userID int64, date time.Time) {
	user := userFromContext(r.Context())
	ml, err := a.WaterLogs.TotalForDate(r.Context(), userID, date)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := waterWidgetData{
		DateStr:       formatDate(date),
		WaterML:       ml,
		WaterTargetML: waterTargetFor(user),
		CSRFToken:     nosurf.Token(r),
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "water_widget", data); err != nil {
		a.serverError(w, r, err)
	}
}
