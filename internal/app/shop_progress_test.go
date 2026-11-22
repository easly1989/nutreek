package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The progress card carries the numbers shop_progress.js animates from, the bar is
// drawn at the right fill, and «Spesa completata» shows only when every item is in
// the cart (#248).
func TestShoppingProgressCard(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	list, err := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Candele", "Pepe"} {
		if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, name, 4, "pz"); err != nil {
			t.Fatal(err)
		}
	}

	page := func() string {
		r := httptest.NewRequest(http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingList)).ServeHTTP(w, requestWithUser(r, alice))
		return w.Body.String()
	}

	body := page()
	if len(strings.Split(body, "data-progress-fill")) != 2 {
		t.Fatal("the progress card must be drawn once")
	}
	if !strings.Contains(body, `aria-valuemax="2" aria-valuenow="0"`) || !strings.Contains(body, `data-list="`+strconv.FormatInt(list.ID, 10)+`"`) || !strings.Contains(body, "inset(0 100% 0 0") {
		t.Error("an empty cart must show 0 of 2 with an empty bar, tagged with its list")
	}
	if strings.Contains(body, "data-progress-done") {
		t.Error("«Spesa completata» must not show before the last item")
	}

	full, err := a.ShoppingLists.LoadFull(ctx, list.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, u := range full.UserItems {
		if err := a.ShoppingLists.ToggleUserItem(ctx, u.ID, alice.ID); err != nil {
			t.Fatal(err)
		}
		body = page()
		switch i {
		case 0:
			if !strings.Contains(body, `aria-valuemax="2" aria-valuenow="1"`) || !strings.Contains(body, "inset(0 50% 0 0") || strings.Contains(body, "data-progress-done") {
				t.Error("one of two must fill half of the bar, with no «Spesa completata»")
			}
		case len(full.UserItems) - 1:
			if !strings.Contains(body, `aria-valuemax="2" aria-valuenow="2"`) || !strings.Contains(body, "inset(0 0% 0 0") || !strings.Contains(body, "data-progress-done") {
				t.Error("a full cart must fill the bar and show «Spesa completata»")
			}
		}
	}

	// Nothing to count (every item ignored): no card, and the page's divs stay balanced.
	for _, u := range full.UserItems {
		if err := a.ShoppingLists.ToggleUserItem(ctx, u.ID, alice.ID); err != nil { // out of the cart again
			t.Fatal(err)
		}
		if err := a.ShoppingLists.ToggleUserItemIgnored(ctx, u.ID, alice.ID); err != nil {
			t.Fatal(err)
		}
	}
	body = page()
	if strings.Contains(body, `id="shop-progress"`) {
		t.Error("a list with nothing to count must not draw the progress card")
	}
	if opened, closed := strings.Count(body, "<div"), strings.Count(body, "</div>"); opened != closed {
		t.Errorf("unbalanced divs: %d opened, %d closed", opened, closed)
	}
}
