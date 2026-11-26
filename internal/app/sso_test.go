package app

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/sso"
	"github.com/amon2126/nutreek/internal/sso/ssotest"
)

// ssoApp is an app wired to a fake provider.
func ssoApp(t *testing.T, autoCreate bool) (*App, *ssotest.Server, *browser) {
	t.Helper()
	p := ssotest.New(t)
	a := newTestApp(t)
	a.Cfg = &config.Config{OIDCAutoCreate: autoCreate}
	a.SSO = sso.New(sso.Config{Issuer: p.URL, ClientID: p.ClientID, ClientSecret: p.ClientSecret,
		RedirectURL: "http://app.test/auth/oidc/callback", Name: "Authelia"}, nil)
	return a, p, newBrowser(t, a)
}

// ssoTrip runs the whole trip and returns the callback's status and redirect.
func ssoTrip(t *testing.T, b *browser, p *ssotest.Server) (int, string) {
	t.Helper()
	code, loc, _ := b.get("/auth/oidc/start")
	if code != 303 {
		t.Fatalf("start: %d", code)
	}
	authCode, state := p.Visit(t, loc)
	status, redirect, _ := b.get("/auth/oidc/callback?code=" + authCode + "&state=" + url.QueryEscape(state))
	return status, redirect
}

func countUsers(t *testing.T, a *App) int {
	t.Helper()
	var n int
	if err := a.Store.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func linkedTo(t *testing.T, a *App, p *ssotest.Server, sub string) int64 {
	t.Helper()
	id, err := a.Identities.UserID(context.Background(), p.URL, sub)
	if err != nil {
		return 0
	}
	return id
}

func TestSSOLoginButtonOnlyWithAProvider(t *testing.T) {
	a, _, b := ssoApp(t, false)
	if _, _, body := b.get("/login"); !strings.Contains(body, "/auth/oidc/start") || !strings.Contains(body, "Authelia") {
		t.Fatal("the login page should offer the provider")
	}
	a.SSO = nil
	b2 := newBrowser(t, a)
	if _, _, body := b2.get("/login"); strings.Contains(body, "/auth/oidc/start") {
		t.Fatal("no provider, no button")
	}
	if code, _, _ := b2.get("/auth/oidc/start"); code != 404 {
		t.Fatalf("the route should not exist without a provider, got %d", code)
	}
}

// An account that already exists is linked only by someone who knows its password:
// registration does not verify emails, so anyone may have made it.
func TestSSOLinksAnExistingAccountWithItsPassword(t *testing.T) {
	a, p, b := ssoApp(t, false)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")

	status, redirect := ssoTrip(t, b, p)
	if status != 303 || redirect != "/auth/oidc/link" {
		t.Fatalf("callback: %d -> %q", status, redirect)
	}
	if code, _, body := b.get("/auth/oidc/link"); code != 200 || !strings.Contains(body, `name="password"`) {
		t.Fatalf("the link page should ask for the password, got %d", code)
	}
	// Someone who does not know the password is not let in, and nothing is linked.
	if code, _, _ := b.submit("/auth/oidc/link", "/auth/oidc/link", url.Values{"password": {"guess"}}); code != 401 {
		t.Fatalf("a wrong password should be refused, got %d", code)
	}
	if code, _, _ := b.get("/login"); code != 200 {
		t.Fatalf("still signed out expected, /login gave %d", code)
	}
	if id := linkedTo(t, a, p, "u1"); id != 0 {
		t.Fatal("nothing should be linked after a wrong password")
	}
	// The right one links and signs in.
	if code, loc, _ := b.submit("/auth/oidc/link", "/auth/oidc/link", url.Values{"password": {"right-password"}}); code != 303 || loc != "/" {
		t.Fatalf("link: %d -> %q", code, loc)
	}
	if id := linkedTo(t, a, p, "u1"); id != user.ID {
		t.Fatalf("linked to %d, want %d", id, user.ID)
	}
	if code, loc, _ := b.get("/login"); code != 303 || loc != "/" {
		t.Fatalf("the person should be signed in: /login gave %d -> %q", code, loc)
	}

	// From now on the identity is enough, even if the email at the provider changes.
	p.Email = "changed@example.test"
	b2 := newBrowser(t, a)
	if status, redirect := ssoTrip(t, b2, p); status != 303 || redirect != "/" {
		t.Fatalf("second sign-in: %d -> %q", status, redirect)
	}
	if n := countUsers(t, a); n != 1 {
		t.Fatalf("no account should be added, users = %d", n)
	}
}

// Someone else who gets the same email at the provider is a different identity: they
// meet the password step, not the account.
func TestSSOAnotherIdentityWithTheSameEmailCannotTakeTheAccount(t *testing.T) {
	a, p, b := ssoApp(t, false)
	user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	if err := a.Identities.Link(context.Background(), user.ID, p.URL, "u1"); err != nil {
		t.Fatal(err)
	}
	p.Sub = "u2"
	status, redirect := ssoTrip(t, b, p)
	if status != 303 || redirect != "/auth/oidc/link" {
		t.Fatalf("callback: %d -> %q", status, redirect)
	}
	if code, _, _ := b.get("/login"); code != 200 {
		t.Fatalf("they must not be signed in, /login gave %d", code)
	}
}

func TestSSOLinkPageNeedsATrip(t *testing.T) {
	_, _, b := ssoApp(t, false)
	if code, loc, _ := b.get("/auth/oidc/link"); code != 303 || loc != "/login" {
		t.Fatalf("got %d -> %q", code, loc)
	}
	if code, loc, _ := b.submit("/login", "/auth/oidc/link", url.Values{"password": {"x"}}); code != 303 || loc != "/login" {
		t.Fatalf("got %d -> %q", code, loc)
	}
}

func TestSSOUnknownEmail(t *testing.T) {
	a, p, b := ssoApp(t, false)
	if status, _ := ssoTrip(t, b, p); status != 401 {
		t.Fatalf("an unknown email without auto-create should be refused, got %d", status)
	}
	if n := countUsers(t, a); n != 0 {
		t.Fatalf("no account should be created, users = %d", n)
	}
}

func TestSSOAutoCreate(t *testing.T) {
	a, p, b := ssoApp(t, true)
	status, redirect := ssoTrip(t, b, p)
	if status != 303 || redirect != "/onboarding" {
		t.Fatalf("callback: %d -> %q", status, redirect)
	}
	u, err := a.Users.GetByEmail(context.Background(), "carlo@example.test")
	if err != nil || u.DisplayName != "Carlo" {
		t.Fatalf("account not created: %+v, %v", u, err)
	}
	if linkedTo(t, a, p, "u1") != u.ID {
		t.Fatal("the new account should be linked")
	}
	// The random password cannot be used to sign in.
	b2 := newBrowser(t, a)
	if code, _, _ := b2.submit("/login", "/login", url.Values{"email": {"carlo@example.test"}, "password": {""}}); code != 401 {
		t.Fatalf("a password sign-in must fail, got %d", code)
	}
	// And the person comes back by identity.
	b3 := newBrowser(t, a)
	if status, redirect := ssoTrip(t, b3, p); status != 303 || redirect != "/" {
		t.Fatalf("second sign-in: %d -> %q", status, redirect)
	}
}

func TestSSOAutoCreateKeepsTheNameWithinTheLimit(t *testing.T) {
	a, p, b := ssoApp(t, true)
	p.Name = strings.Repeat("é", 100)
	if status, _ := ssoTrip(t, b, p); status != 303 {
		t.Fatalf("got %d", status)
	}
	u, err := a.Users.GetByEmail(context.Background(), "carlo@example.test")
	if err != nil || len(u.DisplayName) > 64 || len(u.DisplayName) == 0 {
		t.Fatalf("name = %q (%d bytes), %v", u.DisplayName, len(u.DisplayName), err)
	}
}

func TestSSOUnverifiedEmailIsRefused(t *testing.T) {
	a, p, b := ssoApp(t, true)
	p.Verified = false
	if status, _ := ssoTrip(t, b, p); status != 401 {
		t.Fatalf("got %d", status)
	}
	if n := countUsers(t, a); n != 0 {
		t.Fatalf("users = %d", n)
	}
}

func TestSSOCallbackNeedsItsOwnState(t *testing.T) {
	a, p, b := ssoApp(t, true)

	// No trip started.
	if code, _, _ := b.get("/auth/oidc/callback?code=good&state=x"); code != 401 {
		t.Fatalf("a callback with no start should be refused, got %d", code)
	}
	// Another state.
	_, loc, _ := b.get("/auth/oidc/start")
	authCode, _ := p.Visit(t, loc)
	if code, _, _ := b.get("/auth/oidc/callback?code=" + authCode + "&state=forged"); code != 401 {
		t.Fatalf("a forged state should be refused, got %d", code)
	}
	// The state is spent: the right one works once and never again.
	_, loc, _ = b.get("/auth/oidc/start")
	authCode, state := p.Visit(t, loc)
	if code, _, _ := b.get("/auth/oidc/callback?code=" + authCode + "&state=" + state); code != 303 {
		t.Fatalf("first use: %d", code)
	}
	b2 := newBrowser(t, a)
	if code, _, _ := b2.get("/auth/oidc/callback?code=" + authCode + "&state=" + state); code != 401 {
		t.Fatalf("a replay from another browser should be refused, got %d", code)
	}
	if code, _, _ := b.get("/auth/oidc/callback?code=" + authCode + "&state=" + state); code != 401 {
		t.Fatalf("a replay in the same browser should be refused, got %d", code)
	}
	// The provider says no.
	b3 := newBrowser(t, a)
	_, loc, _ = b3.get("/auth/oidc/start")
	_, state = p.Visit(t, loc)
	if code, _, _ := b3.get("/auth/oidc/callback?error=access_denied&state=" + state); code != 401 {
		t.Fatalf("an error from the provider should be refused, got %d", code)
	}
}
