package app

import (
	"cmp"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/web"
)

// publicPage is one render of the public pages, the sign-in and sign-up
// errors, the onboarding steps or the tour (#156).
type publicPage struct {
	name, method, target string
	form                 url.Values
	anon                 bool   // nobody signed in
	flash                string // flash key stored before the handler runs
	italian              string // what gives the Italian page away, when not a word
	want                 string // English text the page must show
	handler              func(http.ResponseWriter, *http.Request)
}

func TestEnglishPublicPagesAndOnboarding(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	taken := mustCreateTestUser(t, a, "taken@example.test", "correct horse battery")
	userID, err := a.Users.Create(ctx, "ada@example.test", "x", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	// Step 2 shows the presets only when the needs can be computed.
	weight, height, birth, sex, activity := 70.0, 175.0, 1990, "F", "moderate"
	if err := a.Users.UpdateProfile(ctx, userID, db.UserProfileInput{
		WeightKg: &weight, HeightCm: &height, BirthYear: &birth, Sex: &sex, ActivityLevel: &activity,
	}); err != nil {
		t.Fatal(err)
	}
	user, _ := a.Users.GetByID(ctx, userID)

	// A second user with a template, for step 4 with templates to pick from.
	bobID, err := a.Users.Create(ctx, "bob@example.test", "x", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := a.Users.GetByID(ctx, bobID)
	if _, err := a.MealTemplates.Create(ctx, bobID, "Weekday"); err != nil {
		t.Fatal(err)
	}

	// What users typed, not interface text.
	data := []string{"Ada", "Bob", "Weekday", "Utente Test", "taken@example.test"}

	login := func(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			a.loginRateLimiter().reset(remoteIP(r)) // every page starts with a clean limiter
			h(w, r)
		}
	}
	wrongPassword := url.Values{"email": {taken.Email}, "password": {"not the password"}}
	register := func(email, name, password string) url.Values {
		return url.Values{"email": {email}, "display_name": {name}, "password": {password}}
	}

	pages := []publicPage{
		{name: "home", anon: true, target: "/", want: "Plan your meals", handler: a.handleHome},
		{name: "login", anon: true, target: "/login", want: "Your week of meals is waiting.", handler: a.handleLoginForm},
		{name: "login, wrong password", anon: true, method: http.MethodPost, target: "/login", form: wrongPassword,
			want: "Invalid email or password.", handler: login(a.handleLogin)},
		{name: "login, unknown email", anon: true, method: http.MethodPost, target: "/login",
			form: url.Values{"email": {"nobody@example.test"}, "password": {"x"}}, want: "Invalid email or password.", handler: login(a.handleLogin)},
		{name: "login, rate limited", anon: true, method: http.MethodPost, target: "/login", form: wrongPassword,
			want: "Too many attempts. Try again in a minute.", italian: "Troppi tentativi", handler: func(w http.ResponseWriter, r *http.Request) {
				limiter := a.loginRateLimiter()
				limiter.reset(remoteIP(r))
				for range 5 {
					limiter.allow(remoteIP(r))
				}
				a.handleLogin(w, r)
			}},
		{name: "register", anon: true, target: "/register", want: "Create your account", handler: a.handleRegisterForm},
		{name: "register, fields missing", anon: true, method: http.MethodPost, target: "/register", form: register("a@example.test", "", ""),
			want: "Fill in all the fields.", handler: a.handleRegister},
		{name: "register, bad email", anon: true, method: http.MethodPost, target: "/register", form: register("not-an-email", "Eve", "longenough"),
			want: "Invalid email address.", handler: a.handleRegister},
		{name: "register, name too long", anon: true, method: http.MethodPost, target: "/register", form: register("a@example.test", strings.Repeat("e", 65), "longenough"),
			want: "The name can be at most 64 characters.", handler: a.handleRegister},
		{name: "register, short password", anon: true, method: http.MethodPost, target: "/register", form: register("a@example.test", "Eve", "short"),
			want: "Password: at least 8 characters.", italian: "minimo 8 caratteri", handler: a.handleRegister},
		{name: "register, long password", anon: true, method: http.MethodPost, target: "/register", form: register("a@example.test", "Eve", strings.Repeat("p", 73)),
			want: "Password: at most 72 characters.", italian: "massimo 72 caratteri", handler: a.handleRegister},
		{name: "register, email taken", anon: true, method: http.MethodPost, target: "/register", form: register(taken.Email, "Eve", "longenough"),
			want: "This email is already registered.", handler: a.handleRegister},
		{name: "flash after login", anon: true, target: "/", flash: "auth.flash.logged_in", want: "You are logged in.", italian: "Accesso effettuato.", handler: a.handleHome},
		{name: "flash after onboarding", anon: true, target: "/", flash: "onboarding.flash.done", want: "Setup complete. Enjoy Nutreek!", italian: "Configurazione completata", handler: a.handleHome},
		{name: "flash after tour restart", anon: true, target: "/", flash: "tour.flash.restarted", want: "Tour restarted.", italian: "Tour riavviato", handler: a.handleHome},
		{name: "onboarding step 1", target: "/onboarding", want: "Step 1 of 5", handler: a.handleOnboardingPage},
		{name: "onboarding step 2 with presets", target: "/onboarding?step=2", want: "Start from a preset:", handler: a.handleOnboardingPage},
		{name: "onboarding step 3", target: "/onboarding?step=3", want: "Your meal pattern", handler: a.handleOnboardingPage},
		{name: "onboarding step 4, no templates", target: "/onboarding?step=4", want: "You have no templates yet", handler: a.handleOnboardingPage},
		{name: "onboarding step 4, with templates", target: "/onboarding?step=4&restart=true", want: "Don&#39;t apply now", handler: a.handleOnboardingPage},
		{name: "onboarding step 5", target: "/onboarding?step=5", want: "Go to the week", handler: a.handleOnboardingPage},
		{name: "signed in on the home, with the tour", target: "/", want: "Welcome to Nutreek!", handler: a.handleHome},
	}

	render := func(p publicPage, lang string) string {
		method := p.method
		if method == "" {
			method = http.MethodGet
		}
		req := httptest.NewRequest(method, p.target, strings.NewReader(p.form.Encode()))
		if p.form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.AddCookie(&http.Cookie{Name: langCookie, Value: cmp.Or(lang, "it")}) // "" is the Italian run
		if !p.anon {
			viewer := user
			if strings.Contains(p.name, "with templates") {
				viewer = bob
			}
			req = requestWithUser(req, viewer)
		}
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p.flash != "" {
				a.Sessions.Put(r.Context(), "flash", p.flash)
			}
			p.handler(w, r)
		})
		rec := httptest.NewRecorder()
		languageMiddleware(a.Sessions.LoadAndSave(h)).ServeHTTP(rec, req)
		if rec.Code >= 300 && rec.Code < 400 {
			t.Fatalf("%s: redirected to %s", p.name, rec.Header().Get("Location"))
		}
		return rec.Body.String()
	}

	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			body := render(p, "en")
			if !strings.Contains(body, p.want) {
				t.Errorf("%q missing from the English page:\n%s", p.want, body)
			}
			if found := italianIn(body, data); len(found) > 0 {
				t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
			}
			it := render(p, "")
			if p.italian != "" && !strings.Contains(it, p.italian) || p.italian == "" && len(italianIn(it, data)) == 0 {
				t.Errorf("the page without the cookie is not Italian:\n%s", it)
			}
		})
	}

	// The tour's texts travel in a JSON block that tour.js reads.
	t.Run("tour data", func(t *testing.T) {
		re := regexp.MustCompile(`(?s)<script type="application/json" id="tour-steps">(.*?)</script>`)
		for lang, want := range map[string]string{"en": "Welcome to Nutreek!", "": "Benvenuto in Nutreek!"} {
			body := render(pages[len(pages)-1], lang)
			m := re.FindStringSubmatch(body)
			if m == nil {
				t.Fatalf("lang %q: no tour data in the page", lang)
			}
			var steps map[string]map[string]string
			if err := json.Unmarshal([]byte(m[1]), &steps); err != nil {
				t.Fatalf("lang %q: the tour data is not JSON: %v\n%s", lang, err, m[1])
			}
			if steps["welcome"]["title"] != want {
				t.Errorf("lang %q: welcome title = %q, want %q", lang, steps["welcome"]["title"], want)
			}
			for _, k := range []string{"welcome", "week", "days", "template", "sections"} {
				if steps[k]["title"] == "" || steps[k]["text"] == "" {
					t.Errorf("lang %q: step %q is empty", lang, k)
				}
			}
			for _, k := range []string{"skip", "start", "back", "next", "finish"} {
				if steps["buttons"][k] == "" {
					t.Errorf("lang %q: button %q is empty", lang, k)
				}
			}
		}
		// tour.js keeps no text of its own.
		js, err := fs.ReadFile(web.Static(), "js/tour.js")
		if err != nil {
			t.Fatal(err)
		}
		if found := italianIn("<script>"+string(js)+"</script>", nil); len(found) > 0 {
			t.Errorf("Italian left in tour.js:\n  %s", strings.Join(found, "\n  "))
		}
	})

	// The handlers store keys, so the flash follows the language of the page that shows it.
	t.Run("flashes are keys", func(t *testing.T) {
		tests := []struct {
			name    string
			request func() *http.Request
			handler func(http.ResponseWriter, *http.Request)
			want    string
		}{
			{"login", func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"email": {taken.Email}, "password": {"correct horse battery"}}.Encode()))
			}, login(a.handleLogin), "auth.flash.logged_in"},
			{"onboarding done", func() *http.Request {
				return requestWithUser(httptest.NewRequest(http.MethodPost, "/onboarding", strings.NewReader("step=5")), user)
			}, a.handleOnboardingStep, "onboarding.flash.done"},
			{"tour restart", func() *http.Request {
				return requestWithUser(httptest.NewRequest(http.MethodPost, "/tour/restart", nil), user)
			}, a.handleTourRestart, "tour.flash.restarted"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				req := tc.request()
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				var got string
				h := a.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					tc.handler(w, r)
					got = a.Sessions.GetString(r.Context(), "flash")
				}))
				h.ServeHTTP(httptest.NewRecorder(), req)
				if got != tc.want {
					t.Errorf("flash = %q, want the key %q", got, tc.want)
				}
			})
		}
	})
}

// The per-route rate limiter answers in the language of the request.
func TestRateLimitMessageLanguage(t *testing.T) {
	for lang, want := range map[string]string{"en": "Too many requests. Try again in a few minutes.", "": "Troppe richieste. Riprova tra qualche minuto."} {
		limiter := newIPRateLimiter(1, time.Minute)
		h := languageMiddleware(limiter.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
		var body string
		for range 2 {
			req := httptest.NewRequest(http.MethodPost, "/register", nil)
			req.AddCookie(&http.Cookie{Name: langCookie, Value: cmp.Or(lang, "it")}) // "" is the Italian run
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			body = rec.Body.String()
		}
		if !strings.Contains(body, want) {
			t.Errorf("lang %q: body = %q, want %q", lang, body, want)
		}
	}
}

// The manifest's description and language follow the request's.
func TestManifestLanguage(t *testing.T) {
	a := newTestApp(t)
	for lang, want := range map[string][2]string{"en": {"en", "Weekly meal plans, macros and a shopping list."}, "": {"it", "Piani alimentari settimanali, macro e lista della spesa."}} {
		req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
		req.AddCookie(&http.Cookie{Name: langCookie, Value: cmp.Or(lang, "it")}) // "" is the Italian run
		rec := httptest.NewRecorder()
		a.Router().ServeHTTP(rec, req)
		var m struct{ Lang, Description string }
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		if m.Lang != want[0] || m.Description != want[1] {
			t.Errorf("lang %q: manifest = %+v, want %v", lang, m, want)
		}
	}
}
