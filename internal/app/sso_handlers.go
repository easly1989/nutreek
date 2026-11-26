package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"time"
	"unicode/utf8"

	"golang.org/x/oauth2"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/sso"
)

const (
	sessionKeySSOState    = "sso_state"
	sessionKeySSONonce    = "sso_nonce"
	sessionKeySSOVerifier = "sso_verifier"

	// The account waiting for its password before an identity is linked to it.
	sessionKeySSOLinkUser    = "sso_link_user"
	sessionKeySSOLinkIssuer  = "sso_link_issuer"
	sessionKeySSOLinkSubject = "sso_link_subject"
	sessionKeySSOLinkEmail   = "sso_link_email"
	sessionKeySSOLinkAt      = "sso_link_at"

	ssoLinkTTL = 10 * time.Minute
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
	a.clearSSOLink(r)
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
	a.clearSSOLink(r) // a link left open by an earlier trip is not this one's
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

	// A person already linked is found by who they are at the provider, never by email.
	userID, err := a.Identities.UserID(r.Context(), ident.Issuer, ident.Subject)
	switch {
	case err == nil:
		a.finishSSOSignIn(w, r, userID, false)
		return
	case !errors.Is(err, db.ErrIdentityNotFound):
		a.serverError(w, r, err)
		return
	}

	user, err := a.Users.GetByEmail(r.Context(), ident.Email)
	switch {
	case err == nil:
		// An account with this email exists. Registration does not verify emails, so
		// anyone may have made it: the person proves it is theirs with its password.
		a.Sessions.Put(r.Context(), sessionKeySSOLinkUser, user.ID)
		a.Sessions.Put(r.Context(), sessionKeySSOLinkEmail, ident.Email)
		a.Sessions.Put(r.Context(), sessionKeySSOLinkAt, time.Now().Unix())
		a.Sessions.Put(r.Context(), sessionKeySSOLinkIssuer, ident.Issuer)
		a.Sessions.Put(r.Context(), sessionKeySSOLinkSubject, ident.Subject)
		http.Redirect(w, r, "/auth/oidc/link", http.StatusSeeOther)
	case errors.Is(err, db.ErrUserNotFound):
		if !a.Cfg.OIDCAutoCreate {
			a.renderSSOError(w, r, "auth.error.sso_no_account")
			return
		}
		id, err := a.createSSOAccount(r.Context(), ident)
		if errors.Is(err, db.ErrEmailAlreadyUsed) || errors.Is(err, db.ErrIdentityTaken) {
			// Someone registered the address, or this identity got in, a moment ago.
			a.renderSSOError(w, r, "auth.error.sso_failed")
			return
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.finishSSOSignIn(w, r, id, true)
	default:
		a.serverError(w, r, err)
	}
}

// createSSOAccount makes an account for a new identity, with a random password nobody
// sees, and links it. The provider's values get the limits registration has.
func (a *App) createSSOAccount(ctx context.Context, ident *sso.Identity) (int64, error) {
	if _, err := mail.ParseAddress(ident.Email); err != nil {
		return 0, fmt.Errorf("sso email: %w", err)
	}
	name := ident.Name
	for len(name) > 64 { // the limit registration has
		_, n := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-n]
	}
	secret, err := randomHex(32)
	if err != nil {
		return 0, err
	}
	hash, err := auth.HashPassword(secret)
	if err != nil {
		return 0, err
	}
	id, err := a.Identities.CreateAccount(ctx, ident.Email, hash, name, ident.Issuer, ident.Subject)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// finishSSOSignIn puts the person in: the session id is renewed first.
func (a *App) finishSSOSignIn(w http.ResponseWriter, r *http.Request, userID int64, newAccount bool) {
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	user, err := a.Users.GetByID(r.Context(), userID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, userID)
	a.Sessions.Put(r.Context(), sessionKeyPwStamp, pwStamp(user.PasswordHash))
	if newAccount {
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "auth.flash.logged_in")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// pendingSSOLink is the account waiting for its password and the identity to link.
func (a *App) pendingSSOLink(r *http.Request) (userID int64, issuer, subject string, ok bool) {
	userID, _ = a.Sessions.Get(r.Context(), sessionKeySSOLinkUser).(int64)
	issuer = a.Sessions.GetString(r.Context(), sessionKeySSOLinkIssuer)
	subject = a.Sessions.GetString(r.Context(), sessionKeySSOLinkSubject)
	at, _ := a.Sessions.Get(r.Context(), sessionKeySSOLinkAt).(int64)
	ok = userID != 0 && issuer != "" && subject != "" && time.Since(time.Unix(at, 0)) < ssoLinkTTL
	if !ok {
		a.clearSSOLink(r)
	}
	return userID, issuer, subject, ok
}

// clearSSOLink forgets a link waiting for a password.
func (a *App) clearSSOLink(r *http.Request) {
	for _, k := range []string{sessionKeySSOLinkUser, sessionKeySSOLinkIssuer, sessionKeySSOLinkSubject, sessionKeySSOLinkEmail, sessionKeySSOLinkAt} {
		a.Sessions.Remove(r.Context(), k)
	}
}

// handleSSOLinkForm GET /auth/oidc/link: asks for the password of the account.
func (a *App) handleSSOLinkForm(w http.ResponseWriter, r *http.Request) {
	if _, _, _, ok := a.pendingSSOLink(r); !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	data := a.baseData(r)
	data.Form.Email = a.Sessions.GetString(r.Context(), sessionKeySSOLinkEmail)
	if err := a.Render.Render(w, r, http.StatusOK, "sso_link", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleSSOLink POST /auth/oidc/link: with the right password the identity is linked
// for good and the person is signed in.
func (a *App) handleSSOLink(w http.ResponseWriter, r *http.Request) {
	userID, issuer, subject, ok := a.pendingSSOLink(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	limiter := a.loginRateLimiter()
	ip := remoteIP(r)
	if !limiter.allow(ip) {
		a.renderSSOLinkError(w, r, http.StatusTooManyRequests, "auth.error.too_many_attempts")
		return
	}
	user, err := a.Users.GetByID(r.Context(), userID)
	if errors.Is(err, db.ErrUserNotFound) {
		a.clearSSOLink(r)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := auth.VerifyPassword(user.PasswordHash, r.PostFormValue("password")); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			a.renderSSOLinkError(w, r, http.StatusUnauthorized, "auth.error.invalid_credentials")
			return
		}
		a.serverError(w, r, err)
		return
	}
	// The limiter is not reset here: a right password must not buy more guesses.
	err = a.Identities.Link(r.Context(), userID, issuer, subject)
	if errors.Is(err, db.ErrIdentityTaken) {
		a.clearSSOLink(r)
		a.renderSSOLinkError(w, r, http.StatusConflict, "auth.error.sso_already_linked")
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.clearSSOLink(r)
	a.finishSSOSignIn(w, r, userID, false)
}

func (a *App) renderSSOLinkError(w http.ResponseWriter, r *http.Request, status int, key string) {
	data := a.baseData(r)
	data.Error = a.t(r, key)
	if err := a.Render.Render(w, r, status, "sso_link", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) renderSSOError(w http.ResponseWriter, r *http.Request, key string) {
	a.renderLoginError(w, r, a.baseData(r), a.t(r, key))
}
