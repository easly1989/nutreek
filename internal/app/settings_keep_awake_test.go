package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The setting to keep the screen on saves per user, the page marks the body
// when it is on, and the settings page carries the switch (#249).
func TestSettingsKeepAwake(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")

	post := func(form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/settings/keep-awake", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleSettingsKeepAwake)).ServeHTTP(w, requestWithUser(r, alice))
		return w
	}
	page := func() string {
		u, err := a.Users.GetByID(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodGet, "/settings", nil)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleSettingsPage)).ServeHTTP(w, requestWithUser(r, u))
		return w.Body.String()
	}

	if body := page(); !strings.Contains(body, `name="keep_awake"`) || strings.Contains(body, `data-keep-awake="1"`) || strings.Contains(body, ` checked`+" aria-label=") {
		t.Error("off by default: the switch is there, unchecked, and the body is not marked")
	}
	// On: the hidden 0 and the checked 1 are both posted.
	if w := post(url.Values{"keep_awake": {"0", "1"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("turn on: code %d", w.Code)
	}
	u, _ := a.Users.GetByID(ctx, alice.ID)
	if !u.KeepAwake {
		t.Fatal("the setting was not saved")
	}
	if body := page(); !strings.Contains(body, `data-keep-awake="1"`) || !strings.Contains(body, `class="toggle" checked`) {
		t.Error("on: the body must be marked and the switch checked")
	}
	// Off: only the hidden 0 is posted.
	if w := post(url.Values{"keep_awake": {"0"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("turn off: code %d", w.Code)
	}
	if u, _ := a.Users.GetByID(ctx, alice.ID); u.KeepAwake {
		t.Error("the setting was not turned off")
	}
	// Another user is not touched.
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	if u, _ := a.Users.GetByID(ctx, bob.ID); u.KeepAwake {
		t.Error("a new user starts with it off")
	}
}
