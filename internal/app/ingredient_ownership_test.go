package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/config"
	"github.com/amon2126/nutreek/internal/domain"
)

// ingredientTestUser crea un utente con un nome visibile, per leggere «di Bob».
func ingredientTestUser(t *testing.T, a *App, email, name string) *domain.User {
	t.Helper()
	id, err := a.Users.Create(context.Background(), email, "h", name)
	if err != nil {
		t.Fatal(err)
	}
	u, err := a.Users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// serveIngredient chiama h come u con il form dato e {id} nella route.
func serveIngredient(a *App, h http.HandlerFunc, method string, u *domain.User, id int64, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	r = requestWithUser(r, u)
	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(h).ServeHTTP(w, r)
	return w
}

func ingredientUpdateForm(name, kcal, categoryID string) url.Values {
	return url.Values{
		"name": {name}, "unit": {"g"},
		"kcal": {kcal}, "protein": {"20"}, "carbs": {"40"}, "fat": {"10"},
		"category_id": {categoryID},
	}
}

// I dati di un ingrediente li modifica chi l'ha creato, o l'owner; gli altri
// salvano solo la loro categoria, e una categoria altrui viene rifiutata (#143).
func TestIngredientUpdateOwnership(t *testing.T) {
	a := newTestApp(t)
	a.Cfg.OwnerEmail = "owner@x.com"
	ctx := context.Background()
	alice := ingredientTestUser(t, a, "alice@x.com", "Alice")
	bob := ingredientTestUser(t, a, "bob@x.com", "Bob")
	owner := ingredientTestUser(t, a, "owner@x.com", "Owner")
	snackAlice, _ := a.Categories.CreateCategory(ctx, alice.ID, "Snack")
	dolciBob, _ := a.Categories.CreateCategory(ctx, bob.ID, "Dolci")
	barretta, _ := a.Ingredients.CreateLocal(ctx, "Barretta", nil, "g", 380, 30, 40, 12, &bob.ID, nil, nil, nil, nil, nil, nil)
	if err := a.Categories.SetIngredientCategory(ctx, bob.ID, barretta, &dolciBob); err != nil {
		t.Fatal(err)
	}
	catOf := func(u *domain.User) int64 {
		got, err := a.Categories.CategoriesForIngredients(ctx, u.ID, []int64{barretta})
		if err != nil {
			t.Fatal(err)
		}
		return got[barretta].ID
	}

	// Alice prova a cambiare tutto: si salva solo la sua categoria.
	w := serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, alice, barretta,
		ingredientUpdateForm("Rinominata da Alice", "1", strconv.FormatInt(snackAlice, 10)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Alice: code %d, atteso 303", w.Code)
	}
	ing, _ := a.Ingredients.GetByID(ctx, barretta)
	if ing.Name != "Barretta" || ing.KcalPer100 != 380 {
		t.Errorf("Alice ha cambiato i dati di Bob: %q %v kcal", ing.Name, ing.KcalPer100)
	}
	if got := catOf(alice); got != snackAlice {
		t.Errorf("categoria di Alice = %d, attesa %d", got, snackAlice)
	}
	if got := catOf(bob); got != dolciBob {
		t.Errorf("la categoria di Bob e' cambiata: %d, attesa %d", got, dolciBob)
	}

	// Una categoria di Bob nel form di Alice: rifiutata, niente cambia.
	w = serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, alice, barretta,
		ingredientUpdateForm("Barretta", "380", strconv.FormatInt(dolciBob, 10)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("categoria altrui: code %d, atteso 422", w.Code)
	}
	if got := catOf(alice); got != snackAlice {
		t.Errorf("dopo il rifiuto la categoria di Alice e' %d, attesa %d", got, snackAlice)
	}

	// Bob, l'autore, modifica i dati; la categoria di Alice resta.
	w = serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, bob, barretta,
		ingredientUpdateForm("Barretta al cacao", "390", strconv.FormatInt(dolciBob, 10)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Bob: code %d, atteso 303", w.Code)
	}
	if ing, _ := a.Ingredients.GetByID(ctx, barretta); ing.Name != "Barretta al cacao" || ing.KcalPer100 != 390 {
		t.Errorf("Bob non ha modificato il suo ingrediente: %q %v", ing.Name, ing.KcalPer100)
	}
	if got := catOf(alice); got != snackAlice {
		t.Errorf("il salvataggio di Bob ha toccato la categoria di Alice: %d", got)
	}

	// L'owner modifica anche quelli degli altri.
	w = serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, owner, barretta,
		ingredientUpdateForm("Barretta corretta", "385", ""))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("owner: code %d, atteso 303", w.Code)
	}
	if ing, _ := a.Ingredients.GetByID(ctx, barretta); ing.Name != "Barretta corretta" {
		t.Errorf("l'owner non ha modificato: %q", ing.Name)
	}
}

// La pagina di modifica di un ingrediente altrui e' in sola lettura: dice di
// chi e', lascia attiva solo la categoria e salva solo quella.
func TestIngredientEditFormReadOnly(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := ingredientTestUser(t, a, "alice@x.com", "Alice")
	bob := ingredientTestUser(t, a, "bob@x.com", "Bob")
	_, _ = a.Categories.CreateCategory(ctx, alice.ID, "Snack")
	barretta, _ := a.Ingredients.CreateLocal(ctx, "Barretta", nil, "g", 380, 30, 40, 12, &bob.ID, nil, nil, nil, nil, nil, nil)

	body := serveIngredient(a, a.handleIngredientEditForm, http.MethodGet, alice, barretta, nil).Body.String()
	for _, want := range []string{"di Bob", "La tua categoria", "Salva categoria", `name="name" required maxlength="200" value="Barretta" class="field" disabled`} {
		if !strings.Contains(body, want) {
			t.Errorf("pagina di Alice senza %q", want)
		}
	}
	if strings.Contains(body, "Salva modifiche") {
		t.Error("Alice vede «Salva modifiche» su un ingrediente di Bob")
	}

	body = serveIngredient(a, a.handleIngredientEditForm, http.MethodGet, bob, barretta, nil).Body.String()
	if !strings.Contains(body, "Salva modifiche") || strings.Contains(body, "Salva categoria") || strings.Contains(body, " disabled") {
		t.Error("la pagina di Bob, l'autore, non e' modificabile")
	}
}

func TestCanEditIngredient(t *testing.T) {
	alice := &domain.User{ID: 1, Email: "alice@x.com"}
	bob := &domain.User{ID: 2, Email: "bob@x.com"}
	owner := &domain.User{ID: 3, Email: "owner@x.com"}
	diBob := &domain.Ingredient{CreatedBy: &bob.ID}
	senzaAutore := &domain.Ingredient{}

	cases := []struct {
		name     string
		ownerCfg string
		u        *domain.User
		ing      *domain.Ingredient
		want     bool
	}{
		{"autore", "owner@x.com", bob, diBob, true},
		{"altro utente", "owner@x.com", alice, diBob, false},
		{"owner su ingrediente altrui", "owner@x.com", owner, diBob, true},
		{"senza autore, owner configurato: utente", "owner@x.com", alice, senzaAutore, false},
		{"senza autore, owner configurato: owner", "owner@x.com", owner, senzaAutore, true},
		{"senza autore, nessun owner", "", alice, senzaAutore, true},
		{"altro utente, nessun owner", "", alice, diBob, false},
		{"nessun utente", "", nil, senzaAutore, false},
	}
	for _, tc := range cases {
		a := &App{Cfg: &config.Config{OwnerEmail: tc.ownerCfg}}
		if got := a.canEditIngredient(tc.u, tc.ing); got != tc.want {
			t.Errorf("%s: %v, atteso %v", tc.name, got, tc.want)
		}
	}
}
