package app

import (
	"net/http"

	"github.com/amon2126/nutreek/internal/i18n"
)

// sessionKeyDemoWelcome opens the demo's notice on the first page after entering.
const sessionKeyDemoWelcome = "demo_welcome"

// handleDemoSession POST /demo/session — creates a demo account in the
// visitor's language and signs them in (#137). Mounted only in the demo build.
func (a *App) handleDemoSession(w http.ResponseWriter, r *http.Request) {
	id, err := a.Demo.Start(r.Context(), i18n.FromContext(r.Context()))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, id)
	a.Sessions.Put(r.Context(), sessionKeyDemoWelcome, true)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
