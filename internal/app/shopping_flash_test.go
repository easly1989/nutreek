package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// An error while adding a shopping item by hand reloads the page (HX-Redirect)
// when the request came from htmx, so the flash shows; a classic form gets the
// plain redirect.
func TestShoppingFlashBack(t *testing.T) {
	a := newTestApp(t)
	const back = "/shopping-list?week=2026-10-12&weeks=1"
	h := a.Sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.shoppingFlashBack(w, r, back, "shopping.flash.description_required")
	}))

	r := httptest.NewRequest(http.MethodPost, "/shopping-list/1/user-items", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || w.Header().Get("HX-Redirect") != back {
		t.Errorf("htmx: code %d, HX-Redirect %q", w.Code, w.Header().Get("HX-Redirect"))
	}

	r = httptest.NewRequest(http.MethodPost, "/shopping-list/1/user-items", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != back {
		t.Errorf("classic form: code %d, Location %q", w.Code, w.Header().Get("Location"))
	}
}
