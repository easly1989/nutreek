package app

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/i18n"
)

// A language marked Beta shows a "beta" chip next to its name in both
// selectors (#162); Italian and English do not.
func TestBetaLanguageMark(t *testing.T) {
	a := newTestApp(t)
	user := mustCreateTestUser(t, a, "ada@example.test", "correct horse battery")

	orig := i18n.Languages
	t.Cleanup(func() { i18n.Languages = orig })
	i18n.Languages = append(slices.Clone(orig), i18n.Language{Code: "zz", Name: "Zzlang", Beta: true})

	// The buttons of the language selector, by the language they pick.
	button := regexp.MustCompile(`(?s)<button name="lang" value="(\w*)".*?</button>`)
	chip := `class="chip chip-warn`

	pages := []struct {
		name    string
		target  string
		handler http.HandlerFunc
		signed  bool
	}{
		{"signed-out pages", "/login", a.handleLoginForm, false},
		{"settings", "/settings", a.handleSettingsPage, true},
	}
	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p.target, nil)
			req.AddCookie(&http.Cookie{Name: langCookie, Value: "en"})
			if p.signed {
				req = requestWithUser(req, user)
			}
			rec := httptest.NewRecorder()
			languageMiddleware(a.Sessions.LoadAndSave(p.handler)).ServeHTTP(rec, req)

			got := map[string]bool{}
			for _, m := range button.FindAllString(rec.Body.String(), -1) {
				code := button.FindStringSubmatch(m)[1]
				got[code] = strings.Contains(m, chip)
				if got[code] && !strings.Contains(m, ">beta</span>") {
					t.Errorf("%s: the chip does not say beta: %s", code, m)
				}
			}
			for code, want := range map[string]bool{"it": false, "en": false, "zz": true} {
				if has, ok := got[code]; !ok {
					t.Errorf("no button for %q in:\n%s", code, rec.Body.String())
				} else if has != want {
					t.Errorf("%s: beta chip = %v, want %v", code, has, want)
				}
			}
		})
	}
}
