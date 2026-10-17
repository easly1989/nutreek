package app

import (
	"math"
	"net/http"
	"strconv"

	"github.com/amon2126/nutreek/internal/db"
)

// handleAcceptTDEESuggestion updates the user's target_kcal with the estimated
// TDEE sent in the form (hidden field "estimated_tdee").
func (a *App) handleAcceptTDEESuggestion(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	val := r.PostFormValue("estimated_tdee")
	v, err := strconv.ParseFloat(val, 64)
	if err != nil || v < 500 || v > 8000 {
		http.Error(w, a.t(r, "stats.tdee.invalid"), http.StatusBadRequest)
		return
	}
	rounded := math.Round(v)

	if err := a.Users.UpdateProfile(ctx, user.ID, db.UserProfileInput{TargetKcal: &rounded}); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(ctx, "flash", a.t(r, "stats.flash.target_updated", "Kcal", strconv.FormatInt(int64(rounded), 10)))
	http.Redirect(w, r, "/stats", http.StatusSeeOther)
}
