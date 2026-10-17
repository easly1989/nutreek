package app

import (
	"cmp"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/off"
	"github.com/amon2126/nutreek/internal/web"
)

// i18nTestPassword is a throwaway password for the users of this test.
const i18nTestPassword = "test-only-password-1"

var reNewToken = regexp.MustCompile(`select-all">[^<]*</code>`)

// TestEnglishSettingsAdminCoach renders Settings, the Administration panel,
// the Coach panel and /upgrade with the English cookie and finds no Italian
// left (#155). The same pages without the cookie must still be Italian.
func TestEnglishSettingsAdminCoach(t *testing.T) {
	a := newTestApp(t)
	a.OFFSyncer = off.NewSyncer(a.Store.DB, "http://127.0.0.1:1", nil)
	ctx := context.Background()
	t.Cleanup(invalidateFeatureCache)

	hash, err := bcrypt.GenerateFromPassword([]byte(i18nTestPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(email, name, role string) *domain.User {
		t.Helper()
		id, err := a.Users.Create(ctx, email, string(hash), name)
		if err != nil {
			t.Fatal(err)
		}
		if role != "" {
			if err := a.Users.UpdateRole(ctx, id, role); err != nil {
				t.Fatal(err)
			}
		}
		u, err := a.Users.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	ada := mk("ada@example.test", "Ada", "") // the owner
	a.Cfg.OwnerEmail = ada.Email
	cora := mk("cora@example.test", "Cora", domain.RoleCoach)
	bob := mk("bob@example.test", "Bob", "")
	dan := mk("dan@example.test", "Dan", "")
	eve := mk("eve@example.test", "Eve", "")
	fay := mk("fay@example.test", "Fay", "")
	gus := mk("gus@example.test", "Gus", "")

	soon := time.Now().AddDate(0, 0, 10)
	if err := a.Subscriptions.Grant(ctx, bob.ID, "manual", &soon, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.Subscriptions.Grant(ctx, dan.ID, "manual", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.APITokens.Create(ctx, ada.ID, "Home Assistant"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.DB.Exec(`UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.APITokens.Create(ctx, ada.ID, "Backup"); err != nil { // never used
		t.Fatal(err)
	}

	planCut, err := a.NutritionistPlans.CreatePlan(ctx, cora.ID, "Cut")
	if err != nil {
		t.Fatal(err)
	}
	planBulk, err := a.NutritionistPlans.CreatePlan(ctx, cora.ID, "Bulk")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.NutritionistPlans.SetActivePlan(ctx, cora.ID, planCut); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		u    *domain.User
		plan int64
	}{{dan, planBulk}, {eve, planBulk}, {fay, planCut}} {
		if err := a.Coach.AddClient(ctx, cora.ID, c.u.ID); err != nil {
			t.Fatal(err)
		}
		if err := a.Coach.AssignPlan(ctx, cora.ID, c.u.ID, c.plan); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.NutritionistPlans.CreatePlan(ctx, cora.ID, "Idle"); err != nil { // no client uses it
		t.Fatal(err)
	}

	// What users typed, the names and descriptions of the features (#158) and
	// the month names (#157) are not interface text.
	data := []string{"Ada", "Cora", "Bob", "Dan", "Eve", "Fay", "Gus", "Cut", "Bulk", "Idle", "Home Assistant", "Backup",
		"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}
	features, err := a.Features.List(ctx)
	if err != nil || len(features) == 0 {
		t.Fatalf("features: %v (%d)", err, len(features))
	}
	for _, f := range features {
		data = append(data, f.Name, f.Description)
	}
	if err := a.Features.SetPremium(ctx, features[0].Key, true); err != nil {
		t.Fatal(err)
	}

	type page struct {
		name, method, target string
		form                 url.Values
		as                   *domain.User // the viewer, when not Ada (the owner)
		premium              bool         // NUTREEK_PREMIUM_ENABLED
		flash                string
		want                 []string // English text the page must show
		handler              func(http.ResponseWriter, *http.Request)
	}
	pwForm := func(current, next, again string) url.Values {
		return url.Values{"current_password": {current}, "new_password": {next}, "confirm_password": {again}}
	}
	pages := []page{
		// Settings.
		{name: "settings", target: "/settings", want: []string{"Appearance, language, account and your data.", "Like the browser", "Like the system", "Forest", "Install the app", "Add to Home Screen", "Export as JSON", "Create token", "Delete account", "never"}, handler: a.handleSettingsPage},
		{name: "settings, password changed", target: "/settings?ok=password", want: []string{"Password changed."}, handler: a.handleSettingsPage},
		{name: "settings, free plan", target: "/settings", as: bob, premium: true, handler: a.handleSettingsPage},
		{name: "settings, premium", target: "/settings", as: dan, premium: true, handler: a.handleSettingsPage},
		{name: "settings, no tokens", target: "/settings", as: gus, want: []string{"No tokens yet."}, handler: a.handleSettingsPage},
		{name: "settings, free plan, no tokens", target: "/settings", as: gus, premium: true, want: []string{"Creating tokens is part of Premium."}, handler: a.handleSettingsPage},
		{name: "token created", method: http.MethodPost, target: "/settings/api-tokens", form: url.Values{"name": {"Grafana"}}, want: []string{"Token created: copy it now"}, handler: a.handleSettingsAPITokenCreate},
		{name: "password, wrong current", method: http.MethodPost, target: "/settings/password", form: pwForm("nope", "a-new-password", "a-new-password"), want: []string{"Current password is incorrect."}, handler: a.handleChangePassword},
		{name: "password, too short", method: http.MethodPost, target: "/settings/password", form: pwForm(i18nTestPassword, "short", "short"), want: []string{"at least 8 characters."}, handler: a.handleChangePassword},
		{name: "password, mismatch", method: http.MethodPost, target: "/settings/password", form: pwForm(i18nTestPassword, "a-new-password", "another-one-1"), want: []string{"The passwords do not match."}, handler: a.handleChangePassword},
		{name: "delete account, wrong password", method: http.MethodPost, target: "/settings/delete-account", form: url.Values{"password": {"nope"}}, want: []string{"Incorrect password."}, handler: a.handleDeleteAccount},

		// Administration.
		{name: "admin overview", target: "/admin/panel", want: []string{"Instance, features and users.", "Premium is off", "gating off", "OpenFoodFacts products", "Local copy used to search ingredients.", "Update now", "data-msg-days-ago"}, handler: a.handleAdminPanel},
		{name: "admin overview, premium on", target: "/admin/panel", premium: true, want: []string{"gating on", "applied immediately"}, handler: a.handleAdminPanel},
		{name: "admin users", target: "/admin/panel/users", want: []string{"Search by name or email", "Premium is off", "owner", "No user matches the search."}, handler: a.handleAdminUsers},
		{name: "admin users, premium on", target: "/admin/panel/users", premium: true, want: []string{"Premium until", "Premium, no expiry", "Grant Premium", "Make coach", "Make user"}, handler: a.handleAdminUsers},
		{name: "admin users, bad date", target: "/admin/panel/users", flash: "admin.flash.bad_expiry", want: []string{"Invalid expiry date."}, handler: a.handleAdminUsers},
		{name: "admin users, bad role", target: "/admin/panel/users", flash: "admin.flash.bad_role", want: []string{"Invalid role."}, handler: a.handleAdminUsers},

		// /upgrade.
		{name: "upgrade, not gated", target: "/upgrade", as: gus, want: []string{"The extra features of this instance.", "currently open to everyone"}, handler: a.handleUpgradePage},
		{name: "upgrade, free", target: "/upgrade", as: gus, premium: true, want: []string{"included", "How to get it", "gus@example.test"}, handler: a.handleUpgradePage},
		{name: "upgrade, blocked feature", target: "/upgrade?da=" + features[0].Key, as: gus, premium: true, want: []string{"is part of Premium."}, handler: a.handleUpgradePage},
		{name: "upgrade, premium", target: "/upgrade", as: dan, premium: true, want: []string{"everything is already unlocked"}, handler: a.handleUpgradePage},

		// Coach.
		{name: "coach", target: "/coach", as: cora, want: []string{"Your clients and the plans to assign.", "Their own plan", "no clients", "2 clients", "In the editor", "Open in the editor", "Rename…", "Add client"}, handler: a.handleCoachDashboard},
		{name: "coach, no clients", target: "/coach", as: ada, want: []string{"No clients yet: add one with their email."}, handler: a.handleCoachDashboard},
		{name: "coach, flash", target: "/coach", as: cora, flash: "coach.flash.client_added", want: []string{"Client added."}, handler: a.handleCoachDashboard},
	}

	render := func(p page, lang string) string {
		method := p.method
		if method == "" {
			method = http.MethodGet
		}
		req := httptest.NewRequest(method, p.target, strings.NewReader(p.form.Encode()))
		if p.form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.AddCookie(&http.Cookie{Name: langCookie, Value: cmp.Or(lang, "it")}) // "" is the Italian run
		viewer := ada
		if p.as != nil {
			viewer = p.as
		}
		req = requestWithUser(req, viewer)
		a.Cfg.PremiumEnabled = p.premium
		invalidateFeatureCache()
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
			// The new token is random letters, not interface text.
			body := reNewToken.ReplaceAllString(render(p, "en"), "")
			for _, w := range p.want {
				if !strings.Contains(body, w) {
					t.Errorf("%q missing from the English page:\n%s", w, body)
				}
			}
			if found := italianIn(body, data); len(found) > 0 {
				t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
			}
			if it := reNewToken.ReplaceAllString(render(p, ""), ""); len(italianIn(it, data)) == 0 {
				t.Errorf("the page without the cookie is not Italian:\n%s", it)
			}
		})
	}

	// The upgrade page when no feature is premium.
	t.Run("upgrade, nothing premium", func(t *testing.T) {
		for _, f := range features {
			if err := a.Features.SetPremium(ctx, f.Key, false); err != nil {
				t.Fatal(err)
			}
		}
		body := render(page{target: "/upgrade", as: gus, premium: true, handler: a.handleUpgradePage}, "en")
		if !strings.Contains(body, "No premium features are active.") {
			t.Errorf("no empty state:\n%s", body)
		}
		if found := italianIn(body, data); len(found) > 0 {
			t.Errorf("Italian in the English page:\n  %s", strings.Join(found, "\n  "))
		}
	})

	// Every flash the coach and admin handlers can set is a catalog key with a
	// text in both languages, and the English one is not Italian.
	t.Run("flash keys", func(t *testing.T) {
		flashOf := func(handler http.HandlerFunc, viewer *domain.User, target string, form url.Values, id int64) string {
			t.Helper()
			req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if id != 0 {
				rctx := chi.NewRouteContext()
				rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
				req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			}
			req = requestWithUser(req, viewer)
			var flash string
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler(w, r)
				flash = a.Sessions.GetString(r.Context(), "flash")
			})
			rec := httptest.NewRecorder()
			languageMiddleware(a.Sessions.LoadAndSave(h)).ServeHTTP(rec, req)
			return flash
		}
		type step struct {
			name    string
			handler http.HandlerFunc
			viewer  *domain.User
			form    url.Values
			id      int64
			want    string
		}
		steps := []step{
			{"no email", a.handleCoachAddClient, cora, url.Values{"email": {""}}, 0, "coach.flash.email_required"},
			{"unknown email", a.handleCoachAddClient, cora, url.Values{"email": {"nobody@example.test"}}, 0, "coach.flash.no_such_user"},
			{"self", a.handleCoachAddClient, cora, url.Values{"email": {cora.Email}}, 0, "coach.flash.self"},
			{"has a coach", a.handleCoachAddClient, cora, url.Values{"email": {dan.Email}}, 0, "coach.flash.has_coach"},
			{"added", a.handleCoachAddClient, cora, url.Values{"email": {gus.Email}}, 0, "coach.flash.client_added"},
			{"removed", a.handleCoachRemoveClient, cora, nil, gus.ID, "coach.flash.client_removed"},
			{"unassigned", a.handleCoachAssignPlan, cora, url.Values{"plan_id": {""}}, dan.ID, "coach.flash.unassigned"},
			{"bad plan", a.handleCoachAssignPlan, cora, url.Values{"plan_id": {"x"}}, dan.ID, "coach.flash.bad_plan"},
			{"plan of someone else", a.handleCoachAssignPlan, cora, url.Values{"plan_id": {"99999"}}, dan.ID, "coach.flash.bad_plan_or_client"},
			{"assigned", a.handleCoachAssignPlan, cora, url.Values{"plan_id": {strconv.FormatInt(planBulk, 10)}}, dan.ID, "coach.flash.assigned"},
			{"plan without a name", a.handleCoachCreatePlan, cora, url.Values{"name": {" "}}, 0, "coach.flash.name_required"},
			{"plan created", a.handleCoachCreatePlan, cora, url.Values{"name": {"Maintain"}}, 0, "coach.flash.plan_created"},
			{"rename without a name", a.handleCoachRenamePlan, cora, url.Values{"name": {""}}, planBulk, "coach.flash.name_required"},
			{"renamed", a.handleCoachRenamePlan, cora, url.Values{"name": {"Bulk 2"}}, planBulk, "coach.flash.plan_renamed"},
			{"delete the active plan", a.handleCoachDeletePlan, cora, nil, planCut, "coach.flash.cannot_delete_active"},
			{"deleted", a.handleCoachDeletePlan, cora, nil, planBulk, "coach.flash.plan_deleted"},
			{"bad expiry", a.handleAdminUserGrantPremium, ada, url.Values{"expires_at": {"tomorrow"}}, gus.ID, "admin.flash.bad_expiry"},
			{"bad role", a.handleAdminUserRole, ada, url.Values{"role": {"emperor"}}, gus.ID, "admin.flash.bad_role"},
		}
		for _, s := range steps {
			got := flashOf(s.handler, s.viewer, "/", s.form, s.id)
			if got != s.want {
				t.Errorf("%s: flash = %q, want %q", s.name, got, s.want)
				continue
			}
			en := i18n.T("en", got)
			if en == got || i18n.T("it", got) == got {
				t.Errorf("%s: %q has no text in both languages", s.name, got)
			}
			if found := italianIn(en, nil); len(found) > 0 {
				t.Errorf("%s: Italian in the English flash %q", s.name, en)
			}
		}
	})
}

// Every catalog key the templates and handlers of this area name exists in
// both languages, the plural ones included.
func TestSettingsAdminCoachKeysExist(t *testing.T) {
	keyRe := regexp.MustCompile(`"((?:settings|admin|coach|upgrade)\.[a-z0-9_.]+)"`)
	var keys []string
	err := fs.WalkDir(web.Templates(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		src, err := fs.ReadFile(web.Templates(), path)
		if err != nil {
			return err
		}
		for _, m := range keyRe.FindAllStringSubmatch(string(src), -1) {
			keys = append(keys, m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"settings_handlers.go", "coach_handlers.go", "admin_panel_handlers.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range keyRe.FindAllStringSubmatch(string(src), -1) {
			keys = append(keys, m[1])
		}
	}
	if len(keys) < 100 {
		t.Fatalf("found only %d keys: the pattern is off", len(keys))
	}
	for _, k := range keys {
		for _, lang := range []string{"it", "en"} {
			if i18n.T(lang, k, "Count", 2) == k {
				t.Errorf("%s has no %s text", k, lang)
			}
		}
	}
}

// The script of the OpenFoodFacts card keeps its texts in data attributes: no
// Italian word is left in the file.
func TestOFFSyncScriptHasNoItalian(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "web", "static", "js", "off_sync.js"))
	if err != nil {
		t.Fatal(err)
	}
	page := "<script>" + string(src) + "</script>"
	if found := italianIn(page, nil); len(found) > 0 {
		t.Errorf("Italian in off_sync.js:\n  %s", strings.Join(found, "\n  "))
	}
	for _, w := range []string{"mai", "oggi", "ieri", "aggiornat", "Avvio", "Errore", "Pronto", "Vuoto", "stato"} {
		if regexp.MustCompile(`'[^']*` + w + `[^']*'`).Match(src) {
			t.Errorf("off_sync.js still has %q in a string", w)
		}
	}
}

// The JSON answer of requireFeature is plain English, whatever the language.
func TestRequireFeatureAPIErrorIsEnglish(t *testing.T) {
	a := newTestApp(t)
	a.Cfg.PremiumEnabled = true
	invalidateFeatureCache()
	t.Cleanup(invalidateFeatureCache)

	id, err := a.Users.Create(context.Background(), "free@x.test", "h", "Free")
	if err != nil {
		t.Fatal(err)
	}
	h := a.requireFeature("auto_plan")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler reached without premium")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	req = requestWithUser(req, &domain.User{ID: id, Email: "free@x.test"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"feature not available on your plan"`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
