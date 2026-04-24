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
	User      any
	CSRFToken string
	Flash     string
	Error     string
	Form      authFormData
}

func (a *App) baseData(r *http.Request) authPageData {
	return authPageData{
		User:      userFromContext(r.Context()),
		CSRFToken: nosurf.Token(r),
		Flash:     a.Sessions.PopString(r.Context(), "flash"),
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
	a.Sessions.Put(r.Context(), "flash", "Registrazione completata. Benvenuto!")
	http.Redirect(w, r, "/", http.StatusSeeOther)
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

func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	a.Logger.Error("server error", "err", err, "path", r.URL.Path)
	http.Error(w, "Errore interno", http.StatusInternalServerError)
}
