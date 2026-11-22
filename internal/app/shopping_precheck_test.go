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

// The check before shopping shows every row with its three answers, and posting
// them sets rows aside or lowers them; a «some» without a quantity flashes and
// goes back to the check; another user's list is a 404.
func TestShoppingPrecheckHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, _ := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	_, _ = a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Candele", 4, "pz")
	_, _ = a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Pepe", 100, "g")
	full, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	key := map[string]string{}
	for _, u := range full.UserItems {
		key[u.Description] = "u" + strconv.FormatInt(u.ID, 10)
	}

	get := func(q string) string {
		r := httptest.NewRequest(http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1"+q, nil)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingList)).ServeHTTP(w, requestWithUser(r, alice))
		return w.Body.String()
	}
	post := func(user int, form url.Values, htmx bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x?week=2026-10-12&weeks=1", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			r.Header.Set("HX-Request", "true")
		}
		u := alice
		if user == 2 {
			u = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingPrecheck)).ServeHTTP(w, withIDParam(requestWithUser(r, u), list.ID))
		return w
	}

	if body := get(""); !strings.Contains(body, "check=1") || strings.Contains(body, "data-precheck") {
		t.Error("the list must offer the check, and not be the check")
	}
	body := get("&check=1")
	if !strings.Contains(body, "data-precheck") || !strings.Contains(body, `name="st-`+key["Candele"]+`"`) || !strings.Contains(body, `name="have-`+key["Pepe"]+`"`) {
		t.Fatal("the check must show the answers of every row")
	}

	form := url.Values{"row": {key["Candele"], key["Pepe"]}, "st-" + key["Candele"]: {"some"}, "have-" + key["Candele"]: {"1"}, "unit-" + key["Candele"]: {"pz"}, "st-" + key["Pepe"]: {"have"}}
	if w := post(2, form, false); w.Code != http.StatusNotFound {
		t.Errorf("Bob checks Alice's list: code %d, want 404", w.Code)
	}
	// «Some» without a quantity: nothing applied, back to the check with a flash.
	bad := url.Values{"row": {key["Candele"]}, "st-" + key["Candele"]: {"some"}, "unit-" + key["Candele"]: {"pz"}}
	w := post(1, bad, true)
	if w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("HX-Redirect"), "check=1") {
		t.Errorf("a «some» without quantity: code %d, HX-Redirect %q", w.Code, w.Header().Get("HX-Redirect"))
	}
	if w := post(1, form, false); w.Code != http.StatusSeeOther || strings.Contains(w.Header().Get("Location"), "check") {
		t.Fatalf("check: code %d, location %q (it goes back to the list)", w.Code, w.Header().Get("Location"))
	}
	got, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	for _, u := range got.UserItems {
		switch u.Description {
		case "Candele":
			if u.Quantity != 3 || u.Ignored {
				t.Errorf("candles: %+v, want 3 left", u)
			}
		case "Pepe":
			if !u.Ignored {
				t.Errorf("pepper: %+v, want set aside", u)
			}
		}
	}
}
