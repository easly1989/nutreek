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

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/domain"
)

// The household (#238): people are added, changed and removed on their page; a row's
// dialog sets what each person eats of it; a meal can be skipped; and everything is
// refused for the people and rows of another account.
func TestHouseholdHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	target := 2400.0
	if _, err := a.Store.DB.Exec(`UPDATE users SET target_kcal = ? WHERE id = ?`, target, alice.ID); err != nil {
		t.Fatal(err)
	}
	alice.TargetKcal = &target

	call := func(h http.HandlerFunc, method, target string, v url.Values, params map[string]int64, who *domain.User, htmx bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(v.Encode()))
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if htmx {
			r.Header.Set("HX-Request", "true")
		}
		rctx := chi.NewRouteContext()
		for k, id := range params {
			rctx.URLParams.Add(k, strconv.FormatInt(id, 10))
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, requestWithUser(r, who))
		return w
	}

	// Add, change: a person needs a name and a goal in range.
	if w := call(a.handleHouseholdSave, http.MethodPost, "/settings/household", url.Values{"name": {"Giulia"}, "target_kcal": {"1800"}}, nil, alice, false); w.Code != http.StatusSeeOther {
		t.Fatalf("add: %d", w.Code)
	}
	call(a.handleHouseholdSave, http.MethodPost, "/settings/household", url.Values{"name": {""}, "target_kcal": {"1800"}}, nil, alice, false)
	call(a.handleHouseholdSave, http.MethodPost, "/settings/household", url.Values{"name": {"Leo"}, "target_kcal": {"50"}}, nil, alice, false)
	people, _ := a.Household.List(ctx, alice.ID)
	if len(people) != 1 || people[0].Name != "Giulia" || people[0].TargetKcal != 1800 {
		t.Fatalf("people = %+v", people)
	}
	giulia := people[0].ID
	if w := call(a.handleHouseholdSave, http.MethodPost, "/settings/household/x", url.Values{"name": {"Giulia R."}, "target_kcal": {"1850,5"}}, map[string]int64{"id": giulia}, alice, false); w.Code != http.StatusSeeOther {
		t.Fatalf("update: %d", w.Code)
	}
	if p, _ := a.Household.List(ctx, alice.ID); p[0].Name != "Giulia R." || p[0].TargetKcal != 1850.5 {
		t.Errorf("after the update: %+v", p)
	}
	if w := call(a.handleHouseholdSave, http.MethodPost, "/settings/household/x", url.Values{"name": {"Mine"}, "target_kcal": {"2000"}}, map[string]int64{"id": giulia}, bob, false); w.Code != http.StatusNotFound {
		t.Errorf("Bob changing Alice's person: %d, want 404", w.Code)
	}

	// The page lists them with the owner first.
	page := call(a.handleHouseholdPage, http.MethodGet, "/settings/household", nil, nil, alice, false)
	if body := page.Body.String(); page.Code != http.StatusOK || !strings.Contains(body, "Giulia R.") || !strings.Contains(body, "×0,77") {
		t.Errorf("page: %d\n%s", page.Code, body)
	}

	// A week with a meal and a row.
	monday := time.Now()
	for monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, -1)
	}
	oats, _ := a.Ingredients.CreateLocal(ctx, "Oats", nil, domain.UnitGram, 380, 13, 60, 7, &alice.ID, nil, nil, nil, nil, nil, nil)
	plan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, alice.ID, monday)
	meal, _ := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 0, "Breakfast")
	row, _ := a.WeeklyPlans.AddItem(ctx, meal, alice.ID, nil, &oats, 100, "g")
	start := "?start=" + monday.Format("2006-01-02")
	rowParam := map[string]int64{"itemID": row}

	// The row's dialog: one field for each person, the automatic quantity as a hint.
	dlg := call(a.handleHouseholdItemForm, http.MethodGet, "/meal-items/x/people"+start, nil, rowParam, alice, true)
	if body := dlg.Body.String(); dlg.Code != http.StatusOK || !strings.Contains(body, `name="q_`+strconv.FormatInt(giulia, 10)+`"`) || !strings.Contains(body, `placeholder="75"`) {
		t.Fatalf("dialog: %d\n%s", dlg.Code, dlg.Body.String())
	}
	if w := call(a.handleHouseholdItemForm, http.MethodGet, "/meal-items/x/people"+start, nil, rowParam, bob, true); w.Code != http.StatusNotFound {
		t.Errorf("Bob opening Alice's row: %d, want 404", w.Code)
	}

	// The day: a column for each person on the rows and their initials on top of the meal;
	// an account with nobody else keeps the plain rows.
	onMonday := start + "&day=" + monday.Format("2006-01-02")
	dayHTML := call(a.handleDashboard, http.MethodGet, "/"+onMonday, nil, nil, alice, false).Body.String()
	for _, want := range []string{`data-hh `, `data-hh-s`, `hh-headrow`, `class="hh-cell`} {
		if !strings.Contains(dayHTML, want) {
			t.Errorf("the day has no %q", want)
		}
	}
	if plain := call(a.handleDashboard, http.MethodGet, "/"+onMonday, nil, nil, bob, false).Body.String(); strings.Contains(plain, "hh-headrow") {
		t.Error("an account without a household shows the household's header")
	}

	// A quantity by hand is kept; the automatic one, or an empty field, clears it; junk is refused.
	field := "q_" + strconv.FormatInt(giulia, 10)
	save := func(v string) {
		t.Helper()
		if w := call(a.handleHouseholdItemSave, http.MethodPost, "/meal-items/x/people"+start, url.Values{field: {v}}, rowParam, alice, true); w.Code >= 400 {
			t.Fatalf("save %q: %d", v, w.Code)
		}
	}
	week := func() map[int64]float64 {
		w, _ := a.Household.Week(ctx, alice.ID, plan.ID)
		return w.Quantities[giulia]
	}
	save("90")
	if week()[row] != 90 {
		t.Errorf("by hand = %v", week())
	}
	save("92,5")
	if week()[row] != 92.5 {
		t.Errorf("a decimal comma = %v", week())
	}
	save("0")
	if q, ok := week()[row]; !ok || q != 0 {
		t.Errorf("0 means not eaten: %v", week())
	}
	save("")
	if len(week()) != 0 {
		t.Errorf("empty goes back to automatic: %v", week())
	}
	save("75.0")
	if len(week()) != 0 {
		t.Errorf("the automatic quantity is not kept as a hand-set one: %v", week())
	}
	call(a.handleHouseholdItemSave, http.MethodPost, "/meal-items/x/people"+start, url.Values{field: {"lots"}}, rowParam, alice, true)
	call(a.handleHouseholdItemSave, http.MethodPost, "/meal-items/x/people"+start, url.Values{field: {"-5"}}, rowParam, alice, true)
	if len(week()) != 0 {
		t.Errorf("junk was saved: %v", week())
	}
	// A bad field saves nothing, not even the good ones before it.
	leo, _ := a.Household.Add(ctx, alice.ID, "Leo", 1200)
	call(a.handleHouseholdItemSave, http.MethodPost, "/meal-items/x/people"+start, url.Values{field: {"90"}, "q_" + strconv.FormatInt(leo, 10): {"abc"}}, rowParam, alice, true)
	if len(week()) != 0 {
		t.Errorf("a partial save: %v", week())
	}
	if err := a.Household.Delete(ctx, alice.ID, leo); err != nil {
		t.Fatal(err)
	}
	if w := call(a.handleHouseholdItemSave, http.MethodPost, "/meal-items/x/people"+start, url.Values{field: {"50"}}, rowParam, bob, true); w.Code != http.StatusNotFound || len(week()) != 0 {
		t.Errorf("Bob setting Alice's row: %d, %v", w.Code, week())
	}

	// The owner's own row (#353): 0 is not eaten and keeps the base the others scale from,
	// a number brings it back (and sets it), empty changes nothing, a bad field saves nothing.
	saveOwner := func(v url.Values) int {
		return call(a.handleHouseholdItemSave, http.MethodPost, "/meal-items/x/people"+start, v, rowParam, alice, true).Code
	}
	ownerRow := func() domain.MealItemFull {
		full, _ := a.WeeklyPlans.LoadFull(ctx, plan)
		return full.Days[0][0].Items[0]
	}
	if code := saveOwner(url.Values{"q_owner": {"0"}}); code >= 400 {
		t.Fatalf("owner 0: %d", code)
	}
	if it := ownerRow(); !it.OwnerSkip || it.Quantity != 100 {
		t.Errorf("after 0: skip %v, base %v; want skipped with the base kept", it.OwnerSkip, it.Quantity)
	}
	if body := call(a.handleHouseholdItemForm, http.MethodGet, "/meal-items/x/people"+start, nil, rowParam, alice, true).Body.String(); !strings.Contains(body, `placeholder="75"`) || !strings.Contains(body, "base 100 g") {
		t.Errorf("the dialog of a row the owner skips: Giulia keeps her automatic 75 g and the owner's note shows\n%s", body)
	}
	saveOwner(url.Values{"q_owner": {""}})
	if !ownerRow().OwnerSkip {
		t.Error("an empty owner field changed the row")
	}
	if code := saveOwner(url.Values{"q_owner": {"120"}, "qty_unit": {"g"}}); code >= 400 {
		t.Fatalf("owner 120: %d", code)
	}
	if it := ownerRow(); it.OwnerSkip || it.Quantity != 120 {
		t.Errorf("after 120: skip %v, quantity %v; want eaten, 120", it.OwnerSkip, it.Quantity)
	}
	saveOwner(url.Values{"q_owner": {"0"}, field: {"abc"}}) // a bad field of another person: nothing is saved
	saveOwner(url.Values{"q_owner": {"lots"}})
	if it := ownerRow(); it.OwnerSkip || it.Quantity != 120 {
		t.Errorf("a bad field changed the owner's row: skip %v, quantity %v", it.OwnerSkip, it.Quantity)
	}
	if code := saveOwner(url.Values{"q_owner": {"5"}, "qty_unit": {"kg"}}); code != http.StatusUnprocessableEntity || ownerRow().Quantity != 120 {
		t.Errorf("a unit the dialog does not offer: %d", code)
	}
	saveOwner(url.Values{"q_owner": {"100"}})
	if ownerRow().Quantity != 100 {
		t.Error("the owner's quantity is back to 100")
	}
	// An account with nobody else has the same dialog, with the owner only.
	solo := mustCreateTestUser(t, a, "solo@x.com", "password-di-prova")
	soloPlan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, solo.ID, monday)
	soloMeal, _ := a.WeeklyPlans.AddMeal(ctx, soloPlan.ID, solo.ID, 0, "Lunch")
	soloRow, _ := a.WeeklyPlans.AddItem(ctx, soloMeal, solo.ID, nil, &oats, 50, "g")
	soloParam := map[string]int64{"itemID": soloRow}
	if w := call(a.handleHouseholdItemForm, http.MethodGet, "/meal-items/x/people"+start, nil, soloParam, solo, true); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="q_owner"`) || strings.Contains(w.Body.String(), `name="q_`+strconv.FormatInt(giulia, 10)) {
		t.Errorf("the dialog without a household: %d\n%s", w.Code, w.Body.String())
	}
	if w := call(a.handleHouseholdItemSave, http.MethodPost, "/meal-items/x/people"+start, url.Values{"q_owner": {"0"}}, soloParam, solo, true); w.Code >= 400 {
		t.Fatalf("owner 0 without a household: %d", w.Code)
	}
	if full, _ := a.WeeklyPlans.LoadFull(ctx, soloPlan); !full.Days[0][0].Items[0].OwnerSkip {
		t.Error("the owner's skip is not saved without a household")
	}

	// A skipped meal: toggled on and off, refused for another account.
	skip := func(v string, who *domain.User) *httptest.ResponseRecorder {
		return call(a.handleHouseholdMealSkip, http.MethodPost, "/meals/x/people/y/skip"+start, url.Values{"skip": {v}}, map[string]int64{"mealID": meal, "personID": giulia}, who, true)
	}
	if w := skip("1", alice); w.Code != http.StatusOK {
		t.Fatalf("skip: %d", w.Code)
	}
	if w, _ := a.Household.Week(ctx, alice.ID, plan.ID); !w.Skips[giulia][meal] {
		t.Error("the meal is not skipped")
	}
	if w := skip("1", bob); w.Code != http.StatusNotFound {
		t.Errorf("Bob skipping Alice's meal: %d, want 404", w.Code)
	}
	if body := skip("0", alice).Body.String(); strings.Contains(body, "skips") {
		t.Errorf("the skip chip stays after the meal is eaten again")
	}
	if w, _ := a.Household.Week(ctx, alice.ID, plan.ID); len(w.Skips) != 0 {
		t.Errorf("skips = %v", w.Skips)
	}

	// The day shows everyone's quantity and kcal, in the answer of a change.
	save("90")
	day := skip("1", alice).Body.String()
	if !strings.Contains(day, "Giulia R.") || !strings.Contains(day, "kcal") {
		t.Errorf("the day card does not show the household:\n%s", day)
	}

	// Removing a person takes what they changed along; Bob cannot remove Alice's.
	if w := call(a.handleHouseholdDelete, http.MethodPost, "/settings/household/x/delete", nil, map[string]int64{"id": giulia}, bob, false); w.Code != http.StatusNotFound {
		t.Errorf("Bob removing Alice's person: %d, want 404", w.Code)
	}
	if w := call(a.handleHouseholdDelete, http.MethodPost, "/settings/household/x/delete", nil, map[string]int64{"id": giulia}, alice, false); w.Code != http.StatusSeeOther {
		t.Errorf("remove: %d", w.Code)
	}
	if p, _ := a.Household.List(ctx, alice.ID); len(p) != 0 {
		t.Errorf("people left: %+v", p)
	}
}
