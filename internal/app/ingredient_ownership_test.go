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

// ingredientTestUser creates a user with a display name, to read «di Bob».
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

// serveIngredient calls h as u with the given form and {id} in the route.
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

// An ingredient's data is edited by whoever created it, or by the owner; the
// others save only their category, and another user's category is rejected (#143).
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

	// Alice tries to change everything: only her category is saved.
	w := serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, alice, barretta,
		ingredientUpdateForm("Rinominata da Alice", "1", strconv.FormatInt(snackAlice, 10)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Alice: code %d, want 303", w.Code)
	}
	ing, _ := a.Ingredients.GetByID(ctx, barretta)
	if ing.Name != "Barretta" || ing.KcalPer100 != 380 {
		t.Errorf("Alice changed Bob's data: %q %v kcal", ing.Name, ing.KcalPer100)
	}
	if got := catOf(alice); got != snackAlice {
		t.Errorf("Alice's category = %d, want %d", got, snackAlice)
	}
	if got := catOf(bob); got != dolciBob {
		t.Errorf("Bob's category changed: %d, want %d", got, dolciBob)
	}

	// One of Bob's categories in Alice's form: rejected, nothing changes.
	w = serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, alice, barretta,
		ingredientUpdateForm("Barretta", "380", strconv.FormatInt(dolciBob, 10)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("another user's category: code %d, want 422", w.Code)
	}
	if got := catOf(alice); got != snackAlice {
		t.Errorf("after the rejection Alice's category is %d, want %d", got, snackAlice)
	}

	// Bob, the author, edits the data; Alice's category stays.
	w = serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, bob, barretta,
		ingredientUpdateForm("Barretta al cacao", "390", strconv.FormatInt(dolciBob, 10)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Bob: code %d, want 303", w.Code)
	}
	if ing, _ := a.Ingredients.GetByID(ctx, barretta); ing.Name != "Barretta al cacao" || ing.KcalPer100 != 390 {
		t.Errorf("Bob did not edit his own ingredient: %q %v", ing.Name, ing.KcalPer100)
	}
	if got := catOf(alice); got != snackAlice {
		t.Errorf("Bob's save touched Alice's category: %d", got)
	}

	// The owner edits other users' ingredients too.
	w = serveIngredient(a, a.handleIngredientUpdate, http.MethodPost, owner, barretta,
		ingredientUpdateForm("Barretta corretta", "385", ""))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("owner: code %d, want 303", w.Code)
	}
	if ing, _ := a.Ingredients.GetByID(ctx, barretta); ing.Name != "Barretta corretta" {
		t.Errorf("the owner did not edit: %q", ing.Name)
	}
}

// The edit page of another user's ingredient is read-only: it says whose it
// is, leaves only the category active and saves only that.
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
			t.Errorf("Alice's page lacks %q", want)
		}
	}
	if strings.Contains(body, "Salva modifiche") {
		t.Error("Alice sees «Salva modifiche» on one of Bob's ingredients")
	}

	body = serveIngredient(a, a.handleIngredientEditForm, http.MethodGet, bob, barretta, nil).Body.String()
	if !strings.Contains(body, "Salva modifiche") || strings.Contains(body, "Salva categoria") || strings.Contains(body, " disabled") {
		t.Error("the page of Bob, the author, is not editable")
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
		{"another user", "owner@x.com", alice, diBob, false},
		{"owner on another user's ingredient", "owner@x.com", owner, diBob, true},
		{"no author, owner configured: user", "owner@x.com", alice, senzaAutore, false},
		{"no author, owner configured: owner", "owner@x.com", owner, senzaAutore, true},
		{"no author, no owner", "", alice, senzaAutore, true},
		{"another user, no owner", "", alice, diBob, false},
		{"no user", "", nil, senzaAutore, false},
	}
	for _, tc := range cases {
		a := &App{Cfg: &config.Config{OwnerEmail: tc.ownerCfg}}
		if got := a.canEditIngredient(tc.u, tc.ing); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
