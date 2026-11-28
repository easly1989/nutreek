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

func withIDParam(r *http.Request, id int64) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// The quantity editors post to two handlers: a valid post redirects back to
// the list, an invalid one flashes (HX-Redirect for htmx), and another
// user's item is not found.
func TestShoppingEditHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, err := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Candele", 3, "pz"); err != nil {
		t.Fatal(err)
	}
	full, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	itemID := full.UserItems[0].ID

	post := func(h http.HandlerFunc, form url.Values, user int, htmx bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x?week=2026-10-12&weeks=1", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			r.Header.Set("HX-Request", "true")
		}
		u := alice
		if user == 2 {
			u = bob
		}
		r = withIDParam(requestWithUser(r, u), itemID)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, r)
		return w
	}
	edit := a.handleShoppingUserItemEdit

	// Valid: name, number with a comma, unit.
	w := post(edit, url.Values{"description": {"Candele grandi"}, "quantity": {"2,5"}, "qty_unit": {"pz"}}, 1, false)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("valid edit: code %d", w.Code)
	}
	got, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	if u := got.UserItems[0]; u.Description != "Candele grandi" || u.Quantity != 2.5 || u.Unit != "pz" {
		t.Errorf("stored = %+v", u)
	}

	// Free text branch.
	w = post(edit, url.Values{"description": {"Candele grandi"}, "quantity_text": {"una confezione"}}, 1, false)
	got, _ = a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	if w.Code != http.StatusSeeOther || got.UserItems[0].QuantityText != "una confezione" || got.UserItems[0].Unit != "" {
		t.Errorf("free text: code %d, %+v", w.Code, got.UserItems[0])
	}

	// Invalid: empty name, bad number (plain: 303; htmx: 204 + HX-Redirect).
	for _, form := range []url.Values{
		{"description": {" "}, "quantity": {"1"}},
		{"description": {"x"}, "quantity": {"abc"}},
	} {
		if w := post(edit, form, 1, false); w.Code != http.StatusSeeOther {
			t.Errorf("invalid %v: code %d, want 303", form, w.Code)
		}
		if w := post(edit, form, 1, true); w.Code != http.StatusNoContent || w.Header().Get("HX-Redirect") == "" {
			t.Errorf("invalid %v with htmx: code %d, HX-Redirect %q", form, w.Code, w.Header().Get("HX-Redirect"))
		}
	}

	// Another user's item does not exist.
	if w := post(edit, url.Values{"description": {"x"}, "quantity": {"1"}, "qty_unit": {"g"}}, 2, false); w.Code != http.StatusNotFound {
		t.Errorf("Bob edits Alice's item: code %d, want 404", w.Code)
	}
	if w := post(a.handleShoppingItemQuantity, url.Values{"quantity": {"1"}, "qty_unit": {"pz"}}, 2, false); w.Code != http.StatusNotFound {
		t.Errorf("Bob edits Alice's quantity: code %d, want 404", w.Code)
	}
	// Zero and bad quantities on a computed item flash instead of saving.
	if w := post(a.handleShoppingItemQuantity, url.Values{"quantity": {"0"}}, 1, true); w.Header().Get("HX-Redirect") == "" {
		t.Errorf("zero quantity: want HX-Redirect, code %d", w.Code)
	}
}

// POST /shopping-list/grouping saves the choice and goes back to the list (303,
// week and weeks kept); an unknown value is refused (400); the page then shows
// the groups, with «Altro» for the manual items.
func TestShoppingGroupingHandler(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, err := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Candele", 2, "pz"); err != nil {
		t.Fatal(err)
	}

	post := func(group string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/shopping-list/grouping?week=2026-10-12&weeks=1", strings.NewReader(url.Values{"group": {group}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingGrouping)).ServeHTTP(w, requestWithUser(r, alice))
		return w
	}
	if w := post("alphabetical"); w.Code != http.StatusBadRequest {
		t.Errorf("unknown value: code %d, want 400", w.Code)
	}
	w := post("category")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/shopping-list?week=2026-10-12&weeks=1" {
		t.Errorf("save: code %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	if g, _ := a.Users.ShoppingGrouping(ctx, alice.ID); g != "category" {
		t.Fatalf("saved = %q", g)
	}

	r := httptest.NewRequest(http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil)
	w = httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingList)).ServeHTTP(w, requestWithUser(r, alice))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "Candele") || !strings.Contains(body, `id="shop-group-category"`) {
		t.Fatalf("grouped page: code %d", w.Code)
	}
}

// POST /shopping-list/{id}/order saves the order and answers 204; another
// user's list and a bad id are 404; too many keys are 400.
func TestShoppingOrderHandler(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, err := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Candele", 2, "pz"); err != nil {
		t.Fatal(err)
	}
	full, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	key := "u" + strconv.FormatInt(full.UserItems[0].ID, 10)

	post := func(user int, id int64, ids string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/shopping-list/x/order", strings.NewReader(url.Values{"ids": {ids}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		u := alice
		if user == 2 {
			u = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingOrder)).ServeHTTP(w, withIDParam(requestWithUser(r, u), id))
		return w
	}
	if w := post(1, list.ID, key); w.Code != http.StatusNoContent {
		t.Fatalf("save: code %d, want 204", w.Code)
	}
	if got, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID); got.Order[key] != 0 {
		t.Errorf("stored order = %v", got.Order)
	}
	if w := post(2, list.ID, key); w.Code != http.StatusNotFound {
		t.Errorf("Bob: code %d, want 404", w.Code)
	}
	if w := post(1, 999999, key); w.Code != http.StatusNotFound {
		t.Errorf("missing list: code %d, want 404", w.Code)
	}
	if w := post(1, list.ID, strings.Repeat("i1,", 2001)); w.Code != http.StatusBadRequest {
		t.Errorf("too many keys: code %d, want 400", w.Code)
	}
}

// The ignore actions redirect back to the list (303), act on the owner's
// items only (404 for another user) and change the item.
func TestShoppingSwipeHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, err := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Candele", 2, "pz"); err != nil {
		t.Fatal(err)
	}
	full, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	userItem := full.UserItems[0].ID

	post := func(h http.HandlerFunc, user int, id int64) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x?week=2026-10-12&weeks=1", nil)
		u := alice
		if user == 2 {
			u = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, withIDParam(requestWithUser(r, u), id))
		return w
	}
	if w := post(a.handleShoppingUserItemIgnore, 2, userItem); w.Code != http.StatusNotFound {
		t.Errorf("Bob ignores Alice's item: code %d, want 404", w.Code)
	}
	w := post(a.handleShoppingUserItemIgnore, 1, userItem)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/shopping-list?week=2026-10-12&weeks=1" {
		t.Errorf("ignore: code %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	if got, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID); !got.UserItems[0].Ignored {
		t.Error("the manual item must be ignored")
	}
	if w := post(a.handleShoppingIgnore, 1, 999999); w.Code != http.StatusNotFound {
		t.Errorf("ignoring a missing item: code %d, want 404", w.Code)
	}
}

// When every item is ignored the list still shows them, so they can be restored.
func TestShoppingListShowsIgnoredOnly(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, err := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Candele", 2, "pz"); err != nil {
		t.Fatal(err)
	}
	full, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	if err := a.ShoppingLists.ToggleUserItemIgnored(ctx, full.UserItems[0].ID, alice.ID); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil)
	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingList)).ServeHTTP(w, requestWithUser(r, alice))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "Candele") || !strings.Contains(body, "/user-items/") {
		t.Errorf("an all-ignored list must still list its items: code %d", w.Code)
	}
	if !strings.Contains(body, `data-shop-acc="skipped"`) || strings.Contains(body, `data-shop-acc="cart"`) {
		t.Error("the ignored item must be in the «not needed» section, and there is no cart section")
	}
	if !strings.Contains(body, "data-shop-search") || !strings.Contains(body, "shop-dock") {
		t.Error("the page needs the search and the dock")
	}
}
