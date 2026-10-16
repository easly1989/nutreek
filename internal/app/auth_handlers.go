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
	ActiveNav  string // URL path corrente, per evidenziare il link attivo nella sidebar
	ShowTour   bool   // true se il tour guidato deve essere mostrato sulla dashboard
	Bare       bool   // pagina senza barra laterale ne' barra in basso (onboarding)
	IsOwner    bool   // true se l'utente corrente è l'owner dell'istanza
	// Features mappa key→bool: indica se l'utente ha accesso alla feature.
	// Quando PremiumEnabled=false è sempre tutto true (nessun gating).
	Features map[string]bool
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
	return authPageData{
		User:       user,
		CSRFToken:  nosurf.Token(r),
		Flash:      a.Sessions.PopString(r.Context(), "flash"),
		Appearance: readAppearance(r),
		ActiveNav:  r.URL.Path,
		ShowTour:   showTour,
		IsOwner:    a.isOwner(user),
		Features:   features,
	}
}

// ---- home ------------------------------------------------------------------

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if err := a.Render.Render(w, http.StatusOK, "home", a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- login -----------------------------------------------------------------

func (a *App) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if userFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := a.Render.Render(w, http.StatusOK, "login", a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Rate limit per IP: max 5 tentativi per minuto, in-memory.
	limiter := a.loginRateLimiter()
	ip := remoteIP(r)
	if !limiter.allow(ip) {
		a.Logger.Warn("login rate limit hit", "ip", ip)
		data := a.baseData(r)
		data.Error = "Troppi tentativi. Riprova fra un minuto."
		if err := a.Render.Render(w, http.StatusTooManyRequests, "login", data); err != nil {
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
		a.renderLoginError(w, r, data, "Email o password non validi.")
		return
	}
	if err := auth.VerifyPassword(user.PasswordHash, password); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			a.renderLoginError(w, r, data, "Email o password non validi.")
			return
		}
		a.serverError(w, r, err)
		return
	}

	// Login riuscito: reset del rate limiter cosi' un utente legittimo
	// non resta sotto threshold per i tentativi falliti precedenti.
	limiter.reset(ip)

	// Regenerate the session id on privilege change to prevent fixation.
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, user.ID)
	a.Sessions.Put(r.Context(), "flash", "Accesso effettuato.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) renderLoginError(w http.ResponseWriter, r *http.Request, data authPageData, msg string) {
	data.Error = msg
	if err := a.Render.Render(w, http.StatusUnauthorized, "login", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- register --------------------------------------------------------------

func (a *App) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	if userFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := a.Render.Render(w, http.StatusOK, "register", a.baseData(r)); err != nil {
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
		a.renderRegisterError(w, r, data, "Compila tutti i campi.")
		return
	}
	if _, err := mail.ParseAddress(email); err != nil {
		a.renderRegisterError(w, r, data, "Indirizzo email non valido.")
		return
	}
	if len(displayName) > 64 {
		a.renderRegisterError(w, r, data, "Il nome puo' avere al massimo 64 caratteri.")
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		a.renderRegisterError(w, r, data, err.Error())
		return
	}

	id, err := a.Users.Create(r.Context(), email, hash, displayName)
	if err != nil {
		if errors.Is(err, db.ErrEmailAlreadyUsed) {
			a.renderRegisterError(w, r, data, "Questa email e' gia' registrata.")
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
	if err := a.Render.Render(w, http.StatusUnprocessableEntity, "register", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- logout ----------------------------------------------------------------

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := a.Sessions.Destroy(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ---- helpers ---------------------------------------------------------------

// serverError logga, prova a renderizzare la pagina di errore 500 con
// layout, e in caso di doppio fallimento fa il fallback a http.Error
// testuale: meglio una pagina brutta che una connessione mezza scritta.
func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	a.Logger.Error("server error", "err", err, "path", r.URL.Path)
	if rerr := a.Render.Render(w, http.StatusInternalServerError, "error_500", a.baseData(r)); rerr != nil {
		http.Error(w, "Errore interno", http.StatusInternalServerError)
	}
}

// notFound renderizza la pagina 404 con layout. Il fallback testuale
// vale quanto sopra.
func (a *App) notFound(w http.ResponseWriter, r *http.Request) {
	if rerr := a.Render.Render(w, http.StatusNotFound, "error_404", a.baseData(r)); rerr != nil {
		http.NotFound(w, r)
	}
}
