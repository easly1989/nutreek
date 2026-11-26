package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

const (
	sessionKeyWALogin = "wa_login" // the sign-in ceremony in progress (JSON)
	sessionKeyWAReg   = "wa_reg"   // the registration ceremony in progress (JSON)
	sessionKeyWAName  = "wa_name"
	sessionKeyWAAt    = "wa_at"

	passkeyCeremonyTTL = 5 * time.Minute
	maxPasskeyBody     = 64 << 10
)

func (a *App) passkeyOn() bool { return a.WebAuthn != nil && a.Passkeys != nil && a.Demo == nil }

// waUser is an account as the WebAuthn library sees it.
type waUser struct {
	id          int64
	handle      []byte
	name        string
	creds       []webauthn.Credential
	passwordKey string
}

func (u waUser) WebAuthnID() []byte                         { return u.handle }
func (u waUser) WebAuthnName() string                       { return u.name }
func (u waUser) WebAuthnDisplayName() string                { return u.name }
func (u waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// loadWAUser reads the account's passkeys. An account with none gets a fresh random handle.
func (a *App) loadWAUser(ctx context.Context, user *domain.User) (waUser, error) {
	list, err := a.Passkeys.List(ctx, user.ID)
	if err != nil {
		return waUser{}, err
	}
	u := waUser{id: user.ID, name: user.Email}
	for _, p := range list {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(p.Credential), &c); err != nil {
			return waUser{}, err
		}
		u.creds = append(u.creds, c)
		u.handle = p.Handle
	}
	if u.handle == nil {
		u.handle = make([]byte, 32)
		if _, err := rand.Read(u.handle); err != nil {
			return waUser{}, err
		}
	}
	return u, nil
}

// pkJSON answers with JSON that is not kept by any cache.
func pkJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, v)
}

func pkError(w http.ResponseWriter, status int, msg string) {
	pkJSON(w, status, map[string]string{"error": msg})
}

// putCeremony keeps the ceremony's data in the session until it ends.
func (a *App) putCeremony(r *http.Request, key string, sd *webauthn.SessionData) error {
	raw, err := json.Marshal(sd)
	if err != nil {
		return err
	}
	a.Sessions.Put(r.Context(), key, string(raw))
	a.Sessions.Put(r.Context(), sessionKeyWAAt, time.Now().Unix())
	return nil
}

// popCeremony takes the ceremony's data out: it works once, and not after five minutes.
func (a *App) popCeremony(r *http.Request, key string) (webauthn.SessionData, bool) {
	raw := a.Sessions.PopString(r.Context(), key)
	at, _ := a.Sessions.Get(r.Context(), sessionKeyWAAt).(int64)
	var sd webauthn.SessionData
	if raw == "" || time.Since(time.Unix(at, 0)) > passkeyCeremonyTTL || json.Unmarshal([]byte(raw), &sd) != nil {
		return sd, false
	}
	return sd, true
}

// ---- sign in -------------------------------------------------------------------

// handlePasskeyLoginBegin POST /login/passkey/begin: the challenge for a sign-in. The
// person need not say who they are: the passkey does.
func (a *App) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if !a.passkeyLimiter().allow(remoteIP(r)) {
		pkError(w, http.StatusTooManyRequests, "too many attempts")
		return
	}
	opts, sd, err := a.WebAuthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		a.Logger.Error("passkey: begin sign-in", "err", err)
		pkError(w, http.StatusInternalServerError, "could not start")
		return
	}
	if err := a.putCeremony(r, sessionKeyWALogin, sd); err != nil {
		pkError(w, http.StatusInternalServerError, "could not start")
		return
	}
	pkJSON(w, http.StatusOK, opts)
}

// handlePasskeyLoginFinish POST /login/passkey/finish: checks the signature and signs the
// person in. A passkey is a full sign-in: no password and no second step follow.
func (a *App) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPasskeyBody)
	sd, ok := a.popCeremony(r, sessionKeyWALogin)
	if !ok || !a.passkeyLimiter().allow(remoteIP(r)) {
		pkError(w, http.StatusUnauthorized, "sign-in expired")
		return
	}
	var picked db.Passkey
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		p, err := a.Passkeys.ByCredentialID(r.Context(), rawID)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(p.Handle, userHandle) { // the account is decided by the handle, not a name
			return nil, errors.New("user handle does not match")
		}
		user, err := a.Users.GetByID(r.Context(), p.UserID)
		if err != nil {
			return nil, err
		}
		picked = p
		return a.loadWAUser(r.Context(), user)
	}
	wu, cred, err := a.WebAuthn.FinishPasskeyLogin(handler, sd, r)
	if err != nil || cred == nil || cred.Authenticator.CloneWarning {
		if err != nil {
			a.Logger.Warn("passkey: sign-in refused", "err", err)
		} else {
			a.Logger.Warn("passkey: sign-in refused, the sign count went back (a cloned key?)")
		}
		pkError(w, http.StatusUnauthorized, "passkey not accepted")
		return
	}
	raw, err := json.Marshal(cred)
	if err == nil {
		err = a.Passkeys.Touch(r.Context(), picked.ID, string(raw), time.Now())
	}
	if err != nil {
		a.Logger.Error("passkey: record sign-in", "err", err)
		pkError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	user, err := a.Users.GetByID(r.Context(), wu.(waUser).id)
	if err != nil {
		pkError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		pkError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyUserID, user.ID)
	a.Sessions.Put(r.Context(), sessionKeyPwStamp, pwStamp(user.PasswordHash))
	a.Sessions.Put(r.Context(), "flash", "auth.flash.logged_in")
	pkJSON(w, http.StatusOK, map[string]string{"redirect": "/"})
}

// ---- add and remove -------------------------------------------------------------

// handlePasskeyNew GET /settings/security/passkeys/new: the name and the password, then the browser asks.
func (a *App) handlePasskeyNew(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	d := a.secData(r, user)
	if err := a.Render.Render(w, r, http.StatusOK, "sec_passkey_new", d); err != nil {
		a.serverError(w, r, err)
	}
}

// handlePasskeyAddBegin POST /settings/security/passkeys/begin: JSON {name, password}. Adding
// a passkey asks for the password: a session alone must not be able to leave itself a way in.
func (a *App) handlePasskeyAddBegin(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxPasskeyBody)
	var in struct{ Name, Password string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		pkError(w, http.StatusBadRequest, "bad request")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 64 {
		pkError(w, http.StatusUnprocessableEntity, a.t(r, "security.passkey.err_name"))
		return
	}
	if !a.loginRateLimiter().allow(remoteIP(r)) || auth.VerifyPassword(user.PasswordHash, in.Password) != nil {
		pkError(w, http.StatusForbidden, a.t(r, "security.flash.err_password"))
		return
	}
	wu, err := a.loadWAUser(r.Context(), user)
	if err != nil {
		pkError(w, http.StatusInternalServerError, "could not start")
		return
	}
	if len(wu.creds) >= db.MaxPasskeys {
		pkError(w, http.StatusUnprocessableEntity, a.t(r, "security.passkey.err_many"))
		return
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(wu.creds))
	for _, c := range wu.creds {
		exclude = append(exclude, c.Descriptor())
	}
	creation, sd, err := a.WebAuthn.BeginRegistration(wu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(exclude))
	if err != nil {
		a.Logger.Error("passkey: begin registration", "err", err)
		pkError(w, http.StatusInternalServerError, "could not start")
		return
	}
	if err := a.putCeremony(r, sessionKeyWAReg, sd); err != nil {
		pkError(w, http.StatusInternalServerError, "could not start")
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyWAName, name)
	pkJSON(w, http.StatusOK, creation)
}

// handlePasskeyAddFinish POST /settings/security/passkeys/finish: stores the new passkey.
func (a *App) handlePasskeyAddFinish(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxPasskeyBody)
	sd, ok := a.popCeremony(r, sessionKeyWAReg)
	name := a.Sessions.PopString(r.Context(), sessionKeyWAName)
	if !ok || name == "" {
		pkError(w, http.StatusUnauthorized, "expired")
		return
	}
	wu, err := a.loadWAUser(r.Context(), user)
	if err != nil {
		pkError(w, http.StatusInternalServerError, "could not save")
		return
	}
	if len(wu.creds) == 0 {
		wu.handle = sd.UserID // the first passkey: the handle made at the start, not a new one
	}
	cred, err := a.WebAuthn.FinishRegistration(wu, sd, r)
	if err != nil {
		a.Logger.Warn("passkey: registration refused", "err", err)
		pkError(w, http.StatusUnprocessableEntity, a.t(r, "security.passkey.err_failed"))
		return
	}
	raw, err := json.Marshal(cred)
	if err == nil {
		err = a.Passkeys.Add(r.Context(), user.ID, wu.handle, cred.ID, name, string(raw), time.Now())
	}
	if err != nil {
		a.Logger.Error("passkey: save", "err", err)
		pkError(w, http.StatusInternalServerError, "could not save")
		return
	}
	a.Sessions.Put(r.Context(), "flash", "security.flash.on")
	pkJSON(w, http.StatusOK, map[string]string{"redirect": "/settings/sicurezza"})
}

// handlePasskeyDelete POST /settings/security/passkeys/{id}/delete: asks for the password.
func (a *App) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	if !a.passwordOK(r, user) {
		a.securityBack(w, r, "security.flash.err_password")
		return
	}
	if _, err := a.Passkeys.Delete(r.Context(), id, user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.securityBack(w, r, "security.flash.off")
}
