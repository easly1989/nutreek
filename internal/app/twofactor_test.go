package app

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/auth"
	"github.com/amon2126/nutreek/internal/mfa"
	"github.com/amon2126/nutreek/internal/stores"
)

// mfaApp is an app with email and a secret key, so every method is available.
func mfaApp(t *testing.T) (*App, *fakeSender, *browser) {
	t.Helper()
	a, fs, b := recoveryApp(t)
	box, err := stores.NewBox("a-secret-key-of-enough-length")
	if err != nil {
		t.Fatal(err)
	}
	a.StoreBox = box
	return a, fs, b
}

// totpOn turns the app method on for a user and returns its secret.
func totpOn(t *testing.T, a *App, userID int64) string {
	t.Helper()
	ctx := context.Background()
	secret, _ := mfa.NewSecret()
	sealed, _ := a.StoreBox.Seal(secret)
	if err := a.TwoFactor.SetPendingTOTP(ctx, userID, sealed); err != nil {
		t.Fatal(err)
	}
	if err := a.TwoFactor.EnableTOTP(ctx, userID, 0); err != nil {
		t.Fatal(err)
	}
	return secret
}

func logIn(b *browser, email, pw string) (int, string) {
	b.t.Helper()
	code, loc, _ := b.submit("/login", "/login", url.Values{"email": {email}, "password": {pw}})
	return code, loc
}

func code2fa(b *browser, method, code string) (int, string, string) {
	b.t.Helper()
	return b.submit("/login/2fa", "/login/2fa", url.Values{"method": {method}, "code": {code}})
}

func signedIn(b *browser) bool {
	b.t.Helper()
	c, _, _ := b.get("/tu")
	return c == 200
}

func TestMFAAppCodeAtSignIn(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	secret := totpOn(t, a, user.ID)

	if code, loc := logIn(b, "carlo@example.test", "right-password"); code != 303 || loc != "/login/2fa" {
		t.Fatalf("login: %d -> %q", code, loc)
	}
	if signedIn(b) {
		t.Fatal("the right password alone must not sign in")
	}
	if code, _, body := code2fa(b, "totp", "000000"); code != 401 || !strings.Contains(body, "4") {
		t.Fatalf("wrong code: %d", code)
	}
	good, _ := mfa.CodeAt(secret, time.Now())
	if code, loc, _ := code2fa(b, "totp", good); code != 303 || loc != "/" {
		t.Fatalf("right code: %d -> %q", code, loc)
	}
	if !signedIn(b) {
		t.Fatal("should be signed in")
	}

	// The same code cannot be used again.
	b2 := newBrowser(t, a)
	logIn(b2, "carlo@example.test", "right-password")
	if code, _, _ := code2fa(b2, "totp", good); code != 401 {
		t.Fatalf("a code used once must fail: %d", code)
	}
}

func TestMFAFiveWrongCodesLock(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	secret := totpOn(t, a, user.ID)
	logIn(b, "carlo@example.test", "right-password")
	for i := 0; i < 5; i++ {
		if code, _, _ := code2fa(b, "totp", "000000"); code != 401 {
			t.Fatalf("try %d: %d", i+1, code)
		}
	}
	good, _ := mfa.CodeAt(secret, time.Now())
	if code, _, _ := code2fa(b, "totp", good); code != 429 {
		t.Fatalf("a locked step must refuse even a right code: %d", code)
	}
	if signedIn(b) {
		t.Fatal("must stay signed out")
	}
	// The lock ends by itself.
	if _, err := a.Store.DB.Exec(`UPDATE user_2fa SET locked_until = 1`); err != nil {
		t.Fatal(err)
	}
	if code, loc, _ := code2fa(b, "totp", good); code != 303 || loc != "/" {
		t.Fatalf("after the lock: %d -> %q", code, loc)
	}
}

func TestMFARecoveryCodeWorksOnce(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	totpOn(t, a, user.ID)
	codes, _ := mfa.NewRecoveryCodes(3)
	if err := a.TwoFactor.ReplaceRecoveryCodes(context.Background(), user.ID, codes); err != nil {
		t.Fatal(err)
	}
	logIn(b, "carlo@example.test", "right-password")
	typed := strings.ToUpper(strings.ReplaceAll(codes[0], "-", " ")) // a person types it any way
	if code, loc, _ := code2fa(b, "recovery", typed); code != 303 || loc != "/" {
		t.Fatalf("recovery: %d -> %q", code, loc)
	}
	b2 := newBrowser(t, a)
	logIn(b2, "carlo@example.test", "right-password")
	if code, _, _ := code2fa(b2, "recovery", codes[0]); code != 401 {
		t.Fatalf("a used code: %d", code)
	}
	if left, _ := a.TwoFactor.RecoveryLeft(context.Background(), user.ID); left != 2 {
		t.Fatalf("left = %d", left)
	}
}

var sixDigits = regexp.MustCompile(`\b(\d{6})\b`)

func TestMFAEmailCodeAtSignIn(t *testing.T) {
	a, fs, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	if err := a.TwoFactor.SetEmailOn(context.Background(), user.ID, true); err != nil {
		t.Fatal(err)
	}
	if code, loc := logIn(b, "carlo@example.test", "right-password"); code != 303 || loc != "/login/2fa?m=email" {
		t.Fatalf("login: %d -> %q", code, loc)
	}
	m := fs.wait(t, 1)[0]
	g := sixDigits.FindStringSubmatch(m.text)
	if m.to != "carlo@example.test" || g == nil {
		t.Fatalf("mail = %+v", m)
	}
	// Asking again within a minute does not send another.
	b.submit("/login/2fa", "/login/2fa/email", url.Values{})
	time.Sleep(200 * time.Millisecond)
	if fs.count() != 1 {
		t.Fatalf("mails = %d, want 1", fs.count())
	}
	if code, _, _ := code2fa(b, "email", "111111"); code != 401 && g[1] != "111111" {
		t.Fatalf("a wrong code: %d", code)
	}
	if code, loc, _ := code2fa(b, "email", g[1]); code != 303 || loc != "/" {
		t.Fatalf("right code: %d -> %q", code, loc)
	}
	// It works once.
	b2 := newBrowser(t, a)
	logIn(b2, "carlo@example.test", "right-password")
	if code, _, _ := code2fa(b2, "email", g[1]); code != 401 {
		t.Fatalf("a used code: %d", code)
	}
}

func TestMFAEmailCodeExpires(t *testing.T) {
	a, fs, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	_ = a.TwoFactor.SetEmailOn(context.Background(), user.ID, true)
	logIn(b, "carlo@example.test", "right-password")
	g := sixDigits.FindStringSubmatch(fs.wait(t, 1)[0].text)
	if _, err := a.Store.DB.Exec(`UPDATE user_2fa SET email_code_expires = 1`); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := code2fa(b, "email", g[1]); code != 401 {
		t.Fatalf("an expired code: %d", code)
	}
}

func TestMFAStepNeedsTheRightPasswordFirst(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	totpOn(t, a, user.ID)
	if code, loc, _ := b.get("/login/2fa"); code != 303 || loc != "/login" {
		t.Fatalf("no password yet: %d -> %q", code, loc)
	}
	if code, _ := logIn(b, "carlo@example.test", "wrong"); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	if code, loc, _ := b.get("/login/2fa"); code != 303 || loc != "/login" {
		t.Fatalf("a wrong password must not open the step: %d -> %q", code, loc)
	}
}

func TestMFAIsSkippedByProviderSignIn(t *testing.T) {
	a, p, b := ssoApp(t, false)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	box, _ := stores.NewBox("a-secret-key-of-enough-length")
	a.StoreBox = box
	totpOn(t, a, user.ID)
	if err := a.Identities.Link(context.Background(), user.ID, p.URL, "u1"); err != nil {
		t.Fatal(err)
	}
	if status, redirect := ssoTrip(t, b, p); status != 303 || redirect != "/" {
		t.Fatalf("provider sign-in: %d -> %q", status, redirect)
	}
	if !signedIn(b) {
		t.Fatal("the provider does its own second step")
	}
}

func TestSecuritySetUpTheApp(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	logIn(b, "carlo@example.test", "right-password")

	if code, _, body := b.get("/settings/sicurezza"); code != 200 || !strings.Contains(body, "/settings/security/totp") {
		t.Fatalf("list: %d", code)
	}
	code, _, body := b.get("/settings/security/totp")
	if code != 200 || !strings.Contains(body, "<svg") {
		t.Fatalf("step 1: %d", code)
	}
	// Reloading shows the same secret.
	st, _ := a.TwoFactor.Get(context.Background(), user.ID)
	secret, _ := a.openTOTP(st)
	if _, _, again := b.get("/settings/security/totp"); !strings.Contains(again, mfa.FormatKey(secret)) || !strings.Contains(body, mfa.FormatKey(secret)) {
		t.Fatal("the key should stay the same until confirmed")
	}
	if code, _, _ := b.get("/settings/security/totp/confirm"); code != 200 {
		t.Fatalf("step 2: %d", code)
	}
	if code, _, _ := b.submit("/settings/security/totp/confirm", "/settings/security/totp/confirm", url.Values{"code": {"000000"}, "password": {"right-password"}}); code != 422 {
		t.Fatalf("a wrong code: %d", code)
	}
	if st, _ := a.TwoFactor.Get(context.Background(), user.ID); st.TOTPOn {
		t.Fatal("must not be on yet")
	}
	good, _ := mfa.CodeAt(secret, time.Now())
	code, _, body = b.submit("/settings/security/totp/confirm", "/settings/security/totp/confirm", url.Values{"code": {good}, "password": {"right-password"}})
	if code != 200 || strings.Count(body, "<li>") < mfa.RecoveryCodes {
		t.Fatalf("step 3 should show the recovery codes: %d", code)
	}
	if st, _ := a.TwoFactor.Get(context.Background(), user.ID); !st.TOTPOn {
		t.Fatal("should be on")
	}
	if left, _ := a.TwoFactor.RecoveryLeft(context.Background(), user.ID); left != mfa.RecoveryCodes {
		t.Fatalf("codes left = %d", left)
	}

	// Turning it off needs the password, and takes the recovery codes with it.
	b.submit("/settings/sicurezza", "/settings/security/totp/disable", url.Values{"password": {"nope"}})
	if st, _ := a.TwoFactor.Get(context.Background(), user.ID); !st.TOTPOn {
		t.Fatal("a wrong password must not turn it off")
	}
	b.submit("/settings/sicurezza", "/settings/security/totp/disable", url.Values{"password": {"right-password"}})
	if st, _ := a.TwoFactor.Get(context.Background(), user.ID); st.TOTPOn || st.TOTPSecret != "" {
		t.Fatal("should be off, with no secret kept")
	}
	if left, _ := a.TwoFactor.RecoveryLeft(context.Background(), user.ID); left != 0 {
		t.Fatalf("codes left = %d, want 0", left)
	}
}

func TestSecuritySetUpEmailAndNewCodes(t *testing.T) {
	a, fs, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	logIn(b, "carlo@example.test", "right-password")

	b.submit("/settings/sicurezza", "/settings/security/email/start", url.Values{})
	g := sixDigits.FindStringSubmatch(fs.wait(t, 1)[0].text)
	if code, _, _ := b.get("/settings/security/email"); code != 200 {
		t.Fatalf("code page: %d", code)
	}
	if code, _, _ := b.submit("/settings/security/email", "/settings/security/email/confirm", url.Values{"code": {"000000"}, "password": {"right-password"}}); code != 422 && g[1] != "000000" {
		t.Fatalf("a wrong code: %d", code)
	}
	code, _, body := b.submit("/settings/security/email", "/settings/security/email/confirm", url.Values{"code": {g[1]}, "password": {"right-password"}})
	if code != 200 || strings.Count(body, "<li>") < mfa.RecoveryCodes {
		t.Fatalf("the codes page: %d", code)
	}
	if st, _ := a.TwoFactor.Get(context.Background(), user.ID); !st.EmailOn {
		t.Fatal("email step should be on")
	}

	// New codes void the old ones.
	old := a.recoveryHashes(t, user.ID)
	b.submit("/settings/sicurezza", "/settings/security/codes", url.Values{"password": {"wrong"}})
	if len(old) != len(a.recoveryHashes(t, user.ID)) || old[0] != a.recoveryHashes(t, user.ID)[0] {
		t.Fatal("a wrong password must not change the codes")
	}
	b.submit("/settings/sicurezza", "/settings/security/codes", url.Values{"password": {"right-password"}})
	if now := a.recoveryHashes(t, user.ID); len(now) != mfa.RecoveryCodes || now[0] == old[0] {
		t.Fatal("the codes should be new")
	}
}

func (a *App) recoveryHashes(t *testing.T, userID int64) []string {
	t.Helper()
	rows, err := a.Store.DB.Query(`SELECT hash FROM recovery_codes WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		_ = rows.Scan(&h)
		out = append(out, h)
	}
	return out
}

func TestSecurityPageIsForSignedInPeople(t *testing.T) {
	_, _, b := mfaApp(t)
	for _, p := range []string{"/settings/sicurezza", "/settings/security/totp", "/settings/security/email"} {
		if code, loc, _ := b.get(p); code != 303 || loc != "/login" {
			t.Errorf("%s: %d -> %q", p, code, loc)
		}
	}
}

func TestCodeMailEscapesAndCarriesTheCode(t *testing.T) {
	subject, text, body := codeMail("it", "Carlo <b>", "123456")
	if subject == "" || !strings.Contains(text, "123456") || !strings.Contains(body, "123456") || strings.Contains(body, "<b>") {
		t.Fatalf("subject %q text %q", subject, text)
	}
}

func TestMFAReserveIsAtomicAndDecays(t *testing.T) {
	a, _, _ := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	ctx := context.Background()
	now := time.Now()

	// Twenty tries at once: only five are let through, and the step is locked after them.
	var wg sync.WaitGroup
	var mu sync.Mutex
	got := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := a.TwoFactor.Reserve(ctx, user.ID, now); err == nil && ok {
				mu.Lock()
				got++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if got != 5 {
		t.Fatalf("tries let through = %d, want 5", got)
	}
	if ok, _ := a.TwoFactor.Reserve(ctx, user.ID, now); ok {
		t.Fatal("the step should be locked")
	}
	// When the lock has ended the count starts again: one mistake does not lock for good.
	later := now.Add(6 * time.Minute)
	if ok, _ := a.TwoFactor.Reserve(ctx, user.ID, later); !ok {
		t.Fatal("after the lock a try should be allowed")
	}
	if st, _ := a.TwoFactor.Get(ctx, user.ID); st.Failed != 1 || st.Locked(later) {
		t.Fatalf("after the lock: failed=%d locked=%v", st.Failed, st.Locked(later))
	}
}

func TestSecurityTurningOnNeedsThePassword(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	logIn(b, "carlo@example.test", "right-password")
	b.get("/settings/security/totp")
	st, _ := a.TwoFactor.Get(context.Background(), user.ID)
	secret, _ := a.openTOTP(st)
	good, _ := mfa.CodeAt(secret, time.Now())
	for _, pw := range []string{"", "wrong"} {
		if code, _, _ := b.submit("/settings/security/totp/confirm", "/settings/security/totp/confirm", url.Values{"code": {good}, "password": {pw}}); code != 422 {
			t.Fatalf("password %q: %d, want 422", pw, code)
		}
	}
	if st, _ := a.TwoFactor.Get(context.Background(), user.ID); st.TOTPOn {
		t.Fatal("a session alone must not turn the second step on")
	}
}

func TestPasswordResetClearsTheSecondStep(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "old-password")
	totpOn(t, a, user.ID)
	codes, _ := mfa.NewRecoveryCodes(2)
	_ = a.TwoFactor.ReplaceRecoveryCodes(context.Background(), user.ID, codes)
	_ = a.PasswordResets.Create(context.Background(), user.ID, resetHash("tok"), time.Now().Add(time.Hour), 0)
	if code, _, _ := b.submit("/reset/tok", "/reset/tok", url.Values{"password": {"a-new-password"}}); code != 303 {
		t.Fatalf("reset: %d", code)
	}
	if st, _ := a.TwoFactor.Get(context.Background(), user.ID); st.Enabled() {
		t.Fatal("the mailbox proved the account is theirs: the second step goes")
	}
	if left, _ := a.TwoFactor.RecoveryLeft(context.Background(), user.ID); left != 0 {
		t.Fatalf("recovery codes left = %d", left)
	}
	if code, loc := logIn(newBrowser(t, a), "carlo@example.test", "a-new-password"); code != 303 || loc != "/" {
		t.Fatalf("sign-in after the reset: %d -> %q", code, loc)
	}
}

func TestMFAPendingSignInEndsWithAPasswordChange(t *testing.T) {
	a, _, b := mfaApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "old-password")
	secret := totpOn(t, a, user.ID)
	logIn(b, "carlo@example.test", "old-password")
	hash, _ := auth.HashPassword("a-new-password")
	if err := a.Users.UpdatePassword(context.Background(), user.ID, hash); err != nil {
		t.Fatal(err)
	}
	good, _ := mfa.CodeAt(secret, time.Now())
	if code, loc, _ := code2fa(b, "totp", good); code == 303 && loc == "/" {
		t.Fatal("a sign-in opened with the old password must not finish")
	}
	if signedIn(b) {
		t.Fatal("must stay signed out")
	}
}

func TestMFAAfterTheProviderLinkStep(t *testing.T) {
	a, p, b := ssoApp(t, false)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	box, _ := stores.NewBox("a-secret-key-of-enough-length")
	a.StoreBox = box
	totpOn(t, a, user.ID)
	if status, redirect := ssoTrip(t, b, p); status != 303 || redirect != "/auth/oidc/link" {
		t.Fatalf("callback: %d -> %q", status, redirect)
	}
	if code, loc, _ := b.submit("/auth/oidc/link", "/auth/oidc/link", url.Values{"password": {"right-password"}}); code != 303 || loc != "/login/2fa" {
		t.Fatalf("link with the password: %d -> %q, want the second step", code, loc)
	}
	if signedIn(b) {
		t.Fatal("the password alone must not finish the sign-in")
	}
}

func TestIPv6ClientsShareTheirNetworkBucket(t *testing.T) {
	if bucketIP("2001:db8:1:2::1") != bucketIP("2001:db8:1:2:ffff::9") {
		t.Fatal("two addresses of one /64 are one client")
	}
	if bucketIP("2001:db8:1:2::1") == bucketIP("2001:db8:1:3::1") {
		t.Fatal("another /64 is another client")
	}
	if bucketIP("203.0.113.9") != "203.0.113.9" {
		t.Fatal("IPv4 stays as it is")
	}
}
