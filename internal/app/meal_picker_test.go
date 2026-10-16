package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// withMealID aggiunge il parametro di rotta {mealID} come farebbe chi.
func withMealID(r *http.Request, id int64) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("mealID", strconv.FormatInt(id, 10))
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// Il picker «Aggiungi alimento» si apre in una finestra sopra la settimana
// quando lo chiede HTMX, e resta una pagina intera senza JavaScript (#122).
func TestAddItemFormDialogEPagina(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, alice.ID, monday)
	if err != nil {
		t.Fatal(err)
	}
	meal, err := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 3, "Pranzo")
	if err != nil {
		t.Fatal(err)
	}
	h := a.Sessions.LoadAndSave(http.HandlerFunc(a.handleAddItemForm))
	url := "/meals/" + strconv.FormatInt(meal, 10) + "/items/new?start=2026-10-12"

	// HTMX: solo la finestra.
	r := withMealID(requestWithUser(httptest.NewRequest(http.MethodGet, url, nil), alice), meal)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "<dialog") || strings.Contains(body, "<html") {
		t.Fatalf("HTMX: atteso solo il <dialog>, code %d", w.Code)
	}
	for _, want := range []string{"Aggiungi a Pranzo", "Giovedì 15 ottobre", `data-meal-picker`} {
		if !strings.Contains(body, want) {
			t.Errorf("HTMX: manca %q", want)
		}
	}

	// Senza HTMX: pagina intera con lo stesso picker.
	r = withMealID(requestWithUser(httptest.NewRequest(http.MethodGet, url, nil), alice), meal)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<html") || !strings.Contains(w.Body.String(), "data-meal-picker") {
		t.Errorf("pagina: atteso layout completo col picker, code %d", w.Code)
	}

	// Il pasto di un altro utente non esiste.
	r = withMealID(requestWithUser(httptest.NewRequest(http.MethodGet, url, nil), bob), meal)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("pasto altrui: atteso 404, got %d", w.Code)
	}

	// Il nome del pasto va codificato nella query del picker: con «&» i
	// suggerimenti del piano nutrizionale si perderebbero.
	odd, err := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 3, "Pranzo & cena")
	if err != nil {
		t.Fatal(err)
	}
	r = withMealID(requestWithUser(httptest.NewRequest(http.MethodGet, "/meals/x/items/new", nil), alice), odd)
	r.Header.Set("HX-Request", "true")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if body := w.Body.String(); !strings.Contains(body, "meal_name=Pranzo&#43;%26&#43;cena") {
		t.Errorf("meal_name non codificato nel picker")
	}
}

// Un errore durante una richiesta HTMX ricarica la pagina (HX-Redirect)
// invece di far seguire a HTMX il 303 e infilare la settimana intera nel
// frammento da sostituire.
func TestFlashAndBackHTMX(t *testing.T) {
	a := newTestApp(t)
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	h := a.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.flashAndBack(w, r, monday, "errore")
	}))

	r := httptest.NewRequest(http.MethodPost, "/meals/1/items", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || w.Header().Get("HX-Redirect") != "/?start=2026-10-12" {
		t.Errorf("HTMX: code %d, HX-Redirect %q", w.Code, w.Header().Get("HX-Redirect"))
	}

	r = httptest.NewRequest(http.MethodPost, "/meals/1/items", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/?start=2026-10-12" {
		t.Errorf("form classico: code %d, Location %q", w.Code, w.Header().Get("Location"))
	}
}
