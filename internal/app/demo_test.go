package app

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/demo"
)

// browser is a test client that keeps cookies and does not follow redirects.
type browser struct {
	t   *testing.T
	srv *httptest.Server
	c   *http.Client
}

func newBrowser(t *testing.T, a *App) *browser {
	t.Helper()
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &browser{t: t, srv: srv, c: &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (b *browser) get(path string) (int, string, string) {
	b.t.Helper()
	resp, err := b.c.Get(b.srv.URL + path)
	if err != nil {
		b.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location"), string(body)
}

// welcome matches the notice set to open by itself.
var welcome = regexp.MustCompile(`<dialog id="demo-notice"[^>]*data-demo-welcome`)

var csrfField = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

// post sends a form with the CSRF token of the page at from.
func (b *browser) post(from, path string, form url.Values) (int, string) {
	b.t.Helper()
	code, loc, _ := b.submit(from, path, form)
	return code, loc
}

// submit is post that also returns the body.
func (b *browser) submit(from, path string, form url.Values) (int, string, string) {
	b.t.Helper()
	_, _, page := b.get(from)
	m := csrfField.FindStringSubmatch(page)
	if m == nil {
		b.t.Fatalf("no CSRF token on %s", from)
	}
	if form == nil {
		form = url.Values{}
	}
	form.Set("csrf_token", html.UnescapeString(m[1])) // base64 "+" comes escaped
	req, _ := http.NewRequest(http.MethodPost, b.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", b.srv.URL) // what a browser sends; nosurf checks it
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location"), string(body)
}

func (b *browser) setLang(lang string) {
	u, _ := url.Parse(b.srv.URL)
	b.c.Jar.SetCookies(u, []*http.Cookie{{Name: langCookie, Value: lang}})
}

// The normal build has no demo: sign-in and sign-up as usual, no /demo/session.
func TestDemoAbsentFromNormalBuild(t *testing.T) {
	b := newBrowser(t, newTestApp(t))
	for _, p := range []string{"/login", "/register"} {
		if code, _, _ := b.get(p); code != http.StatusOK {
			t.Errorf("GET %s: %d, want 200", p, code)
		}
	}
	if code, _ := b.post("/login", "/demo/session", nil); code != http.StatusNotFound {
		t.Errorf("POST /demo/session: %d, want 404", code)
	}
	if _, _, body := b.get("/"); strings.Contains(body, "/demo/session") {
		t.Error("the public home offers the demo")
	}
}

// newDemoApp is the app as cmd/nutreek/demo_on.go wires it; zero limits are off.
func newDemoApp(t *testing.T, limits DemoLimits) *App {
	t.Helper()
	a := newTestApp(t)
	if err := demo.Prepare(context.Background(), a.Store.DB); err != nil {
		t.Fatal(err)
	}
	db.ScopeCatalog(a.Store.DB, demo.Peers)
	a.Demo = demo.New(a.Store.DB, demo.Config{})
	a.DemoLimits = limits
	return a
}

func TestDemoEntry(t *testing.T) {
	a := newDemoApp(t, DemoLimits{})
	b := newBrowser(t, a)

	code, _, entry := b.get("/")
	if code != http.StatusOK || !strings.Contains(entry, `action="/demo/session"`) {
		t.Fatalf("GET /: %d, no demo button", code)
	}
	for _, p := range []string{`href="/login"`, `href="/register"`} {
		if strings.Contains(entry, p) {
			t.Errorf("the entry page links %s", p)
		}
	}
	for _, p := range []string{"/login", "/register"} {
		if code, _, _ := b.get(p); code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", p, code)
		}
	}
	if code, loc, _ := b.get("/recipes"); code != http.StatusSeeOther || loc != "/" {
		t.Errorf("signed-out /recipes: %d to %q, want the entry", code, loc)
	}

	b.setLang("it")
	if code, loc := b.post("/", "/demo/session", nil); code != http.StatusSeeOther || loc != "/" {
		t.Fatalf("POST /demo/session: %d to %q", code, loc)
	}
	_, _, week := b.get("/")
	if !strings.Contains(week, `id="demo-notice"`) || !welcome.MatchString(week) {
		t.Error("the first page after entering does not open the notice")
	}
	if !strings.Contains(week, `id="demo-band"`) {
		t.Error("no demo band")
	}
	if strings.Contains(week, `action="/settings/password"`) {
		t.Error("the week links a password change")
	}
	_, _, again := b.get("/")
	if welcome.MatchString(again) {
		t.Error("the notice opens again on the second page")
	}

	var name, lang string
	if err := a.Store.DB.QueryRow(`SELECT u.display_name, g.lang FROM users u
		JOIN demo_members m ON m.user_id = u.id JOIN demo_groups g ON g.id = m.group_id`).Scan(&name, &lang); err != nil {
		t.Fatal(err)
	}
	if lang != "it" || !strings.Contains(week, name) {
		t.Errorf("group in %q for %q; the week shows the name: %v", lang, name, strings.Contains(week, name))
	}

	_, _, settings := b.get("/settings")
	for _, s := range []string{`action="/settings/password"`, `action="/settings/delete-account"`, emailOf(t, a)} {
		if strings.Contains(settings, s) {
			t.Errorf("settings show %s", s)
		}
	}
	for _, p := range []string{"/settings/password", "/settings/delete-account"} {
		if code, _ := b.post("/settings", p, nil); code != http.StatusNotFound {
			t.Errorf("POST %s: %d, want 404", p, code)
		}
	}
	if code, loc := b.post("/settings", "/logout", nil); code != http.StatusSeeOther || loc != "/" {
		t.Errorf("logout: %d to %q, want the entry", code, loc)
	}
}

// emailOf returns the demo user's generated address.
func emailOf(t *testing.T, a *App) string {
	t.Helper()
	var email string
	if err := a.Store.DB.QueryRow(`SELECT email FROM users`).Scan(&email); err != nil {
		t.Fatal(err)
	}
	return email
}
