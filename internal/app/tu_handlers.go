package app

import "net/http"

// handleTuPage GET /tu: the You page, behind the avatar. It lists what is not
// one of the four destinations: profile and goals, household, account, the
// preferences, the guide, and sign-out.
func (a *App) handleTuPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	if err := a.Render.Render(w, r, http.StatusOK, "tu", data); err != nil {
		a.serverError(w, r, err)
	}
}
