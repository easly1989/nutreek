package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleIngredients_RequiresAuth checks that the protected sections
// redirect to /login when not authenticated.
func TestHandleIngredients_RequiresAuth(t *testing.T) {
	a := newTestApp(t)
	srv := httptest.NewServer(a.Router())
	defer srv.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // do not follow redirects
		},
	}

	paths := []string{
		"/ingredients",
		"/recipes",
		"/profile/peso",
		"/stats",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp, err := client.Get(srv.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusSeeOther {
				t.Errorf("GET %s without auth: want 303, got %d", path, resp.StatusCode)
			}
			if loc := resp.Header.Get("Location"); loc != "/login" {
				t.Errorf("GET %s without auth: want redirect to /login, got %q", path, loc)
			}
		})
	}
}

// TestHandleDashboard_Unauthenticated checks that the home shows the landing
// page (not a redirect) to anonymous users.
func TestHandleDashboard_Unauthenticated(t *testing.T) {
	a := newTestApp(t)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleDashboard)).ServeHTTP(w, r)

	// The home without auth shows the "home" page, not a redirect.
	if w.Code != http.StatusOK {
		t.Errorf("home without auth: want 200, got %d", w.Code)
	}
}

// TestHandleWeekExport_RequiresAuth checks that the CSV export requires auth.
func TestHandleWeekExport_RequiresAuth(t *testing.T) {
	a := newTestApp(t)
	srv := httptest.NewServer(a.Router())
	defer srv.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(srv.URL + "/week/2025-01-06/export")
	if err != nil {
		t.Fatalf("GET /week/.../export: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("export without auth: want 303, got %d", resp.StatusCode)
	}
}
