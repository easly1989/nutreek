package app

import (
	"net/http"
	"time"

	"github.com/amon2126/nutreek/internal/updates"
)

// updateView is the update check as the owner sees it (#167), with its times
// already said in the page's language.
type updateView struct {
	updates.Status
	PublishedAgo string // "2 days ago"; empty when unknown
	CheckedAgo   string
}

func (a *App) updateView(r *http.Request, st updates.Status) *updateView {
	v := &updateView{Status: st}
	if !st.Published.IsZero() {
		v.PublishedAgo = a.ago(r, st.Published)
	}
	if !st.CheckedAt.IsZero() {
		v.CheckedAgo = a.ago(r, st.CheckedAt)
	}
	return v
}

// ago says how long ago t was: minutes, hours or days.
func (a *App) ago(r *http.Request, t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return a.t(r, "updates.ago.minutes", "Count", max(1, int(d/time.Minute)))
	case d < 24*time.Hour:
		return a.t(r, "updates.ago.hours", "Count", int(d/time.Hour))
	}
	return a.t(r, "updates.ago.days", "Count", int(d/(24*time.Hour)))
}

// ownerUpdate is the newer release to tell the owner about on every page: nil
// for anyone else, when the check is off or nothing newer is out. It never
// asks GitHub: it reads the last answer.
func (a *App) ownerUpdate(r *http.Request, isOwner bool) *updateView {
	if !isOwner || a.Updates == nil {
		return nil
	}
	st, err := a.Updates.Status(r.Context())
	if err != nil || !st.Available {
		return nil
	}
	return a.updateView(r, st)
}

type updateHowToData struct {
	authPageData
	Version string // the release to update to; empty when none is known
}

// handleUpdateHowTo GET /admin/update — how to update, as a page: the dialog's
// content for a browser without JS (#167).
func (a *App) handleUpdateHowTo(w http.ResponseWriter, r *http.Request) {
	data := updateHowToData{authPageData: a.baseData(r)}
	if st, err := a.Updates.Status(r.Context()); err == nil {
		data.Version = st.Latest
	}
	if err := a.Render.Render(w, r, http.StatusOK, "update_howto", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleUpdateCheck POST /admin/panel/updates/check — «Check now».
func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if _, err := a.Updates.Check(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/panel#aggiornamenti", http.StatusSeeOther)
}

// handleUpdateToggle POST /admin/panel/updates/toggle — «Check every day»;
// turning it on checks at once.
func (a *App) handleUpdateToggle(w http.ResponseWriter, r *http.Request) {
	st, err := a.Updates.Status(r.Context())
	if err == nil {
		_, err = a.Updates.SetEnabled(r.Context(), !st.Enabled)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/panel#aggiornamenti", http.StatusSeeOther)
}
