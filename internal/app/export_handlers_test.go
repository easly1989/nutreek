package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestHandleWeekExport_ReturnsCSV verifica che un utente autenticato riceva
// una risposta CSV valida con gli header corretti.
func TestHandleWeekExport_ReturnsCSV(t *testing.T) {
	a := newTestApp(t)
	user := mustCreateTestUser(t, a, "export@example.com", "password123")

	const weekStart = "2025-01-06"

	r := httptest.NewRequest(http.MethodGet, "/week/"+weekStart+"/export", nil)
	r = requestWithUser(r, user)

	// chi.URLParam richiede il route context: lo iniettamo manualmente.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("start", weekStart)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleWeekExport)).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("week export: want 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("week export: want Content-Type text/csv, got %q", ct)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("week export: Content-Disposition dovrebbe contenere 'attachment', got %q", cd)
	}
	// Verifica che il CSV abbia l'header atteso.
	body := w.Body.String()
	if !strings.Contains(body, "giorno") || !strings.Contains(body, "pasto") {
		t.Errorf("week export: body CSV manca degli header attesi, got: %q", body[:min(len(body), 200)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
