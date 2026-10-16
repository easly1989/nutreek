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
)

// Gli id di tipi pasto, varianti e voci di un altro utente rispondono 404
// e non toccano il suo piano (#129).
func TestNutritionistHandlersOwnership(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	bobPlan, err := a.NutritionistPlans.GetOrCreateActivePlan(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	bobMT, _ := a.NutritionistPlans.CreateMealType(ctx, bobPlan.ID, "Pranzo")
	bobVar, err := a.NutritionistPlans.CreateVariation(ctx, bobPlan.ID, bobMT, nil)
	if err != nil {
		t.Fatal(err)
	}
	pane, _ := a.Ingredients.CreateLocal(ctx, "Pane", nil, "g", 240, 8, 44, 2, &bob.ID, nil, nil, nil, nil, nil, nil)
	bobItem, err := a.NutritionistPlans.AddItem(ctx, bobPlan.ID, bobVar, nil, &pane, 80, "g")
	if err != nil {
		t.Fatal(err)
	}

	post := func(h http.HandlerFunc, form url.Values, id int64) int {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if id > 0 {
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		}
		r = requestWithUser(r, alice)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, r)
		return w.Code
	}
	id := func(v int64) string { return strconv.FormatInt(v, 10) }

	cases := []struct {
		name string
		h    http.HandlerFunc
		form url.Values
		id   int64
	}{
		{"variante nel tipo pasto di Bob", a.handleNutritionistCreateVariation, url.Values{"meal_type_id": {id(bobMT)}}, 0},
		{"voce nella variante di Bob", a.handleNutritionistAddItem, url.Values{"variation_id": {id(bobVar)}, "ingredient_id": {id(pane)}, "quantity": {"50"}}, 0},
		{"elimina tipo pasto di Bob", a.handleNutritionistDeleteMealType, nil, bobMT},
		{"elimina variante di Bob", a.handleNutritionistDeleteVariation, nil, bobVar},
		{"elimina voce di Bob", a.handleNutritionistDeleteItem, nil, bobItem},
	}
	for _, tc := range cases {
		if code := post(tc.h, tc.form, tc.id); code != http.StatusNotFound {
			t.Errorf("%s: code %d, atteso 404", tc.name, code)
		}
	}

	full, err := a.NutritionistPlans.LoadPlanFull(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.MealTypes) != 1 || len(full.MealTypes[0].Variations) != 1 || len(full.MealTypes[0].Variations[0].Items) != 1 {
		t.Errorf("il piano di Bob e' cambiato: %+v", full.MealTypes)
	}
}
