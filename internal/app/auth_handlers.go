package app

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/db"
)

const sessionKeyUserID = "user_id"

type authFormData struct {
	Email       string
	DisplayName string
}

type authPageData struct {
	User       any
	CSRFToken  string
	Flash      string
	Error      string
	Form       authFormData
	Appearance Appearance
	ActiveNav  string // current URL path, to highlight the active link in the sidebar
	ShowTour   bool   // true when the guided tour must be shown on the dashboard
	Bare       bool   // a page with no sidebar and no bottom bar (onboarding)
	IsOwner    bool   // true when the current user is the instance owner
	// Features maps key→bool: whether the user has access to the feature.
	// With PremiumEnabled=false everything is always true (no gating).
	Features map[string]bool
	// The public demo (#137): the band on every page, and the notice that
	// opens by itself on the first page after entering.
	Demo          bool
	DemoWelcome   bool
	DemoIdleHours int
}

var premiumFeatureKeys = []string{"auto_plan", "tdee_adaptive", "api_access", "coach_mode"}

func (a *App) baseData(r *http.Request) authPageData {
	user := userFromContext(r.Context())
	showTour := false
	if user != nil && r.URL.Path == "/" {
		showTour = user.TourDismissedAt == nil
	}
	features := make(map[string]bool, len(premiumFeatureKeys))
	for _, k := range premiumFeatureKeys {
		features[k] = a.HasFeature(r.Context(), user, k)
	}
	data := authPageData{
		User:       user,
		CSRFToken:  nosurf.Token(r),
		Flash:      a.t(r, a.Sessions.PopString(r.Context(), "flash")),
		Appearance: readAppearance(r),
		ActiveNav:  r.URL.Path,
		ShowTour:   showTour,
		IsOwner:    a.isOwner(user),
		Features:   features,
	}
	if a.Demo != nil {
		data.Demo = true
		data.DemoIdleHours = a.Demo.IdleHours()
		data.DemoWelcome = user != nil && a.Sessions.PopBool(r.Context(), sessionKeyDemoWelcome)
	}
	return data
}

// ---- home ------------------------------------------------------------------

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	page := "home"
	if a.Demo != nil {
		page = "demo_entry"
	}
	if err := a.Render.Render(w, r, http.StatusOK, page, a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- login -----------------------------------------------------------------

func (a *App) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if userFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := a.Render.Render(w, r, http.StatusOK, "login", a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Per-IP rate limit: at most 5 attempts per minute, in memory.
	limiter := a.loginRateLimiter()
	ip := remoteIP(r)
	if !limiter.allow(ip) {
		a.Logger.Warn("login rate limit hit", "ip", ip)
		data := a.baseData(r)
		data.Error = a.t(r, "auth.error.too_many_attempts")
		if err := a.Render.Render(w, r, http.StatusTooManyRequests, "login", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}

	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")

	data := a.baseData(r)
	data.Form.Email = email

	user, err := a.Users.GetByEmail(r.Context(), email)
	if err != nil {
		if !errors.Is(err, db.ErrUserNotFound) {
			a.serverError(w, r, err)
			return
		}
		// same error message as password mismatch to avoid enumeration
		a.renderLoginError(w, r, data, a.t(r, "auth.error.invalid_credentials"))
		return
	}
	if err := auth.VerifyPassword(user.PasswordHash, password); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			a.renderLoginError(w, r, data, a.t(r, "auth.error.invalid_credentials"))
			return
		}
		a.serverError(w, r, err)
		return
	}

	// Successful login: reset the rate limiter so a legitimate user does not
	// stay under the threshold because of earlier failed attempts.
	limiter.reset(ip)

	// Regenerate the session id on privilege change to prevent fixation.
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, user.ID)
	a.Sessions.Put(r.Context(), "flash", "auth.flash.logged_in")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) renderLoginError(w http.ResponseWriter, r *http.Request, data authPageData, msg string) {
	data.Error = msg
	if err := a.Render.Render(w, r, http.StatusUnauthorized, "login", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- register --------------------------------------------------------------

func (a *App) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	if userFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := a.Render.Render(w, r, http.StatusOK, "register", a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	displayName := strings.TrimSpace(r.PostFormValue("display_name"))
	password := r.PostFormValue("password")

	data := a.baseData(r)
	data.Form.Email = email
	data.Form.DisplayName = displayName

	// Validation
	if email == "" || displayName == "" || password == "" {
		a.renderRegisterError(w, r, data, a.t(r, "auth.error.fields_required"))
		return
	}
	if _, err := mail.ParseAddress(email); err != nil {
		a.renderRegisterError(w, r, data, a.t(r, "auth.error.email_invalid"))
		return
	}
	if len(displayName) > 64 {
		a.renderRegisterError(w, r, data, a.t(r, "auth.error.name_too_long"))
		return
	}

	hash, err := auth.HashPassword(password)
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		a.renderRegisterError(w, r, data, a.t(r, "auth.error.password_short"))
		return
	case errors.Is(err, auth.ErrPasswordTooLong):
		a.renderRegisterError(w, r, data, a.t(r, "auth.error.password_long"))
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}

	id, err := a.Users.Create(r.Context(), email, hash, displayName)
	if err != nil {
		if errors.Is(err, db.ErrEmailAlreadyUsed) {
			a.renderRegisterError(w, r, data, a.t(r, "auth.error.email_taken"))
			return
		}
		a.serverError(w, r, err)
		return
	}

	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, id)
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

func (a *App) renderRegisterError(w http.ResponseWriter, r *http.Request, data authPageData, msg string) {
	data.Error = msg
	if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "register", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- logout ----------------------------------------------------------------

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := a.Sessions.Destroy(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, a.signInPath(), http.StatusSeeOther)
}

// ---- helpers ---------------------------------------------------------------

// serverError logs, tries to render the 500 error page with the layout, and
// if that fails too falls back to a plain-text http.Error: an ugly page is
// better than a half-written connection.
func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	a.Logger.Error("server error", "err", err, "path", r.URL.Path)
	if rerr := a.Render.Render(w, r, http.StatusInternalServerError, "error_500", a.baseData(r)); rerr != nil {
		http.Error(w, a.t(r, "layout.error.plain"), http.StatusInternalServerError)
	}
}

// notFound renders the 404 page with the layout. The plain-text fallback
// works as above.
func (a *App) notFound(w http.ResponseWriter, r *http.Request) {
	if rerr := a.Render.Render(w, r, http.StatusNotFound, "error_404", a.baseData(r)); rerr != nil {
		http.NotFound(w, r)
	}
}
