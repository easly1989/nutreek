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
)

func withParams(r *http.Request, kv ...string) *http.Request {
	rctx := chi.NewRouteContext()
	for i := 0; i+1 < len(kv); i += 2 {
		rctx.URLParams.Add(kv[i], kv[i+1])
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// Replacing the ingredient of a row (#227): the dialog and its suggestions show
// the differences, the post puts the new ingredient in the same row with the same
// weight, and another user's row or recipe is out of reach.
func TestReplaceIngredientHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	mk := func(name string, kcal, p, c, f float64) int64 {
		id, err := a.Ingredients.CreateLocal(ctx, name, nil, "g", kcal, p, c, f, &alice.ID, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	greek := mk("Yogurt greco 0%", 58, 10, 4, 0.3)
	skyr := mk("Skyr naturale", 62, 11, 3.5, 0.2)
	ricotta := mk("Ricotta vaccina", 140, 8, 2.6, 11)
	dairy, _ := a.Categories.CreateCategory(ctx, alice.ID, "Latticini")
	for _, id := range []int64{greek, skyr} {
		if err := a.Categories.SetIngredientCategory(ctx, alice.ID, id, &dairy); err != nil {
			t.Fatal(err)
		}
	}

	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	plan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, alice.ID, monday)
	meal, _ := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 0, "Colazione")
	item, err := a.WeeklyPlans.AddItem(ctx, meal, alice.ID, nil, &greek, 150, "g")
	if err != nil {
		t.Fatal(err)
	}
	itemKey := strconv.FormatInt(item, 10)

	call := func(h http.HandlerFunc, method, target string, form url.Values, u int, kv ...string) *httptest.ResponseRecorder {
		var body *strings.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		} else {
			body = strings.NewReader("")
		}
		r := httptest.NewRequest(method, target, body)
		r.Header.Set("HX-Request", "true")
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

	// The dialog names the row, keeps the weight and posts to the replace route.
	form := call(a.handleMealItemReplaceForm, http.MethodGet, "/meal-items/"+itemKey+"/replace?start=2026-10-12", nil, 1, "itemID", itemKey)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "data-replace-form") ||
		!strings.Contains(form.Body.String(), "/meal-items/"+itemKey+"/replace?start=2026-10-12") || !strings.Contains(form.Body.String(), "Yogurt greco 0%") ||
		!strings.Contains(form.Body.String(), "150 g") ||
		// The list sits in a form that targets the day: it must not inherit that.
		!strings.Contains(form.Body.String(), `hx-trigger="load" hx-target="this"`) ||
		// ...nor hx-disabled-elt: the list inherits it, and with nothing to find htmx never ends its request.
		strings.Contains(form.Body.String(), "hx-disabled-elt") {
		t.Fatalf("dialog: %d %s", form.Code, form.Body.String())
	}

	// The suggestions: the same category first, the current one never, each with
	// its difference at 150 g (Skyr: +6 kcal).
	list := call(a.handleMealItemReplaceResults, http.MethodGet, "/meal-items/"+itemKey+"/replace/results?q=", nil, 1, "itemID", itemKey).Body.String()
	iSkyr, iRicotta := strings.Index(list, `value="ing:`+strconv.FormatInt(skyr, 10)+`"`), strings.Index(list, `value="ing:`+strconv.FormatInt(ricotta, 10)+`"`)
	if iSkyr < 0 || iRicotta < 0 || iSkyr > iRicotta {
		t.Fatalf("suggestions must hold Skyr before Ricotta: %d, %d\n%s", iSkyr, iRicotta, list)
	}
	if strings.Contains(list, `value="ing:`+strconv.FormatInt(greek, 10)+`"`) {
		t.Error("the current ingredient must not be suggested")
	}
	if !strings.Contains(list, "&#43;6 kcal") || !strings.Contains(list, "stessa categoria") && !strings.Contains(list, "same category") { // html/template writes + as &#43;
		t.Errorf("Skyr must show +6 kcal and its category:\n%s", list)
	}

	// A search lists matches with the same differences.
	found := call(a.handleMealItemReplaceResults, http.MethodGet, "/meal-items/"+itemKey+"/replace/results?q=ricotta", nil, 1, "itemID", itemKey).Body.String()
	if !strings.Contains(found, `value="ing:`+strconv.FormatInt(ricotta, 10)+`"`) || strings.Contains(found, `value="ing:`+strconv.FormatInt(skyr, 10)+`"`) {
		t.Errorf("search «ricotta» must list Ricotta only:\n%s", found)
	}

	// Another user's row is not found, in the dialog, the list and the post.
	if w := call(a.handleMealItemReplaceForm, http.MethodGet, "/x", nil, 2, "itemID", itemKey); w.Code != http.StatusNotFound {
		t.Errorf("dialog of another user's row: %d, want 404", w.Code)
	}
	if w := call(a.handleMealItemReplace, http.MethodPost, "/x?start=2026-10-12", url.Values{"choice": {"ing:" + strconv.FormatInt(skyr, 10)}}, 2, "itemID", itemKey); w.Code != http.StatusNotFound {
		t.Errorf("post on another user's row: %d, want 404", w.Code)
	}

	// Nothing chosen: back to the week with a flash, and the row is as it was.
	if w := call(a.handleMealItemReplace, http.MethodPost, "/x?start=2026-10-12", url.Values{"choice": {"nothing"}}, 1, "itemID", itemKey); w.Header().Get("HX-Redirect") == "" {
		t.Errorf("an empty choice must flash and go back: %d", w.Code)
	}

	// The post: same row, same weight, the day answers.
	w := call(a.handleMealItemReplace, http.MethodPost, "/x?start=2026-10-12", url.Values{"choice": {"ing:" + strconv.FormatInt(skyr, 10)}}, 1, "itemID", itemKey)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Skyr naturale") {
		t.Fatalf("replace: %d %s", w.Code, w.Body.String())
	}
	var ing int64
	var qty float64
	var unit string
	if err := a.Store.DB.QueryRowContext(ctx, `SELECT ingredient_id, quantity, quantity_unit FROM meal_items WHERE id = ?`, item).Scan(&ing, &qty, &unit); err != nil {
		t.Fatal(err)
	}
	if ing != skyr || qty != 150 || unit != "g" {
		t.Errorf("row = %d, %v %s; want Skyr, 150 g", ing, qty, unit)
	}

	// A recipe's row: only its author, and the row keeps its place.
	rec, _ := a.Recipes.Create(ctx, alice.ID, "Ciotola", nil, 1, false)
	row, err := a.Recipes.AddItem(ctx, rec, alice.ID, greek, 150, "g")
	if err != nil {
		t.Fatal(err)
	}
	recKey, rowKey := strconv.FormatInt(rec, 10), strconv.FormatInt(row, 10)
	post := func(u int) *httptest.ResponseRecorder {
		return call(a.handleRecipeReplaceItem, http.MethodPost, "/x", url.Values{"choice": {"ing:" + strconv.FormatInt(ricotta, 10)}}, u, "id", recKey, "itemID", rowKey)
	}
	if w := post(2); w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("another user's post on a recipe row: %d, want 403 or 404", w.Code)
	}
	// A recipe that would become identical to another of the author's is refused
	// with its own message, and the row stays as it was.
	other, _ := a.Recipes.Create(ctx, alice.ID, "Twin", nil, 1, false)
	if _, err := a.Recipes.AddItem(ctx, other, alice.ID, ricotta, 150, "g"); err != nil {
		t.Fatal(err)
	}
	if w := post(1); w.Code == http.StatusSeeOther || !strings.Contains(w.Body.String(), "Twin") {
		t.Fatalf("identical recipe: %d, want the page back with the message naming Twin", w.Code)
	}
	if untouched, _ := a.Recipes.GetWithItems(ctx, rec, alice.ID); untouched.Items[0].IngredientID != greek {
		t.Error("a refused replace must leave the row as it was")
	}
	// Choosing the ingredient the row already has writes nothing.
	if w := call(a.handleRecipeReplaceItem, http.MethodPost, "/x", url.Values{"choice": {"ing:" + strconv.FormatInt(greek, 10)}}, 1, "id", recKey, "itemID", rowKey); w.Code != http.StatusSeeOther {
		t.Errorf("same ingredient: %d, want a redirect", w.Code)
	}
	if err := a.Recipes.RemoveItem(ctx, func() int64 {
		g, _ := a.Recipes.GetWithItems(ctx, other, alice.ID)
		return g.Items[0].ID
	}(), alice.ID); err != nil {
		t.Fatal(err)
	}
	if w := post(1); w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "saved="+rowKey) {
		t.Fatalf("recipe replace: %d %q", w.Code, w.Header().Get("Location"))
	}
	full, _ := a.Recipes.GetWithItems(ctx, rec, alice.ID)
	if len(full.Items) != 1 || full.Items[0].ID != row || full.Items[0].IngredientID != ricotta || full.Items[0].Quantity != 150 {
		t.Errorf("recipe items = %+v, want the same row with Ricotta, 150", full.Items)
	}
	if rl := call(a.handleRecipeReplaceResults, http.MethodGet, "/x?q=", nil, 1, "id", recKey, "itemID", rowKey); rl.Code != http.StatusOK || !strings.Contains(rl.Body.String(), `value="ing:`+strconv.FormatInt(skyr, 10)+`"`) {
		t.Errorf("recipe suggestions: %d", rl.Code)
	}
}
