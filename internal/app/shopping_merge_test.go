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

// The page offers to merge two similar rows; merging and keeping them apart
// post to their handlers (303, 404 for another user's list, a flash through
// HX-Redirect for rows that cannot be merged).
func TestShoppingMergeHandlers(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, err := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Sale grosso", "Sale grosso marino"} {
		if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, n, 2, "pz"); err != nil {
			t.Fatal(err)
		}
	}
	full, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	k1 := "u" + strconv.FormatInt(full.UserItems[0].ID, 10)
	k2 := "u" + strconv.FormatInt(full.UserItems[1].ID, 10)

	get := func() string {
		r := httptest.NewRequest(http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingList)).ServeHTTP(w, requestWithUser(r, alice))
		return w.Body.String()
	}
	post := func(h http.HandlerFunc, user int, form url.Values, htmx bool) *httptest.ResponseRecorder {
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
		a.Sessions.LoadAndSave(h).ServeHTTP(w, withIDParam(requestWithUser(r, u), list.ID))
		return w
	}

	if body := get(); !strings.Contains(body, "data-similar") || !strings.Contains(body, "/merge?week=") {
		t.Fatal("the page must offer to merge two similar manual items")
	}
	// Keep them apart: no notice any more.
	if w := post(a.handleShoppingSimilarDismiss, 1, url.Values{"a": {k1}, "b": {k2}}, false); w.Code != http.StatusSeeOther {
		t.Fatalf("dismiss: code %d", w.Code)
	}
	if body := get(); strings.Contains(body, "data-similar") {
		t.Error("a pair kept apart must not be offered again")
	}
	if w := post(a.handleShoppingSimilarDismiss, 2, url.Values{"a": {k1}, "b": {k2}}, false); w.Code != http.StatusNotFound {
		t.Errorf("Bob dismisses on Alice's list: code %d, want 404", w.Code)
	}

	// Merge: 303; a bad pair flashes; Bob cannot.
	if w := post(a.handleShoppingMerge, 2, url.Values{"keep": {k1}, "drop": {k2}}, false); w.Code != http.StatusNotFound {
		t.Errorf("Bob merges on Alice's list: code %d, want 404", w.Code)
	}
	if w := post(a.handleShoppingMerge, 1, url.Values{"keep": {k1}, "drop": {"u999"}}, true); w.Code != http.StatusNoContent || w.Header().Get("HX-Redirect") == "" {
		t.Errorf("bad pair with htmx: code %d, HX-Redirect %q", w.Code, w.Header().Get("HX-Redirect"))
	}
	w := post(a.handleShoppingMerge, 1, url.Values{"keep": {k1}, "drop": {k2}}, false)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("merge: code %d, want 303", w.Code)
	}
	if got, _ := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID); len(got.UserItems) != 1 || got.UserItems[0].Quantity != 4 {
		t.Errorf("after merge: %+v", got.UserItems)
	}
}
