package app

import (
	"context"
	"errors"
	"html"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/mfa"
)

const (
	sessionKeyMFAUser = "mfa_user"
	sessionKeyMFAAt   = "mfa_at"

	mfaPendingTTL  = 10 * time.Minute // from the right password to the right code
	emailCodeTTL   = 10 * time.Minute
	emailCodeEvery = time.Minute // a new email code is not sent more often than this
)

// ---- the step at sign-in -----------------------------------------------------

type mfaPageData struct {
	authPageData
	Method    string // totp, email or recovery
	HasTOTP   bool
	HasEmail  bool
	Email     string // where the code was mailed, masked
	Sent      bool
	Left      int
	Locked    bool
	FormError bool
}

// startMFA keeps who gave the right password and sends them to the second step. The person
// is not signed in yet: nothing but this step can follow.
func (a *App) startMFA(w http.ResponseWriter, r *http.Request, user *domain.User, st db.TwoFactorState) {
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), sessionKeyMFAUser, user.ID)
	a.Sessions.Put(r.Context(), sessionKeyMFAAt, time.Now().Unix())
	if !st.TOTPOn && st.EmailOn {
		a.mailCode(r.Context(), user, st) // errors are logged; "send again" is on the page
		http.Redirect(w, r, "/login/2fa?m=email", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login/2fa", http.StatusSeeOther)
}

// pendingMFA is the account waiting for its second step.
func (a *App) pendingMFA(r *http.Request) (int64, bool) {
	uid, _ := a.Sessions.Get(r.Context(), sessionKeyMFAUser).(int64)
	at, _ := a.Sessions.Get(r.Context(), sessionKeyMFAAt).(int64)
	if uid == 0 || time.Since(time.Unix(at, 0)) > mfaPendingTTL {
		a.clearMFA(r)
		return 0, false
	}
	return uid, true
}

func (a *App) clearMFA(r *http.Request) {
	a.Sessions.Remove(r.Context(), sessionKeyMFAUser)
	a.Sessions.Remove(r.Context(), sessionKeyMFAAt)
}

// mailCode sends a new email code unless one went a moment ago. It reports whether the
// person has one coming.
func (a *App) mailCode(ctx context.Context, user *domain.User, st db.TwoFactorState) bool {
	if a.Mailer == nil {
		return false
	}
	now := time.Now()
	if now.Sub(time.Unix(st.EmailCodeSent, 0)) < emailCodeEvery && st.EmailCodeExpires > now.Unix() {
		return true
	}
	code, err1 := mfa.NewEmailCode()
	salt, err2 := mfa.NewSalt()
	if err := errors.Join(err1, err2); err != nil {
		a.Logger.Error("2fa: email code", "err", err)
		return false
	}
	if err := a.TwoFactor.StoreEmailCode(ctx, user.ID, salt, mfa.HashCode(salt, code), now.Add(emailCodeTTL), now); err != nil {
		a.Logger.Error("2fa: store email code", "err", err)
		return false
	}
	lang := "en"
	if user.Locale != "" && i18n.Supported(user.Locale) {
		lang = user.Locale
	}
	subject, text, body := codeMail(lang, user.DisplayName, code)
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := a.Mailer.Send(sctx, user.Email, subject, text, body); err != nil {
		a.Logger.Error("2fa: send email code", "err", err)
		return false
	}
	return true
}

// codeMail is the email with the sign-in code.
func codeMail(lang, name, code string) (subject, text, htmlBody string) {
	subject = i18n.T(lang, "mail.code.subject")
	hello := i18n.T(lang, "mail.reset.hello", "Name", name)
	intro := i18n.T(lang, "mail.code.intro")
	expiry := i18n.T(lang, "mail.code.expiry")
	text = hello + "\n\n" + intro + " " + code + "\n\n" + expiry + "\n"
	e := html.EscapeString
	htmlBody = `<!doctype html><html lang="` + e(lang) + `"><body style="margin:0;padding:24px;background:#f7f8fa;font-family:Arial,Helvetica,sans-serif;color:#1f2328">` +
		`<div style="max-width:520px;margin:0 auto;background:#ffffff;border:1px solid #e3e6ea;border-radius:6px;padding:24px;font-size:15px;line-height:22px">` +
		`<p style="margin:0 0 12px">` + e(hello) + `</p><p style="margin:0 0 8px">` + e(intro) + `</p>` +
		`<p style="margin:0 0 16px;font-size:30px;letter-spacing:6px;font-weight:bold">` + e(code) + `</p>` +
		`<p style="margin:0;color:#5d6570;font-size:13px;line-height:19px">` + e(expiry) + `</p></div></body></html>`
	return subject, text, htmlBody
}

func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return email
	}
	return email[:1] + "***" + email[at:]
}

func (a *App) mfaData(r *http.Request, user *domain.User, st db.TwoFactorState, method string) mfaPageData {
	d := mfaPageData{authPageData: a.baseData(r), HasTOTP: st.TOTPOn, HasEmail: st.EmailOn && a.Mailer != nil, Method: method}
	d.Email = maskEmail(user.Email)
	d.Left = max(db.MaxMFAFailures-st.Failed, 0)
	d.Locked = st.Locked(time.Now())
	return d
}

// mfaMethod picks the screen: what was asked for if the account has it, else the app code
// or the email code.
func mfaMethod(asked string, st db.TwoFactorState, mailOn bool) string {
	switch {
	case asked == "recovery":
		return "recovery"
	case asked == "email" && st.EmailOn && mailOn:
		return "email"
	case asked == "totp" && st.TOTPOn:
		return "totp"
	case st.TOTPOn:
		return "totp"
	}
	return "email"
}

// handleMFAForm GET /login/2fa: the code to type.
func (a *App) handleMFAForm(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.pendingMFA(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	st, err := a.TwoFactor.Get(r.Context(), uid)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := a.mfaData(r, user, st, mfaMethod(r.URL.Query().Get("m"), st, a.Mailer != nil))
	d.Sent = r.URL.Query().Get("sent") == "1"
	if err := a.Render.Render(w, r, http.StatusOK, "mfa", d); err != nil {
		a.serverError(w, r, err)
	}
}

// handleMFAEmail POST /login/2fa/email: sends the email code again.
func (a *App) handleMFAEmail(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.pendingMFA(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	st, err := a.TwoFactor.Get(r.Context(), uid)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !st.EmailOn || a.Mailer == nil || !a.mfaLimiter().allow(remoteIP(r)) {
		http.Redirect(w, r, "/login/2fa", http.StatusSeeOther)
		return
	}
	a.mailCode(r.Context(), user, st)
	http.Redirect(w, r, "/login/2fa?m=email&sent=1", http.StatusSeeOther)
}

// handleMFA POST /login/2fa: checks the code. Five wrong ones in a row lock the step for
// five minutes; a right one finishes the sign-in.
func (a *App) handleMFA(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.pendingMFA(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	st, err := a.TwoFactor.Get(r.Context(), uid)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	method := mfaMethod(r.PostFormValue("method"), st, a.Mailer != nil)
	fail := func(status int) {
		d := a.mfaData(r, user, st, method)
		d.FormError = true
		if d.Locked {
			d.Error = a.t(r, "security.mfa.err_locked")
		} else {
			d.Error = a.t(r, "security.mfa.err_code", "Left", d.Left)
		}
		if err := a.Render.Render(w, r, status, "mfa", d); err != nil {
			a.serverError(w, r, err)
		}
	}
	if st.Locked(time.Now()) || !a.mfaLimiter().allow(remoteIP(r)) {
		st.LockedUntil = time.Now().Add(db.MFALock).Unix()
		fail(http.StatusTooManyRequests)
		return
	}
	good, err := a.checkMFACode(r.Context(), user.ID, st, method, r.PostFormValue("code"))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !good {
		if st, err = a.TwoFactor.Fail(r.Context(), uid, time.Now()); err != nil {
			a.serverError(w, r, err)
			return
		}
		fail(http.StatusUnauthorized)
		return
	}
	if err := a.TwoFactor.ResetFailures(r.Context(), uid); err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.clearMFA(r)
	a.Sessions.Put(r.Context(), sessionKeyUserID, user.ID)
	a.Sessions.Put(r.Context(), sessionKeyPwStamp, pwStamp(user.PasswordHash))
	a.Sessions.Put(r.Context(), "flash", "auth.flash.logged_in")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// checkMFACode tells whether code is right for the method, and spends it if so.
func (a *App) checkMFACode(ctx context.Context, uid int64, st db.TwoFactorState, method, code string) (bool, error) {
	switch method {
	case "totp":
		secret, err := a.openTOTP(st)
		if err != nil {
			return false, err
		}
		step, ok := mfa.Verify(secret, code, time.Now(), st.LastStep)
		if !ok {
			return false, nil
		}
		return a.TwoFactor.UseStep(ctx, uid, step)
	case "email":
		code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
		return a.TwoFactor.UseEmailCode(ctx, uid, code, time.Now())
	case "recovery":
		return a.TwoFactor.UseRecoveryCode(ctx, uid, mfa.NormalizeRecovery(code), time.Now())
	}
	return false, nil
}

func (a *App) openTOTP(st db.TwoFactorState) (string, error) {
	if a.StoreBox == nil || st.TOTPSecret == "" {
		return "", errors.New("the authenticator app is not available")
	}
	return a.StoreBox.Open(st.TOTPSecret)
}

// ---- Settings: Security -------------------------------------------------------

// securityView is the page that lists what is on.
type securityView struct {
	TOTPOn, EmailOn bool
	CodesLeft       int
	HasKey, MailOn  bool
	Any             bool
}

func (a *App) loadSecurity(r *http.Request, user *domain.User) (*securityView, error) {
	st, err := a.TwoFactor.Get(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	left, err := a.TwoFactor.RecoveryLeft(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	return &securityView{TOTPOn: st.TOTPOn, EmailOn: st.EmailOn, CodesLeft: left, HasKey: a.StoreBox != nil,
		MailOn: a.Mailer != nil, Any: st.Enabled()}, nil
}

// secPageData is a page of the set-up procedure.
type secPageData struct {
	authPageData
	QR    template.HTML // the QR code, drawn by this app from the secret
	Key   string
	Codes []string
	Email string
}

func (a *App) secData(r *http.Request, user *domain.User) secPageData {
	d := secPageData{authPageData: a.baseData(r), Email: user.Email}
	d.ActiveNav = "/settings"
	return d
}

func (a *App) securityBack(w http.ResponseWriter, r *http.Request, flash string) {
	if flash != "" {
		a.Sessions.Put(r.Context(), "flash", flash)
	}
	http.Redirect(w, r, "/settings/sicurezza", http.StatusSeeOther)
}

// passwordOK checks the password sent with a form that turns something off: the sign-in
// limiter counts a wrong one.
func (a *App) passwordOK(r *http.Request, user *domain.User) bool {
	if !a.loginRateLimiter().allow(remoteIP(r)) {
		return false
	}
	return auth.VerifyPassword(user.PasswordHash, r.PostFormValue("password")) == nil
}

// showCodes makes new recovery codes and shows them, once.
func (a *App) showCodes(w http.ResponseWriter, r *http.Request, user *domain.User) {
	codes, err := mfa.NewRecoveryCodes(mfa.RecoveryCodes)
	if err == nil {
		err = a.TwoFactor.ReplaceRecoveryCodes(r.Context(), user.ID, codes)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := a.secData(r, user)
	d.Codes = codes
	w.Header().Set("Cache-Control", "no-store")
	if err := a.Render.Render(w, r, http.StatusOK, "sec_codes", d); err != nil {
		a.serverError(w, r, err)
	}
}

// afterEnable shows the recovery codes when the account has none left, else goes back.
func (a *App) afterEnable(w http.ResponseWriter, r *http.Request, user *domain.User) {
	left, err := a.TwoFactor.RecoveryLeft(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if left == 0 {
		a.showCodes(w, r, user)
		return
	}
	a.securityBack(w, r, "security.flash.on")
}

// handleTOTPScan GET /settings/security/totp: step 1, the QR code. A secret not yet
// confirmed is kept, so reloading the page does not change the code.
func (a *App) handleTOTPScan(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if a.StoreBox == nil {
		a.securityBack(w, r, "")
		return
	}
	st, err := a.TwoFactor.Get(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if st.TOTPOn {
		a.securityBack(w, r, "")
		return
	}
	secret := ""
	if st.TOTPSecret != "" {
		secret, _ = a.openTOTP(st)
	}
	if secret == "" {
		if secret, err = mfa.NewSecret(); err == nil {
			var sealed string
			if sealed, err = a.StoreBox.Seal(secret); err == nil {
				err = a.TwoFactor.SetPendingTOTP(r.Context(), user.ID, sealed)
			}
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	svg, err := mfa.QRSVG(mfa.URI("Nutreek", user.Email, secret))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := a.secData(r, user)
	d.QR = template.HTML(svg) //nolint:gosec // drawn by mfa.QRSVG from the secret: digits, no markup of anyone's
	d.Key = mfa.FormatKey(secret)
	w.Header().Set("Cache-Control", "no-store")
	if err := a.Render.Render(w, r, http.StatusOK, "sec_totp_scan", d); err != nil {
		a.serverError(w, r, err)
	}
}

// handleTOTPConfirmForm GET /settings/security/totp/confirm: step 2, the code from the app.
func (a *App) handleTOTPConfirmForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	st, err := a.TwoFactor.Get(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if st.TOTPOn || st.TOTPSecret == "" {
		a.securityBack(w, r, "")
		return
	}
	if err := a.Render.Render(w, r, http.StatusOK, "sec_totp_confirm", a.secData(r, user)); err != nil {
		a.serverError(w, r, err)
	}
}

// handleTOTPConfirm POST /settings/security/totp/confirm: a right code turns the app on.
func (a *App) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	st, err := a.TwoFactor.Get(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if st.TOTPOn || st.TOTPSecret == "" {
		a.securityBack(w, r, "")
		return
	}
	secret, err := a.openTOTP(st)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	step, ok := mfa.Verify(secret, r.PostFormValue("code"), time.Now(), 0)
	if !ok || !a.mfaLimiter().allow(remoteIP(r)) {
		d := a.secData(r, user)
		d.Error = a.t(r, "security.totp.err_code")
		if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "sec_totp_confirm", d); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	if err := a.TwoFactor.EnableTOTP(r.Context(), user.ID, step); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.afterEnable(w, r, user)
}

// handleTOTPDisable POST /settings/security/totp/disable
func (a *App) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if !a.passwordOK(r, user) {
		a.securityBack(w, r, "security.flash.err_password")
		return
	}
	if err := a.TwoFactor.DisableTOTP(r.Context(), user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropCodesIfOff(r.Context(), user.ID)
	a.securityBack(w, r, "security.flash.off")
}

// dropCodesIfOff forgets the recovery codes when no method is left on.
func (a *App) dropCodesIfOff(ctx context.Context, userID int64) {
	if st, err := a.TwoFactor.Get(ctx, userID); err == nil && !st.Enabled() {
		if err := a.TwoFactor.DeleteRecoveryCodes(ctx, userID); err != nil {
			a.Logger.Error("2fa: forget recovery codes", "err", err)
		}
	}
}

// handleEmailStart POST /settings/security/email/start: mails a code to confirm the address works.
func (a *App) handleEmailStart(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if a.Mailer == nil {
		a.securityBack(w, r, "")
		return
	}
	st, err := a.TwoFactor.Get(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !st.EmailOn && a.mfaLimiter().allow(remoteIP(r)) {
		a.mailCode(r.Context(), user, st)
	}
	http.Redirect(w, r, "/settings/security/email", http.StatusSeeOther)
}

// handleEmailForm GET /settings/security/email: the code that was mailed.
func (a *App) handleEmailForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if a.Mailer == nil {
		a.securityBack(w, r, "")
		return
	}
	if err := a.Render.Render(w, r, http.StatusOK, "sec_email", a.secData(r, user)); err != nil {
		a.serverError(w, r, err)
	}
}

// handleEmailConfirm POST /settings/security/email/confirm: a right code turns the email step on.
func (a *App) handleEmailConfirm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if a.Mailer == nil {
		a.securityBack(w, r, "")
		return
	}
	ok := false
	if a.mfaLimiter().allow(remoteIP(r)) {
		var err error
		code := strings.ReplaceAll(strings.TrimSpace(r.PostFormValue("code")), " ", "")
		if ok, err = a.TwoFactor.UseEmailCode(r.Context(), user.ID, code, time.Now()); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if !ok {
		d := a.secData(r, user)
		d.Error = a.t(r, "security.totp.err_code")
		if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "sec_email", d); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	if err := a.TwoFactor.SetEmailOn(r.Context(), user.ID, true); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.afterEnable(w, r, user)
}

// handleEmailDisable POST /settings/security/email/disable
func (a *App) handleEmailDisable(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if !a.passwordOK(r, user) {
		a.securityBack(w, r, "security.flash.err_password")
		return
	}
	if err := a.TwoFactor.SetEmailOn(r.Context(), user.ID, false); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.dropCodesIfOff(r.Context(), user.ID)
	a.securityBack(w, r, "security.flash.off")
}

// handleCodesRegenerate POST /settings/security/codes: a new set, the old one is void.
func (a *App) handleCodesRegenerate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	st, err := a.TwoFactor.Get(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !st.Enabled() {
		a.securityBack(w, r, "")
		return
	}
	if !a.passwordOK(r, user) {
		a.securityBack(w, r, "security.flash.err_password")
		return
	}
	a.showCodes(w, r, user)
}
