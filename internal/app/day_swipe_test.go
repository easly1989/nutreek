package app

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The week page loads the script that swipes between days, and every day tab
// carries the name of its day, which the swipe shows at the edge (#250).
func TestDashboardDaySwipe(t *testing.T) {
	a := newTestApp(t)
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")

	get := func(path string) string {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleDashboard)).ServeHTTP(w, requestWithUser(r, alice))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: code %d", path, w.Code)
		}
		return w.Body.String()
	}
	body := get("/?start=2026-11-16&day=2026-11-21")
	if !strings.Contains(body, "js/day_swipe.js") {
		t.Error("the week page must load day_swipe.js")
	}
	titles := regexp.MustCompile(`data-title="([^"]+)"`).FindAllStringSubmatch(body, -1)
	if len(titles) != 7 {
		t.Fatalf("%d day tabs with a title, want 7", len(titles))
	}
	if titles[0][1] != "Lunedì 16" || titles[6][1] != "Domenica 22" {
		t.Errorf("titles: first %q, last %q", titles[0][1], titles[6][1])
	}
}
