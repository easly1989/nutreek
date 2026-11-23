package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Moving a row to another meal (#228): the menu lists the other meals and days,
// the post moves the row (answering with the day it left, and the tab of the day
// it went to), and another user's row or meal is out of reach.
func TestMoveItemHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	oats, _ := a.Ingredients.CreateLocal(ctx, "Fiocchi", nil, "g", 380, 13, 60, 7, &alice.ID, nil, nil, nil, nil, nil, nil)
	milk, _ := a.Ingredients.CreateLocal(ctx, "Latte", nil, "g", 46, 3.4, 4.8, 1.6, &alice.ID, nil, nil, nil, nil, nil, nil)

	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	plan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, alice.ID, monday)
	breakfast, _ := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 0, "Colazione")
	lunch, _ := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 0, "Pranzo")
	a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 1, "Colazione")
	oatsRow, err := a.WeeklyPlans.AddItem(ctx, breakfast, alice.ID, nil, &oats, 40, "g")
	if err != nil {
		t.Fatal(err)
	}
	milkRow, _ := a.WeeklyPlans.AddItem(ctx, lunch, alice.ID, nil, &milk, 200, "g")
	key := func(v int64) string { return strconv.FormatInt(v, 10) }

	call := func(h http.HandlerFunc, method, target string, form url.Values, htmx bool, u int, kv ...string) *httptest.ResponseRecorder {
		body := strings.NewReader("")
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		r := httptest.NewRequest(method, target, body)
		if htmx {
			r.Header.Set("HX-Request", "true")
		}
		if form != nil {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		who := alice
		if u == 2 {
			who = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, withParams(requestWithUser(r, who), kv...))
		return w
	}
	const start = "?start=2026-10-12"
	where := func(item int64) int64 {
		var m int64
		if err := a.Store.DB.QueryRowContext(ctx, `SELECT meal_id FROM meal_items WHERE id = ?`, item).Scan(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	// The menu: the other meal of the day, and the six other days.
	menu := call(a.handleMealItemMoveMenu, http.MethodGet, "/meal-items/"+key(oatsRow)+"/move"+start, nil, true, 1, "itemID", key(oatsRow))
	body := menu.Body.String()
	if menu.Code != http.StatusOK || !strings.Contains(body, `name="meal_id" value="`+key(lunch)+`"`) || strings.Contains(body, `name="meal_id" value="`+key(breakfast)+`"`) ||
		strings.Count(body, `name="to_day"`) != 6 || strings.Contains(body, `name="to_day" value="0"`) {
		t.Fatalf("menu: %d\n%s", menu.Code, body)
	}
	if w := call(a.handleMealItemMoveMenu, http.MethodGet, "/x", nil, true, 2, "itemID", key(oatsRow)); w.Code != http.StatusNotFound {
		t.Errorf("menu of another user's row: %d, want 404", w.Code)
	}

	// A drop between two rows of another meal: the day card answers with the row in
	// its new meal.
	w := call(a.handleMealItemMove, http.MethodPost, "/x"+start, url.Values{"meal_id": {key(lunch)}, "ids": {key(oatsRow) + "," + key(milkRow)}}, true, 1, "itemID", key(oatsRow))
	if w.Code != http.StatusOK || where(oatsRow) != lunch || !strings.Contains(w.Body.String(), "Fiocchi") {
		t.Fatalf("move: %d, row in meal %d (want %d)\n%s", w.Code, where(oatsRow), lunch, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `id="pill-2026-10-13"`) {
		t.Error("a move inside the day must not send another day's tab")
	}
	// The rows of a meal reorder without a form (dnd.js): the list must carry the
	// CSRF token itself, or the server answers 403 to every drop. A row moves to
	// another meal from its menu only: no drag across meals, no drag of meals.
	if got := w.Body.String(); !strings.Contains(got, `data-sortable="items" data-sortable-handle="[data-item-grip]"`) ||
		!strings.Contains(got, `data-csrf=""`) {
		t.Error("a meal's sortable list must carry data-csrf")
	} else if strings.Contains(got, `data-sortable-group`) || strings.Contains(got, `data-move-url`) || strings.Contains(got, `data-sortable="meals"`) {
		t.Error("rows and meals must not drag across lists")
	}

	// A row has one menu (#251): its «Move to…» choices load into it and the
	// delete is in it, with no icon of its own on the row.
	if got := w.Body.String(); !strings.Contains(got, `id="move-targets-`) || !strings.Contains(got, `hx-target="#move-targets-`) ||
		strings.Contains(got, `item-macros`) {
		t.Error("a row must have a single menu and no macros of its own")
	}

	// Another day: the row leaves, and the tab of that day comes OOB.
	w = call(a.handleMealItemMove, http.MethodPost, "/x"+start, url.Values{"to_day": {"1"}}, true, 1, "itemID", key(milkRow))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `id="pill-2026-10-13"`) || !strings.Contains(w.Body.String(), `hx-swap-oob`) {
		t.Fatalf("move to a day: %d\n%s", w.Code, w.Body.String())
	}
	var day int
	if err := a.Store.DB.QueryRowContext(ctx, `SELECT m.day_of_week FROM meals m JOIN meal_items mi ON mi.meal_id = m.id WHERE mi.id = ?`, milkRow).Scan(&day); err != nil || day != 1 {
		t.Errorf("milk on day %d (%v), want 1", day, err)
	}

	// Refused: another user's row, a meal of another user, an order that is not the
	// meal's rows, a day out of range, nothing chosen. Without htmx it redirects.
	bobPlan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, bob.ID, monday)
	bobMeal, _ := a.WeeklyPlans.AddMeal(ctx, bobPlan.ID, bob.ID, 0, "Colazione")
	cases := []struct {
		name string
		form url.Values
		user int
		want int
	}{
		{"another user's row", url.Values{"meal_id": {key(lunch)}}, 2, http.StatusNotFound},
		{"a meal of another user", url.Values{"meal_id": {key(bobMeal)}}, 1, http.StatusNotFound},
		{"an order that is not the meal's rows", url.Values{"meal_id": {key(lunch)}, "ids": {"999"}}, 1, http.StatusConflict},
		{"a day out of range", url.Values{"to_day": {"9"}}, 1, http.StatusBadRequest},
		{"nothing chosen", url.Values{}, 1, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if w := call(a.handleMealItemMove, http.MethodPost, "/x"+start, tc.form, true, tc.user, "itemID", key(oatsRow)); w.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, w.Code, tc.want)
		}
	}
	if where(oatsRow) != lunch {
		t.Error("a refused move must leave the row where it was")
	}
	if w := call(a.handleMealItemMove, http.MethodPost, "/x"+start, url.Values{"meal_id": {key(breakfast)}}, false, 1, "itemID", key(oatsRow)); w.Code != http.StatusSeeOther || where(oatsRow) != breakfast {
		t.Errorf("without htmx: %d, want a redirect with the row moved", w.Code)
	}
}
