package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/updates"
)

// newUpdatesApp is an app built as v2.0.0 whose versions.json says v2.1.0 is
// out; hits counts the requests for it.
func newUpdatesApp(t *testing.T) (*App, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"stable": {"version": "v2.1.0", "published": "2026-10-14T09:12:00Z"}}`))
	}))
	t.Cleanup(srv.Close)
	a := newTestApp(t)
	a.Updates = updates.New(db.NewInstanceSettings(a.Store.DB), "v2.0.0", true)
	a.Updates.SetURL(srv.URL)
	return a, &hits
}

// signIn returns a browser signed in as a new user with email.
func signIn(t *testing.T, a *App, email string) *browser {
	t.Helper()
	mustCreateTestUser(t, a, email, "password-123")
	b := newBrowser(t, a)
	if code, _ := b.post("/login", "/login", url.Values{"email": {email}, "password": {"password-123"}}); code != http.StatusSeeOther {
		t.Fatalf("sign in as %s: %d", email, code)
	}
	return b
}

// Only the owner is told about a newer release, and only once a check has
// run; page loads never ask GitHub (#167).
func TestUpdateOwnerOnly(t *testing.T) {
	a, hits := newUpdatesApp(t)
	a.Cfg.OwnerEmail = "owner@example.test"
	owner := signIn(t, a, "owner@example.test")
	other := signIn(t, a, "other@example.test")

	if _, _, page := owner.get("/"); strings.Contains(page, "data-update-card") || *hits != 0 {
		t.Fatalf("before any check: card shown or GitHub asked (hits=%d)", *hits)
	}
	if code, _ := owner.post("/admin/panel", "/admin/panel/updates/check", nil); code != http.StatusSeeOther || *hits != 1 {
		t.Fatalf("check now: %d, hits=%d", code, *hits)
	}

	_, _, page := owner.get("/")
	for _, want := range []string{`data-update-card="v2.1.0"`, `data-update-dot="v2.1.0"`, `id="update-howto"`, "Nutreek v2.1.0 is available", "docker compose pull", "releases/tag/v2.1.0"} {
		if !strings.Contains(page, want) {
			t.Errorf("the owner's page lacks %q", want)
		}
	}
	if _, _, panel := owner.get("/admin/panel"); !strings.Contains(panel, "v2.1.0 is available") || !strings.Contains(panel, "Installed version: v2.0.0") {
		t.Error("Administration does not show the newer release")
	}
	if code, _, howto := owner.get("/admin/update"); code != http.StatusOK || !strings.Contains(howto, "Update to v2.1.0") || !strings.Contains(howto, "docker pull ghcr.io/easly1989/nutreek:latest") {
		t.Errorf("GET /admin/update: %d", code)
	}

	_, _, page = other.get("/")
	if strings.Contains(page, "data-update") || strings.Contains(page, "update-howto") {
		t.Error("another user is told about the update")
	}
	for _, p := range []string{"/admin/update", "/admin/panel"} {
		if code, _, _ := other.get(p); code == http.StatusOK {
			t.Errorf("another user reaches %s", p)
		}
	}
	if code, _ := other.post("/", "/admin/panel/updates/check", nil); code == http.StatusSeeOther {
		t.Error("another user ran a check")
	}
	if *hits != 1 {
		t.Errorf("page loads asked GitHub: hits=%d", *hits)
	}
}

// dailyOff matches the «Check every day» switch turned off.
var dailyOff = regexp.MustCompile(`id="update-daily"[^>]*data-autosubmit>`)

// The owner's switch turns the check off, and back on at once (#167).
func TestUpdateSwitch(t *testing.T) {
	a, hits := newUpdatesApp(t)
	a.Cfg.OwnerEmail = "owner@example.test"
	owner := signIn(t, a, "owner@example.test")

	owner.post("/admin/panel", "/admin/panel/updates/toggle", nil) // off
	owner.post("/admin/panel", "/admin/panel/updates/check", nil)
	if _, _, panel := owner.get("/admin/panel"); *hits != 0 || !dailyOff.MatchString(panel) || strings.Contains(panel, "Check now") {
		t.Fatalf("turned off: hits=%d or the switch still on", *hits)
	}
	owner.post("/admin/panel", "/admin/panel/updates/toggle", nil) // on: checks at once
	if _, _, panel := owner.get("/admin/panel"); *hits != 1 || !strings.Contains(panel, "v2.1.0 is available") {
		t.Errorf("turned back on: hits=%d", *hits)
	}
}

// The demo build has no owner and no update check: no route answers.
func TestUpdateAbsentFromDemo(t *testing.T) {
	a := newDemoApp(t, DemoLimits{})
	b := newBrowser(t, a)
	enterAs(t, a, b, "plan")
	if code, _, _ := b.get("/admin/update"); code == http.StatusOK {
		t.Error("the demo serves /admin/update")
	}
}
