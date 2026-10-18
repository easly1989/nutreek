package app

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/demo"
)

// currentAs matches the band menu's entry of whom the page is signed in as.
var currentAs = regexp.MustCompile(`name="user_id" value="(\d+)"[^>]*aria-current="true"`)

// signedInAs is whom b's band says they are, 0 when it shows no menu.
func signedInAs(t *testing.T, b *browser) int64 {
	t.Helper()
	_, _, page := b.get("/")
	m := currentAs.FindStringSubmatch(page)
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// "View as" switches to every account of the visitor's group and back, with
// no password, landing the coach on the panel (#187).
func TestDemoViewAs(t *testing.T) {
	ctx := context.Background()
	a := newDemoApp(t, DemoLimits{})
	for _, as := range []string{demo.AsCoach, demo.AsClient} {
		t.Run(as, func(t *testing.T) {
			b := newBrowser(t, a)
			visitor, _ := enterAs(t, a, b, as)
			group, err := a.Demo.Group(ctx, visitor)
			if err != nil || len(group) != 4 || group[0].ID != visitor || !group[0].Visitor {
				t.Fatalf("group %+v (%v), want 4 with the visitor first", group, err)
			}
			if got := signedInAs(t, b); got != visitor {
				t.Fatalf("signed in as %d, want the visitor %d", got, visitor)
			}
			for _, m := range append(group[1:], group[0]) {
				code, loc := b.post("/", "/demo/as", url.Values{"user_id": {strconv.FormatInt(m.ID, 10)}})
				want := "/"
				if m.Coach {
					want = "/coach"
				}
				if code != http.StatusSeeOther || loc != want {
					t.Errorf("to %s: %d to %q, want %q", m.Name, code, loc, want)
				}
				if got := signedInAs(t, b); got != m.ID {
					t.Errorf("signed in as %d, want %s %d", got, m.Name, m.ID)
				}
			}
		})
	}
}

// Nobody outside the visitor's group: another group's user, a made-up id and
// no id are 404, and the session stays whom it was (#187).
func TestDemoViewAsGroupOnly(t *testing.T) {
	a := newDemoApp(t, DemoLimits{})
	alice, bob := newBrowser(t, a), newBrowser(t, a)
	aliceID, _ := enterAs(t, a, alice, demo.AsCoach)
	bobID, _ := enterAs(t, a, bob, demo.AsClient)
	bobFree, _ := freeMember(t, a, bobID)
	for _, id := range []string{strconv.FormatInt(bobID, 10), strconv.FormatInt(bobFree, 10), "999999", "x", ""} {
		if code, _ := alice.post("/", "/demo/as", url.Values{"user_id": {id}}); code != http.StatusNotFound {
			t.Errorf("Alice as %q: %d, want 404", id, code)
		}
	}
	// A form from elsewhere, with no CSRF token, switches nobody.
	clientID := strconv.FormatInt(clientOf(t, a, aliceID), 10)
	req, _ := http.NewRequest(http.MethodPost, alice.srv.URL+"/demo/as", strings.NewReader("user_id="+clientID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", alice.srv.URL)
	resp, err := alice.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("with no CSRF token: %d", resp.StatusCode)
	}
	if got := signedInAs(t, alice); got != aliceID {
		t.Errorf("Alice is now %d, want %d", got, aliceID)
	}

	// Signed out, the switch is no way in.
	out := newBrowser(t, a)
	if code, loc := out.post("/", "/demo/as", url.Values{"user_id": {strconv.FormatInt(bobID, 10)}}); code != http.StatusSeeOther || loc != "/" {
		t.Errorf("signed out: %d to %q, want back to the entry", code, loc)
	}
	if code, _, _ := out.get("/coach"); code == http.StatusOK {
		t.Error("signed out, the switch signed someone in")
	}
}

// clientOf returns one of coach's clients.
func clientOf(t *testing.T, a *App, coach int64) int64 {
	t.Helper()
	clients, err := a.Coach.ListClients(context.Background(), coach)
	if err != nil || len(clients) == 0 {
		t.Fatalf("clients of %d: %d %v", coach, len(clients), err)
	}
	return clients[0].ClientID
}

// Someone following their own plan is alone in their group: no menu.
func TestDemoViewAsAlone(t *testing.T) {
	a := newDemoApp(t, DemoLimits{})
	b := newBrowser(t, a)
	enterAs(t, a, b, demo.AsPlan)
	if _, _, page := b.get("/"); strings.Contains(page, `action="/demo/as"`) {
		t.Error("the band offers to switch with nobody to switch to")
	}
}
