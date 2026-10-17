package app

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

// enter starts a demo in b and returns the new user's id.
func enter(t *testing.T, a *App, b *browser) int64 {
	t.Helper()
	if code, loc := b.post("/", "/demo/session", nil); code != http.StatusSeeOther || loc != "/" {
		t.Fatalf("POST /demo/session: %d to %q", code, loc)
	}
	var id int64
	if err := a.Store.DB.QueryRow(`SELECT max(id) FROM users`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// switchedOff are the routes the demo does not mount (#185).
var switchedOff = []struct{ method, path string }{
	{http.MethodGet, "/admin/panel"},
	{http.MethodGet, "/admin/panel/users"},
	{http.MethodGet, "/admin/off-status"},
	{http.MethodPost, "/admin/refresh-off-dump"},
	{http.MethodPost, "/admin/reconcile-ingredients"},
	{http.MethodGet, "/api/v1/me"},
	{http.MethodPost, "/settings/api-tokens"},
	{http.MethodPost, "/settings/api-tokens/1/revoke"},
	{http.MethodPost, "/recipes/import"},
	{http.MethodGet, "/profile/off-status"},
	{http.MethodPost, "/profile/off-sync"},
}

// What the demo has no use for is not mounted, and nothing links it.
func TestDemoSwitchedOff(t *testing.T) {
	// The normal build has these routes: signed out, none of them is a 404
	// (but the legacy admin ones, which hide without NUTREEK_ADMIN_TOKEN).
	normal := newBrowser(t, newTestApp(t))
	for _, r := range switchedOff {
		if r.method == http.MethodGet && r.path != "/admin/off-status" {
			if code, _, _ := normal.get(r.path); code == http.StatusNotFound {
				t.Errorf("normal build: GET %s is 404", r.path)
			}
		}
	}

	a := newDemoApp(t, DemoLimits{})
	b := newBrowser(t, a)
	enter(t, a, b)
	for _, r := range switchedOff {
		var code int
		if r.method == http.MethodGet {
			code, _, _ = b.get(r.path)
		} else {
			code, _ = b.post("/settings", r.path, nil)
		}
		if code != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", r.method, r.path, code)
		}
	}

	for _, page := range []string{"/", "/settings", "/recipes", "/recipes/new", "/ingredients", "/profile"} {
		_, _, body := b.get(page)
		for _, link := range []string{"/admin", "/api-tokens", "/api/v1", "/recipes/import", "#importa", `href="#token"`, `id="token"`, "/profile/off-"} {
			if strings.Contains(body, link) {
				t.Errorf("%s links %s", page, link)
			}
		}
	}
}

func TestDemoStartsPerAddress(t *testing.T) {
	a := newDemoApp(t, DemoLimits{StartsPerHour: 2})
	b := newBrowser(t, a)
	enter(t, a, b)
	enter(t, a, b)
	if code, _ := b.post("/", "/demo/session", nil); code != http.StatusTooManyRequests {
		t.Errorf("third start in the hour: %d, want 429", code)
	}
}

func TestDemoFull(t *testing.T) {
	a := newDemoApp(t, DemoLimits{MaxUsers: 1})
	enter(t, a, newBrowser(t, a))
	code, _, body := newBrowser(t, a).submit("/", "/demo/session", nil)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "The demo is full") || !strings.Contains(body, `action="/demo/session"`) {
		t.Errorf("start on a full demo: %d, want 503 and the entry saying so", code)
	}
	var n int
	_ = a.Store.DB.QueryRow(`SELECT count(*) FROM users`).Scan(&n)
	if n != 1 {
		t.Errorf("%d users, want 1", n)
	}
}

func TestDemoBodyLimit(t *testing.T) {
	a := newDemoApp(t, DemoLimits{MaxBody: 1024})
	b := newBrowser(t, a)
	enter(t, a, b)
	form := url.Values{"name": {strings.Repeat("x", 2000)}}
	if code, _ := b.post("/ingredients/new", "/ingredients", form); code != http.StatusRequestEntityTooLarge {
		t.Errorf("2 KB body: %d, want 413", code)
	}
}

func TestDemoCaps(t *testing.T) {
	a := newDemoApp(t, DemoLimits{MaxIngredients: 1, MaxRecipes: 1})
	b := newBrowser(t, a)
	uid := enter(t, a, b)
	count := func(q string) int {
		var n int
		if err := a.Store.DB.QueryRow(q, uid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	ingredient := func(name string) url.Values {
		return url.Values{"name": {name}, "unit": {"g"}, "kcal": {"100"}, "protein": {"1"}, "carbs": {"1"}, "fat": {"1"}}
	}

	if code, loc := b.post("/ingredients/new", "/ingredients", ingredient("First")); code != http.StatusSeeOther || loc != "/ingredients" {
		t.Fatalf("first ingredient: %d to %q", code, loc)
	}
	if code, loc := b.post("/ingredients/new", "/ingredients", ingredient("Second")); code != http.StatusSeeOther || loc != "/ingredients" {
		t.Errorf("second ingredient: %d to %q, want back to the list", code, loc)
	}
	b.post("/ingredients/new", "/quick/ingredient", ingredient("Third"))
	if n := count(`SELECT count(*) FROM ingredients WHERE created_by = ?`); n != 1 {
		t.Errorf("%d ingredients, want 1", n)
	}
	if _, _, list := b.get("/ingredients"); !strings.Contains(list, "at most 1 ingredient") {
		t.Error("the list does not say why")
	}

	if code, _ := b.post("/recipes/new", "/recipes", url.Values{"name": {"Bowl"}, "servings": {"1"}}); code != http.StatusSeeOther {
		t.Fatalf("first recipe: %d", code)
	}
	b.post("/recipes/new", "/recipes", url.Values{"name": {"Soup"}, "servings": {"1"}})
	b.post("/recipes/new", "/quick/recipe", url.Values{"name": {"Salad"}, "servings": {"1"}})
	var bowl int64
	_ = a.Store.DB.QueryRow(`SELECT id FROM recipes WHERE user_id = ?`, uid).Scan(&bowl)
	b.post("/recipes", "/recipes/"+strconv.FormatInt(bowl, 10)+"/duplicate", url.Values{"name": {"Bowl 2"}})
	if n := count(`SELECT count(*) FROM recipes WHERE user_id = ?`); n != 1 {
		t.Errorf("%d recipes, want 1", n)
	}
}

// An ingredient or public recipe made in one demo group is neither seen nor
// changed from another; OpenFoodFacts products are seen by all, changed by none.
func TestDemoGroupScope(t *testing.T) {
	ctx := context.Background()
	a := newDemoApp(t, DemoLimits{})
	alice, bob := newBrowser(t, a), newBrowser(t, a)
	aliceID := enter(t, a, alice)
	bobID := enter(t, a, bob)

	paste, err := a.Ingredients.CreateLocal(ctx, "Zesty paste", nil, domain.UnitGram, 100, 1, 1, 1, &aliceID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	offBar, err := a.Ingredients.CreateOFF(ctx, "8000000000001", "Zesty bar", nil, domain.UnitGram, 400, 5, 60, 15, &aliceID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	bowl, err := a.Recipes.Create(ctx, aliceID, "Zesty bowl", nil, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	bobSoup, err := a.Recipes.Create(ctx, bobID, "Bob soup", nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt

	// Alice's group sees its own.
	if _, _, body := alice.get("/ingredients/search?q=Zesty"); !strings.Contains(body, "Zesty paste") || !strings.Contains(body, "Zesty bar") {
		t.Error("Alice does not find her ingredients")
	}
	if _, _, body := alice.get("/recipes?f=public"); !strings.Contains(body, "Zesty bowl") {
		t.Error("Alice does not see her recipe")
	}

	// Bob's group does not see Alice's hand-made ingredient or public recipe.
	for _, page := range []string{"/ingredients/search?q=Zesty", "/ingredients/search?q=", "/ingredients", "/recipes/" + id(bobSoup, 10) + "/picker?q=Zesty"} {
		code, _, body := bob.get(page)
		if strings.Contains(body, "Zesty paste") {
			t.Errorf("Bob sees Alice's ingredient on %s (%d)", page, code)
		}
	}
	if _, _, body := bob.get("/ingredients/search?q=Zesty"); !strings.Contains(body, "Zesty bar") {
		t.Error("Bob does not see the OpenFoodFacts product")
	}
	for _, page := range []string{"/recipes?f=others"} {
		if _, _, body := bob.get(page); strings.Contains(body, "Zesty bowl") {
			t.Errorf("Bob sees Alice's public recipe on %s", page)
		}
	}

	// ...nor opens or changes them by id.
	for _, page := range []string{"/ingredients/" + id(paste, 10) + "/edit", "/recipes/" + id(bowl, 10), "/recipes/" + id(bowl, 10) + "/duplicate"} {
		if code, _, _ := bob.get(page); code != http.StatusNotFound {
			t.Errorf("Bob opens %s: %d, want 404", page, code)
		}
	}
	rename := url.Values{"name": {"Renamed"}, "unit": {"g"}, "kcal": {"1"}, "protein": {"1"}, "carbs": {"1"}, "fat": {"1"}, "servings": {"1"}}
	bob.post("/recipes", "/ingredients/"+id(paste, 10), rename)
	bob.post("/recipes", "/recipes/"+id(bowl, 10), rename)
	bob.post("/recipes", "/recipes/"+id(bowl, 10)+"/duplicate", url.Values{"name": {"Mine now"}})
	bob.post("/recipes", "/recipes/"+id(bobSoup, 10)+"/items", url.Values{"ingredient_id": {id(paste, 10)}, "quantity": {"100"}})
	if ing, _ := a.Ingredients.GetByID(ctx, paste); ing.Name != "Zesty paste" {
		t.Errorf("Bob renamed Alice's ingredient to %q", ing.Name)
	}
	if rec, _ := a.Recipes.GetWithItems(ctx, bowl, aliceID); rec.Name != "Zesty bowl" {
		t.Errorf("Bob renamed Alice's recipe to %q", rec.Name)
	}
	var leaks int
	_ = a.Store.DB.QueryRow(`SELECT (SELECT count(*) FROM recipes WHERE user_id = ?) + (SELECT count(*) FROM recipe_ingredients WHERE ingredient_id = ?)`, bobID, paste).Scan(&leaks)
	if leaks != 1 { // Bob's own soup only
		t.Errorf("Bob duplicated Alice's recipe or used her ingredient (%d rows)", leaks-1)
	}

	// A code linked to Alice's ingredient tells Bob neither its name nor the row.
	if err := a.Ingredients.SetLocalBarcode(ctx, paste, "8000000000002"); err != nil {
		t.Fatal(err)
	}
	if _, _, body := bob.get("/ingredients/search?q=8000000000002"); strings.Contains(body, "Zesty paste") {
		t.Error("Bob finds Alice's ingredient by its code")
	}
	quick := url.Values{"name": {"Mine"}, "unit": {"g"}, "kcal": {"1"}, "protein": {"1"}, "carbs": {"1"}, "fat": {"1"}, "barcode": {"8000000000002"}}
	if _, _, body := bob.submit("/recipes", "/quick/ingredient", quick); strings.Contains(body, "Zesty paste") || !strings.Contains(body, "another ingredient") {
		t.Error("the quick form names Alice's ingredient to Bob, or does not say the code is taken")
	}

	// The OpenFoodFacts product is read-only, even for whoever imported it.
	alice.post("/ingredients", "/ingredients/"+id(offBar, 10), rename)
	if ing, _ := a.Ingredients.GetByID(ctx, offBar); ing.Name != "Zesty bar" {
		t.Errorf("an OpenFoodFacts product was renamed to %q", ing.Name)
	}
}
