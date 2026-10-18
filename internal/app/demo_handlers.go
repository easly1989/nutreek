package app

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
)

// sessionKeyDemoWelcome opens the demo's notice on the first page after entering.
const sessionKeyDemoWelcome = "demo_welcome"

// handleDemoSession POST /demo/session — creates a demo group in the
// visitor's language, the visitor playing the role posted, and signs them in
// (#137, #186). Mounted only in the demo build.
func (a *App) handleDemoSession(w http.ResponseWriter, r *http.Request) {
	if max := a.DemoLimits.MaxUsers; max > 0 {
		// Every user of a demo database is a demo one (demo.Prepare).
		// shortcut: a coach or client group adds 4 users, so the cap may be passed by 3; count per group if that matters.
		n, err := a.Users.Count(r.Context())
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if n >= max {
			data := a.baseData(r)
			data.Error = a.t(r, "demo.entry.full")
			if err := a.Render.Render(w, r, http.StatusServiceUnavailable, "demo_entry", data); err != nil {
				a.serverError(w, r, err)
			}
			return
		}
	}
	role := r.PostFormValue("role")
	id, err := a.Demo.Start(r.Context(), i18n.FromContext(r.Context()), role)
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
	to := "/"
	if role == "coach" {
		to = "/coach"
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// handleDemoAs POST /demo/as — signs the visitor in as user_id, one of their
// own demo group, with no password (#187); anyone else is 404. Mounted only in
// the demo build.
func (a *App) handleDemoAs(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PostFormValue("user_id"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	group, err := a.Demo.Group(r.Context(), viewerID(r.Context()))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	i := slices.IndexFunc(group, func(m domain.DemoMember) bool { return m.ID == id })
	if i < 0 {
		a.notFound(w, r)
		return
	}
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, id)
	to := "/"
	if group[i].Coach {
		to = "/coach"
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// demoCandidate returns the client the coach picked by client_id, when it is
// one they may add: the demo has no emails, only the coach's group (#186).
func (a *App) demoCandidate(r *http.Request, coachID int64) (int64, bool, error) {
	id, err := strconv.ParseInt(r.PostFormValue("client_id"), 10, 64)
	if err != nil {
		return 0, false, nil
	}
	candidates, err := a.Demo.Candidates(r.Context(), coachID)
	if err != nil {
		return 0, false, err
	}
	ok := slices.ContainsFunc(candidates, func(u domain.User) bool { return u.ID == id })
	return id, ok, nil
}

// limitBody answers 413 to a request body over max bytes (#185).
func limitBody(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > max {
				http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
				return
			}
			// A body without a length fails to parse once past max.
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// demoCap refuses a create once the user has max of them (#185), back to the
// list at back with the flash key saying why. count reads how many they have.
// With max 0 it lets everything through.
func (a *App) demoCap(max int, count func(context.Context, int64) (int, error), flash, back string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if max <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n, err := count(r.Context(), viewerID(r.Context()))
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			if n < max {
				next.ServeHTTP(w, r)
				return
			}
			a.Sessions.Put(r.Context(), "flash", a.t(r, flash, "Count", max))
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", back)
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.Redirect(w, r, back, http.StatusSeeOther)
		})
	}
}
