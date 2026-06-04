package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/justinas/nosurf"
)

// maxWaterLogML limita un singolo quick-log a 3 litri: oltre e' quasi sempre
// un errore di battitura.
const maxWaterLogML = 3000

// handleWaterLog POST /water — registra un'assunzione d'acqua per la data e
// risponde con il widget aggiornato (HTMX outerHTML su #water-widget-{date}).
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

// handleWaterDelete POST /water/delete-last — annulla l'ultimo quick-log del
// giorno. Idempotente: se non c'e' nulla, ri-renderizza comunque il widget.
func (a *App) handleWaterDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	date, ok := a.parseWaterDate(w, r)
	if !ok {
		return
	}
	_ = a.WaterLogs.DeleteLast(r.Context(), user.ID, date)
	a.renderWaterWidget(w, r, user.ID, date)
}

// parseWaterDate legge il campo `date` (YYYY-MM-DD) dal form; default a oggi.
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

// renderWaterWidget scrive il partial water_widget aggiornato per la data.
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
	if err := a.Render.RenderPartial(w, http.StatusOK, "dashboard", "water_widget", data); err != nil {
		a.serverError(w, r, err)
	}
}
