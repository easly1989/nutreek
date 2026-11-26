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

func TestSSOSignsInAnExistingAccount(t *testing.T) {
	a, p, b := ssoApp(t, false)
	user := mustCreateTestUser(t, a, "carlo@example.test", "irrelevant")
	status, redirect := ssoTrip(t, b, p)
	if status != 303 || redirect != "/" {
		t.Fatalf("callback: %d -> %q", status, redirect)
	}
	if code, loc, _ := b.get("/login"); code != 303 || loc != "/" {
		t.Fatalf("the person should be signed in now: /login gave %d -> %q", code, loc)
	}
	if n := countUsers(t, a); n != 1 || user.ID == 0 {
		t.Fatalf("no account should be added, users = %d", n)
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
	// The random password cannot be used to sign in.
	b2 := newBrowser(t, a)
	if code, _, _ := b2.submit("/login", "/login", url.Values{"email": {"carlo@example.test"}, "password": {""}}); code != 401 {
		t.Fatalf("a password sign-in must fail, got %d", code)
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
	a, p, b := ssoApp(t, false)
	mustCreateTestUser(t, a, "carlo@example.test", "irrelevant")

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
	// The state is spent: the right one no longer works either.
	_, loc, _ = b.get("/auth/oidc/start")
	authCode, state := p.Visit(t, loc)
	if code, redirect, _ := b.get("/auth/oidc/callback?code=" + authCode + "&state=" + state); code != 303 || redirect != "/" {
		t.Fatalf("first use: %d -> %q", code, redirect)
	}
	b2 := newBrowser(t, a)
	if code, _, _ := b2.get("/auth/oidc/callback?code=" + authCode + "&state=" + state); code != 401 {
		t.Fatalf("a replay from another browser should be refused, got %d", code)
	}
	// The provider says no.
	if code, _, _ := b.get("/auth/oidc/callback?error=access_denied&state=" + state); code != 401 {
		t.Fatalf("an error from the provider should be refused, got %d", code)
	}
}

func countUsers(t *testing.T, a *App) int {
	t.Helper()
	var n int
	if err := a.Store.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
