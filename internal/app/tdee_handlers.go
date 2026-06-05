package app

import (
	"math"
	"net/http"
	"strconv"

	"github.com/amon2126/nutreek/internal/db"
)

// handleAcceptTDEESuggestion aggiorna target_kcal dell'utente con il TDEE
// stimato inviato nel form (campo hidden "estimated_tdee").
func (a *App) handleAcceptTDEESuggestion(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	val := r.PostFormValue("estimated_tdee")
	v, err := strconv.ParseFloat(val, 64)
	if err != nil || v < 500 || v > 8000 {
		http.Error(w, "Valore TDEE non valido", http.StatusBadRequest)
		return
	}
	rounded := math.Round(v)

	if err := a.Users.UpdateProfile(ctx, user.ID, db.UserProfileInput{TargetKcal: &rounded}); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(ctx, "flash", "Target calorico aggiornato a "+strconv.FormatInt(int64(rounded), 10)+" kcal/giorno.")
	http.Redirect(w, r, "/stats", http.StatusSeeOther)
}
