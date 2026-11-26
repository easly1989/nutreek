package app

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/config"
)

type sentMail struct{ to, subject, text, html string }

// fakeSender records what would be mailed.
type fakeSender struct {
	mu   sync.Mutex
	sent []sentMail
}

func (f *fakeSender) Send(_ context.Context, to, subject, text, html string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{to, subject, text, html})
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// wait returns the mail once n have arrived: the app sends after it has answered.
func (f *fakeSender) wait(t *testing.T, n int) []sentMail {
	t.Helper()
	for i := 0; i < 100; i++ {
		if f.count() >= n {
			f.mu.Lock()
			defer f.mu.Unlock()
			return append([]sentMail(nil), f.sent...)
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("expected %d mails, got %d", n, f.count())
	return nil
}

func recoveryApp(t *testing.T) (*App, *fakeSender, *browser) {
	t.Helper()
	a := newTestApp(t)
	a.Cfg = &config.Config{BaseURL: "https://n.test"}
	fs := &fakeSender{}
	a.Mailer = fs
	return a, fs, newBrowser(t, a)
}

var resetLink = regexp.MustCompile(`https://n\.test(/reset/[A-Za-z0-9_-]+)`)

func tokenPath(t *testing.T, m sentMail) string {
	t.Helper()
	g := resetLink.FindStringSubmatch(m.text)
	if g == nil {
		t.Fatalf("no reset link in %q", m.text)
	}
	return g[1]
}

func forgot(b *browser, email string) (int, string) {
	b.t.Helper()
	code, loc, _ := b.submit("/forgot", "/forgot", url.Values{"email": {email}})
	return code, loc
}

func TestRecoveryOnlyWithAMailer(t *testing.T) {
	a, _, b := recoveryApp(t)
	if _, _, body := b.get("/login"); !strings.Contains(body, `href="/forgot"`) {
		t.Fatal("the login page should offer the link when email is set up")
	}
	if code, _, _ := b.get("/forgot"); code != 200 {
		t.Fatalf("/forgot: %d", code)
	}
	a.Mailer = nil
	b2 := newBrowser(t, a)
	if _, _, body := b2.get("/login"); strings.Contains(body, `href="/forgot"`) {
		t.Fatal("no mailer, no link")
	}
	if code, _, _ := b2.get("/forgot"); code != 404 {
		t.Fatalf("/forgot without a mailer: %d", code)
	}
}

func TestRecoveryAnswersTheSameForEveryone(t *testing.T) {
	a, fs, b := recoveryApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "old-password")

	code1, loc1 := forgot(b, "carlo@example.test")
	code2, loc2 := forgot(b, "nobody@example.test")
	if code1 != 303 || loc1 != "/forgot/sent" || code2 != code1 || loc2 != loc1 {
		t.Fatalf("known: %d -> %q, unknown: %d -> %q", code1, loc1, code2, loc2)
	}
	mails := fs.wait(t, 1)
	time.Sleep(300 * time.Millisecond) // the unknown address must not send anything
	if fs.count() != 1 || mails[0].to != "carlo@example.test" {
		t.Fatalf("mails = %+v", mails)
	}
	if code, _, body := b.get("/forgot/sent"); code != 200 || !strings.Contains(body, "1") {
		t.Fatalf("sent page: %d", code)
	}
}

func TestRecoveryWholeFlow(t *testing.T) {
	a, fs, b := recoveryApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "old-password")
	forgot(b, "carlo@example.test")
	path := tokenPath(t, fs.wait(t, 1)[0])

	// What is stored cannot be used as the link.
	var stored string
	if err := a.Store.DB.QueryRow(`SELECT token_hash FROM password_resets`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(path, stored) {
		t.Fatal("the token must be stored hashed")
	}

	if code, _, body := b.get(path); code != 200 || !strings.Contains(body, `name="password"`) {
		t.Fatalf("form: %d", code)
	}
	// A short password is corrected without spending the link.
	if code, _, _ := b.submit(path, path, url.Values{"password": {"short"}}); code != 422 {
		t.Fatalf("short password: %d", code)
	}
	if code, _, _ := b.get(path); code != 200 {
		t.Fatalf("the link should still work: %d", code)
	}
	if code, loc, _ := b.submit(path, path, url.Values{"password": {"a-new-password"}}); code != 303 || loc != "/login" {
		t.Fatalf("reset: %d -> %q", code, loc)
	}
	// The link is spent.
	if code, _, _ := b.get(path); code != 410 {
		t.Fatalf("a used link: %d", code)
	}
	if code, _, _ := b.submit("/forgot", path, url.Values{"password": {"another-password"}}); code != 410 {
		t.Fatalf("a used link, posted: %d", code)
	}
	// The new password works and the old one does not.
	if code, _, _ := b.submit("/login", "/login", url.Values{"email": {"carlo@example.test"}, "password": {"old-password"}}); code != 401 {
		t.Fatalf("old password: %d", code)
	}
	if code, loc, _ := b.submit("/login", "/login", url.Values{"email": {"carlo@example.test"}, "password": {"a-new-password"}}); code != 303 || loc != "/" {
		t.Fatalf("new password: %d -> %q", code, loc)
	}
}

func TestRecoveryExpiredAndReplacedLinks(t *testing.T) {
	a, _, b := recoveryApp(t)
	user := mustCreateTestUser(t, a, "carlo@example.test", "old-password")
	ctx := context.Background()

	if err := a.PasswordResets.Create(ctx, user.ID, resetHash("expired"), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := b.get("/reset/expired"); code != 410 {
		t.Fatalf("an expired link: %d", code)
	}
	if code, _, _ := b.get("/reset/never-issued"); code != 410 {
		t.Fatalf("an unknown link: %d", code)
	}
	// Asking again replaces the earlier link.
	_ = a.PasswordResets.Create(ctx, user.ID, resetHash("first"), time.Now().Add(time.Hour))
	_ = a.PasswordResets.Create(ctx, user.ID, resetHash("second"), time.Now().Add(time.Hour))
	if code, _, _ := b.get("/reset/first"); code != 410 {
		t.Fatalf("the replaced link: %d", code)
	}
	if code, _, _ := b.get("/reset/second"); code != 200 {
		t.Fatalf("the new link: %d", code)
	}
}

func TestRecoveryLimitsAndBadEmail(t *testing.T) {
	_, _, b := recoveryApp(t)
	if code, _, _ := b.submit("/forgot", "/forgot", url.Values{"email": {"not an email"}}); code != 422 {
		t.Fatalf("bad email: %d", code)
	}
	var last int
	for i := 0; i < 6; i++ {
		last, _ = forgot(b, "x"+string(rune('a'+i))+"@example.test")
	}
	if last != 429 {
		t.Fatalf("the 6th request in a minute should be refused, got %d", last)
	}
}

func TestRecoveryOneMailAMinutePerAddress(t *testing.T) {
	a, fs, b := recoveryApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "old-password")
	forgot(b, "carlo@example.test")
	forgot(b, "Carlo@Example.test")
	fs.wait(t, 1)
	time.Sleep(300 * time.Millisecond)
	if fs.count() != 1 {
		t.Fatalf("mails = %d, want 1", fs.count())
	}
}

// Changing the password ends the other sessions of the account.
func TestPasswordChangeEndsOtherSessions(t *testing.T) {
	a, fs, phone := recoveryApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "old-password")
	if code, _, _ := phone.submit("/login", "/login", url.Values{"email": {"carlo@example.test"}, "password": {"old-password"}}); code != 303 {
		t.Fatalf("login: %d", code)
	}
	if code, _, _ := phone.get("/tu"); code != 200 {
		t.Fatalf("signed in expected: %d", code)
	}

	laptop := newBrowser(t, a)
	forgot(laptop, "carlo@example.test")
	path := tokenPath(t, fs.wait(t, 1)[0])
	if code, _, _ := laptop.submit(path, path, url.Values{"password": {"a-new-password"}}); code != 303 {
		t.Fatalf("reset: %d", code)
	}
	if code, loc, _ := phone.get("/tu"); code != 303 || loc != "/login" {
		t.Fatalf("the old session should be over: %d -> %q", code, loc)
	}
}

// The person who changes the password in Settings stays signed in; their other sessions do not.
func TestSettingsPasswordChangeKeepsThisSession(t *testing.T) {
	a, _, here := recoveryApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "old-password")
	there := newBrowser(t, a)
	for _, b := range []*browser{here, there} {
		if code, _, _ := b.submit("/login", "/login", url.Values{"email": {"carlo@example.test"}, "password": {"old-password"}}); code != 303 {
			t.Fatalf("login: %d", code)
		}
		b.get("/tu") // the session takes its stamp
	}
	form := url.Values{"current_password": {"old-password"}, "new_password": {"a-new-password"}, "confirm_password": {"a-new-password"}}
	if code, _, _ := here.submit("/settings/account", "/settings/password", form); code != 303 {
		t.Fatalf("change: %d", code)
	}
	if code, _, _ := here.get("/tu"); code != 200 {
		t.Fatalf("this session should go on: %d", code)
	}
	if code, loc, _ := there.get("/tu"); code != 303 || loc != "/login" {
		t.Fatalf("the other session should end: %d -> %q", code, loc)
	}
}

func TestResetMailInBothLanguages(t *testing.T) {
	for lang, want := range map[string]string{"en": "Reset your Nutreek password", "it": "Reimposta la password di Nutreek"} {
		subject, text, body := resetMail(lang, `Carlo <b>`, "https://n.test/reset/abc")
		if subject != want {
			t.Errorf("%s subject = %q", lang, subject)
		}
		if !strings.Contains(text, "https://n.test/reset/abc") || !strings.Contains(body, `href="https://n.test/reset/abc"`) {
			t.Errorf("%s: the link is missing", lang)
		}
		if strings.Contains(body, "<b>") || !strings.Contains(body, "&lt;b&gt;") {
			t.Errorf("%s: the name must be escaped in the HTML", lang)
		}
		if strings.Contains(text, "{{") || strings.Contains(body, "{{") {
			t.Errorf("%s: a placeholder is left", lang)
		}
	}
}
