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

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// While typing, the field suggests what was added by hand before, with the
// part that was typed in bold; short or unknown text suggests nothing.
func TestShoppingSuggestHandler(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	monday, _ := time.Parse("2006-01-02", "2026-10-05")
	list, _ := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, monday, 1)
	if _, err := a.ShoppingLists.AddManualItem(ctx, list.ID, alice.ID, "Candele", 2, "pz"); err != nil {
		t.Fatal(err)
	}
	get := func(q string) string {
		r := httptest.NewRequest(http.MethodGet, "/shopping-list/suggest?description="+url.QueryEscape(q), nil)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingSuggest)).ServeHTTP(w, requestWithUser(r, alice))
		if w.Code != http.StatusOK {
			t.Fatalf("suggest %q: code %d", q, w.Code)
		}
		return w.Body.String()
	}
	body := get("can")
	if !strings.Contains(body, `data-desc="Candele"`) || !strings.Contains(body, `data-qty="2"`) || !strings.Contains(body, `data-unit="pz"`) || !strings.Contains(body, "<b>Can</b>dele") {
		t.Errorf("suggestion = %s", body)
	}
	if strings.TrimSpace(get("c")) != "" {
		t.Error("one letter must suggest nothing")
	}
	if strings.TrimSpace(get("zucchero")) != "" {
		t.Error("an unknown text must suggest nothing")
	}
}

// The same text in two past lists is ticked in the latest one only, and an item
// that was set aside starts unticked.
func TestPastCopyListsDedupes(t *testing.T) {
	w2, _ := time.Parse("2006-01-02", "2026-11-09")
	past := []db.PastManualList{
		{ListID: 2, WeekStart: w2, WeekCount: 1, Items: []domain.ShoppingUserItem{{ID: 1, Description: "Candele", Quantity: 4, Unit: "pz"}, {ID: 2, Description: "Sale", Ignored: true}}},
		{ListID: 1, WeekStart: w2.AddDate(0, 0, -7), WeekCount: 1, Items: []domain.ShoppingUserItem{{ID: 3, Description: " candele ", Quantity: 2, Unit: "pz"}, {ID: 4, Description: "Pepe"}}},
	}
	got := pastCopyLists(past, []domain.ShoppingUserItem{{Description: "PEPE"}})
	if len(got) != 2 || !got[0].Active || got[1].Active {
		t.Fatalf("tabs = %+v", got)
	}
	if !got[0].Items[0].Checked || got[0].Items[1].Checked || got[1].Items[0].Checked {
		t.Errorf("ticks = %+v", got)
	}
	if len(got[1].Items) != 1 {
		t.Errorf("an item already on the list was offered: %+v", got[1].Items)
	}
}

// The page offers to copy from the earlier lists once there is something to
// copy, and posting the ticked items adds them; nothing ticked flashes.
func TestShoppingCopyHandler(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	w1, _ := time.Parse("2006-01-02", "2026-10-05")
	w2 := w1.AddDate(0, 0, 7)
	old, _ := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, w1, 1)
	cur, _ := a.ShoppingLists.GetOrCreateList(ctx, alice.ID, w2, 1)
	_, _ = a.ShoppingLists.AddManualItem(ctx, old.ID, alice.ID, "Carta da forno", 2, "pz")
	_, _ = a.ShoppingLists.AddManualItem(ctx, old.ID, alice.ID, "Sale grosso", 1, "g")
	// Already on the current list: not offered again.
	_, _ = a.ShoppingLists.AddManualItem(ctx, cur.ID, alice.ID, "Sale grosso", 1, "g")
	full, _ := a.ShoppingLists.LoadFull(ctx, old.ID, alice.ID)
	idOf := func(name string) string {
		for _, u := range full.UserItems {
			if u.Description == name {
				return strconv.FormatInt(u.ID, 10)
			}
		}
		t.Fatalf("no item %q", name)
		return ""
	}

	page := func() string {
		r := httptest.NewRequest(http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil)
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingList)).ServeHTTP(w, requestWithUser(r, alice))
		return w.Body.String()
	}
	body := page()
	if !strings.Contains(body, "data-copy") || !strings.Contains(body, "Carta da forno") {
		t.Fatal("the page must offer to copy the items of the past list")
	}
	if strings.Contains(body, `value="`+idOf("Sale grosso")+`"`) {
		t.Error("an item already on the list must not be offered")
	}

	post := func(user int, form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x?week=2026-10-12&weeks=1", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		u := alice
		if user == 2 {
			u = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(http.HandlerFunc(a.handleShoppingCopy)).ServeHTTP(w, withIDParam(requestWithUser(r, u), cur.ID))
		return w
	}
	if w := post(1, url.Values{}); w.Code != http.StatusSeeOther {
		t.Errorf("nothing ticked: code %d (it flashes and goes back)", w.Code)
	}
	if w := post(2, url.Values{"item": {idOf("Carta da forno")}}); w.Code != http.StatusNotFound {
		t.Errorf("Bob copies into Alice's list: code %d, want 404", w.Code)
	}
	if w := post(1, url.Values{"item": {idOf("Carta da forno"), "abc", "-4"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("copy: code %d", w.Code)
	}
	got, _ := a.ShoppingLists.LoadFull(ctx, cur.ID, alice.ID)
	found := false
	for _, u := range got.UserItems {
		if u.Description == "Carta da forno" && u.Quantity == 2 && u.Unit == "pz" {
			found = true
		}
	}
	if !found || len(got.UserItems) != 2 {
		t.Errorf("after the copy: %+v", got.UserItems)
	}
}
