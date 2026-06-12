package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleIngredients_RequiresAuth verifica che le sezioni protette
// reindirizzino a /login quando non autenticati.
func TestHandleIngredients_RequiresAuth(t *testing.T) {
	a := newTestApp(t)
	srv := httptest.NewServer(a.Router())
	defer srv.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // non seguire i redirect
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
				t.Errorf("GET %s senza auth: want 303, got %d", path, resp.StatusCode)
			}
			if loc := resp.Header.Get("Location"); loc != "/login" {
				t.Errorf("GET %s senza auth: want redirect a /login, got %q", path, loc)
			}
		})
	}
}

// TestHandleDashboard_Unauthenticated verifica che la home mostri la landing
// page (non redirect) agli utenti non autenticati.
func TestHandleDashboard_Unauthenticated(t *testing.T) {
	a := newTestApp(t)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleDashboard)).ServeHTTP(w, r)

	// La home senza auth mostra la pagina "home", non un redirect.
	if w.Code != http.StatusOK {
		t.Errorf("home senza auth: want 200, got %d", w.Code)
	}
}

// TestHandleWeekExport_RequiresAuth verifica che l'export CSV richieda auth.
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
		t.Errorf("export senza auth: want 303, got %d", resp.StatusCode)
	}
}
