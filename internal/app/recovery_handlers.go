package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"html"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/i18n"
)

// Sender is what the app needs from the mailer; nil when no email is set up.
type Sender interface {
	Send(ctx context.Context, to, subject, text, html string) error
}

const (
	resetTTL = time.Hour
	resetGap = 15 * time.Minute // a link younger than this is not replaced

	sessionKeyPwStamp = "pw_stamp"
)

func (a *App) mailOn() bool { return a.Mailer != nil && a.PasswordResets != nil }

// resetHash is what is stored of a reset token: the token itself only travels in the email.
func resetHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// pwStamp is a short fingerprint of a password hash. A session remembers the one it
// started with, and stops working when the password changes.
func pwStamp(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return hex.EncodeToString(sum[:8])
}

// logPath is a path fit for the logs: the token of a reset link is not written there.
func logPath(p string) string {
	if strings.HasPrefix(p, "/reset/") {
		return "/reset/…"
	}
	return p
}

// handleForgotForm GET /forgot: asks for the email of the account.
func (a *App) handleForgotForm(w http.ResponseWriter, r *http.Request) {
	if userFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := a.Render.Render(w, r, http.StatusOK, "forgot", a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}

// handleForgot POST /forgot: always answers the same, whether the account exists or not.
// The work (lookup, token, email) happens after the answer, so the time it takes says
// nothing either.
func (a *App) handleForgot(w http.ResponseWriter, r *http.Request) {
	if !a.forgotLimiter().allow(remoteIP(r)) {
		data := a.baseData(r)
		data.Error = a.t(r, "auth.error.too_many_requests")
		if err := a.Render.Render(w, r, http.StatusTooManyRequests, "forgot", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	if _, err := mail.ParseAddress(email); err != nil {
		data := a.baseData(r)
		data.Form.Email = email
		data.Error = a.t(r, "auth.error.email_invalid")
		if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "forgot", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	if a.forgotEmailLimiter().allow(email) {
		lang := i18n.FromContext(r.Context())
		select {
		case a.forgotSlots <- struct{}{}: // at most a few sends at once
			go func() {
				defer func() { <-a.forgotSlots }()
				a.sendReset(email, lang)
			}()
		default:
		}
	}
	http.Redirect(w, r, "/forgot/sent", http.StatusSeeOther)
}

// sendReset makes a token for the account behind email and mails the link. Nothing is
// sent, and nothing said, when there is no such account.
func (a *App) sendReset(email, lang string) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	user, err := a.Users.GetByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, db.ErrUserNotFound) {
			a.Logger.Error("password reset: find account", "err", err)
		}
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		a.Logger.Error("password reset: token", "err", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := a.PasswordResets.Create(ctx, user.ID, resetHash(token), time.Now().Add(resetTTL), resetGap); err != nil {
		if !errors.Is(err, db.ErrResetRecent) {
			a.Logger.Error("password reset: store token", "err", err)
		}
		return
	}
	if user.Locale != "" && i18n.Supported(user.Locale) {
		lang = user.Locale
	}
	subject, text, body := resetMail(lang, user.DisplayName, a.Cfg.BaseURL+"/reset/"+token)
	if err := a.Mailer.Send(ctx, user.Email, subject, text, body); err != nil {
		a.Logger.Error("password reset: send", "err", err)
	}
}

// resetMail is the email with the link, in the person's language.
func resetMail(lang, name, link string) (subject, text, htmlBody string) {
	subject = i18n.T(lang, "mail.reset.subject")
	hello := i18n.T(lang, "mail.reset.hello", "Name", name)
	intro := i18n.T(lang, "mail.reset.intro")
	button := i18n.T(lang, "mail.reset.button")
	expiry := i18n.T(lang, "mail.reset.expiry")
	fallback := i18n.T(lang, "mail.reset.fallback")
	text = hello + "\n\n" + intro + "\n\n" + link + "\n\n" + expiry + "\n"
	e := html.EscapeString
	htmlBody = `<!doctype html><html lang="` + e(lang) + `"><body style="margin:0;padding:24px;background:#f7f8fa;font-family:Arial,Helvetica,sans-serif;color:#1f2328">` +
		`<div style="max-width:520px;margin:0 auto;background:#ffffff;border:1px solid #e3e6ea;border-radius:6px;padding:24px;font-size:15px;line-height:22px">` +
		`<p style="margin:0 0 12px">` + e(hello) + `</p>` +
		`<p style="margin:0 0 16px">` + e(intro) + `</p>` +
		`<p style="margin:0 0 16px"><a href="` + e(link) + `" style="display:inline-block;background:#276b49;color:#ffffff;text-decoration:none;font-weight:bold;padding:12px 18px;border-radius:6px">` + e(button) + `</a></p>` +
		`<p style="margin:0 0 12px;color:#5d6570;font-size:13px;line-height:19px">` + e(expiry) + `</p>` +
		`<p style="margin:0;color:#5d6570;font-size:13px;line-height:19px">` + e(fallback) + `<br><span style="word-break:break-all">` + e(link) + `</span></p>` +
		`</div></body></html>`
	return subject, text, htmlBody
}

// handleForgotSent GET /forgot/sent: the same page for everyone.
func (a *App) handleForgotSent(w http.ResponseWriter, r *http.Request) {
	if err := a.Render.Render(w, r, http.StatusOK, "forgot_sent", a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}

// handleResetForm GET /reset/{token}: the page to choose a new password, if the link still works.
func (a *App) handleResetForm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "same-origin") // the token is in the address: no other site gets it
	token := chi.URLParam(r, "token")
	ok, err := a.PasswordResets.Valid(r.Context(), resetHash(token), time.Now())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !ok {
		a.renderResetExpired(w, r)
		return
	}
	data := a.baseData(r)
	data.FormAction = "/reset/" + token
	if err := a.Render.Render(w, r, http.StatusOK, "reset", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleReset POST /reset/{token}: sets the password and uses the link up. The password is
// checked before the link is spent, so a short one can be corrected.
func (a *App) handleReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "same-origin")
	token := chi.URLParam(r, "token")
	hash, err := auth.HashPassword(r.PostFormValue("password"))
	if err != nil {
		key := ""
		switch {
		case errors.Is(err, auth.ErrPasswordTooShort):
			key = "auth.error.password_short"
		case errors.Is(err, auth.ErrPasswordTooLong):
			key = "auth.error.password_long"
		default:
			a.serverError(w, r, err)
			return
		}
		data := a.baseData(r)
		data.FormAction = "/reset/" + token
		data.Error = a.t(r, key)
		if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "reset", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	userID, err := a.PasswordResets.Consume(r.Context(), resetHash(token), time.Now())
	if errors.Is(err, db.ErrResetNotFound) {
		a.renderResetExpired(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Users.UpdatePassword(r.Context(), userID, hash); err != nil {
		a.serverError(w, r, err)
		return
	}
	// Whoever held the account before is out: sessions that started with the old password
	// stop working (see loadUserMiddleware), and API tokens and provider links go too.
	if err := errors.Join(a.APITokens.RevokeAll(r.Context(), userID), a.Identities.UnlinkAll(r.Context(), userID)); err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Remove(r.Context(), sessionKeyUserID)
	a.Sessions.Remove(r.Context(), sessionKeyPwStamp)
	a.Sessions.Put(r.Context(), "flash", "auth.flash.password_reset")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *App) renderResetExpired(w http.ResponseWriter, r *http.Request) {
	if err := a.Render.Render(w, r, http.StatusGone, "reset_expired", a.baseData(r)); err != nil {
		a.serverError(w, r, err)
	}
}
