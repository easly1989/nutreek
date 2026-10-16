package app

import "net/http"

// handleTourDismiss POST /tour/dismiss — marks the tour as dismissed.
// Called via fetch() by tour.js after the user completes or closes the tour.
func (a *App) handleTourDismiss(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	_ = a.Users.DismissTour(r.Context(), user.ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleTourRestart POST /tour/restart — clears the dismissed flag and goes back to the dashboard.
func (a *App) handleTourRestart(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	_ = a.Users.ResetTour(r.Context(), user.ID)
	a.Sessions.Put(r.Context(), "flash", "Tour riavviato. Torna alla dashboard per iniziare.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
