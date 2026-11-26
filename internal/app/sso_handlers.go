package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/sso"
)

const (
	sessionKeySSOState    = "sso_state"
	sessionKeySSONonce    = "sso_nonce"
	sessionKeySSOVerifier = "sso_verifier"
)

// ssoName is the label of the sign-in button; empty when no provider is set.
func (a *App) ssoName() string {
	if a.SSO == nil {
		return ""
	}
	return a.SSO.Name()
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// handleSSOStart GET /auth/oidc/start: sends the browser to the provider. State,
// nonce and the PKCE verifier wait in the session until the callback.
func (a *App) handleSSOStart(w http.ResponseWriter, r *http.Request) {
	if userFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	state, err1 := randomHex(16)
	nonce, err2 := randomHex(16)
	if err := errors.Join(err1, err2); err != nil {
		a.serverError(w, r, err)
		return
	}
	verifier := oauth2.GenerateVerifier()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	target, err := a.SSO.AuthURL(ctx, state, nonce, verifier)
	if err != nil {
		a.Logger.Warn("sso start failed", "err", err)
		a.renderSSOError(w, r, "auth.error.sso_failed")
		return
	}
	a.Sessions.Put(r.Context(), sessionKeySSOState, state)
	a.Sessions.Put(r.Context(), sessionKeySSONonce, nonce)
	a.Sessions.Put(r.Context(), sessionKeySSOVerifier, verifier)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// handleSSOCallback GET /auth/oidc/callback: the provider sends the person back
// with a code. An identity is matched to an account by its verified email; a new
// email gets an account only with NUTREEK_OIDC_AUTO_CREATE.
func (a *App) handleSSOCallback(w http.ResponseWriter, r *http.Request) {
	state := a.Sessions.PopString(r.Context(), sessionKeySSOState)
	nonce := a.Sessions.PopString(r.Context(), sessionKeySSONonce)
	verifier := a.Sessions.PopString(r.Context(), sessionKeySSOVerifier)
	q := r.URL.Query()
	got := q.Get("state")
	if state == "" || nonce == "" || verifier == "" || subtle.ConstantTimeCompare([]byte(state), []byte(got)) != 1 {
		a.renderSSOError(w, r, "auth.error.sso_failed")
		return
	}
	if e := q.Get("error"); e != "" || q.Get("code") == "" {
		a.Logger.Warn("sso callback refused", "error", q.Get("error"))
		a.renderSSOError(w, r, "auth.error.sso_failed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	ident, err := a.SSO.Exchange(ctx, q.Get("code"), nonce, verifier)
	if err != nil {
		a.Logger.Warn("sso exchange failed", "err", err)
		key := "auth.error.sso_failed"
		if errors.Is(err, sso.ErrNoEmail) {
			key = "auth.error.sso_no_email"
		}
		a.renderSSOError(w, r, key)
		return
	}

	user, err := a.Users.GetByEmail(r.Context(), ident.Email)
	newAccount := false
	var userID int64
	switch {
	case err == nil:
		userID = user.ID
	case errors.Is(err, db.ErrUserNotFound):
		if !a.Cfg.OIDCAutoCreate {
			a.renderSSOError(w, r, "auth.error.sso_no_account")
			return
		}
		// No password: the hash is of a random secret nobody sees.
		secret, err := randomHex(32)
		if err == nil {
			var hash string
			if hash, err = auth.HashPassword(secret); err == nil {
				userID, err = a.Users.Create(r.Context(), ident.Email, hash, ident.Name)
			}
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		newAccount = true
	default:
		a.serverError(w, r, err)
		return
	}

	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, userID)
	if newAccount {
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "auth.flash.logged_in")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) renderSSOError(w http.ResponseWriter, r *http.Request, key string) {
	a.renderLoginError(w, r, a.baseData(r), a.t(r, key))
}
