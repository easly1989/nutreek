package app

import "net/http"

// handleTourDismiss POST /tour/dismiss — segna il tour come dismesso.
// Chiamato via fetch() da tour.js dopo che l'utente completa o chiude il tour.
func (a *App) handleTourDismiss(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	_ = a.Users.DismissTour(r.Context(), user.ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleTourRestart POST /tour/restart — azzera il flag dismissed e torna alla dashboard.
func (a *App) handleTourRestart(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	_ = a.Users.ResetTour(r.Context(), user.ID)
	a.Sessions.Put(r.Context(), "flash", "Tour riavviato. Torna alla dashboard per iniziare.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
