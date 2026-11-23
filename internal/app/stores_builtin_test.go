package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/stores"
)

// An integrated store (#277) runs inside Nutreek: it is connected with one click, needs no
// address, token or secret key, answers the same contract, and its price shows on the list
// under a notice that the prices are indicative.
func TestIntegratedStore(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")

	// The store: the fake plugin's answers, with no token asked for.
	srv, _, _ := fakePlugin(t)
	a.StoreClient.Builtin = map[string]http.Handler{"conad": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, srv.URL+r.URL.Path+"?"+r.URL.RawQuery, nil)
		req.Header.Set("Authorization", "Bearer secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		buf := make([]byte, 1<<16)
		n, _ := resp.Body.Read(buf)
		_, _ = w.Write(buf[:n])
	})}
	a.StoreBuiltins = []StoreBuiltin{
		{ID: "conad", Name: "Conad", Description: "x", Site: "https://spesaonline.conad.it"},
		{ID: "newshop", Name: "Newshop", Description: "Plugin's own description", Site: "https://newshop.example"},
	}

	jars := map[int64][]*http.Cookie{} // each user's cookies, kept between requests as a browser does
	do := func(h http.HandlerFunc, method, target string, v url.Values, params map[string]string, userID int64) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(v.Encode()))
		for _, c := range jars[userID] {
			r.AddCookie(c)
		}
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		rctx := chi.NewRouteContext()
		for k, val := range params {
			rctx.URLParams.Add(k, val)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		u := alice
		if userID == bob.ID {
			u = bob
		}
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, requestWithUser(r, u))
		jars[userID] = append(jars[userID], w.Result().Cookies()...)
		return w
	}

	// Connecting needs no secret key (there is none here), and twice is once.
	if a.StoreBox != nil {
		t.Fatal("the test app has a secret key")
	}
	conadID := map[string]string{"id": "conad"}
	if w := do(a.handleStoresBuiltin, http.MethodPost, "/settings/stores/builtin/conad", nil, conadID, alice.ID); w.Code != http.StatusSeeOther {
		t.Fatalf("connect: %d", w.Code)
	}
	do(a.handleStoresBuiltin, http.MethodPost, "/settings/stores/builtin/conad", nil, conadID, alice.ID)
	list, _ := a.StorePlugins.List(ctx, alice.ID)
	if len(list) != 1 || !list[0].Builtin() || list[0].BaseURL != stores.BuiltinURL("conad") || list[0].Name != "Conad" {
		t.Fatalf("plugins = %+v", list)
	}
	if w := do(a.handleStoresBuiltin, http.MethodPost, "/settings/stores/builtin/nope", nil, map[string]string{"id": "nope"}, alice.ID); w.Code != http.StatusNotFound {
		t.Errorf("an unknown integrated store: %d, want 404", w.Code)
	}
	page := do(a.handleStoresPage, http.MethodGet, "/settings/stores", nil, nil, alice.ID).Body.String()
	if !strings.Contains(page, "Collegato") || strings.Contains(page, "/settings/stores/builtin/conad") {
		t.Errorf("the page does not show it connected:\n%s", page)
	}
	if !strings.Contains(page, "Plugin&#39;s own description") && !strings.Contains(page, "Plugin's own description") {
		t.Error("a plugin with no catalog text shows no description")
	}
	if strings.Contains(page, "stores.builtin.desc") {
		t.Error("a catalog key is shown as text")
	}
	if bobPage := do(a.handleStoresPage, http.MethodGet, "/settings/stores", nil, nil, bob.ID).Body.String(); strings.Contains(bobPage, "Collegato") || !strings.Contains(bobPage, "/settings/stores/builtin/conad") {
		t.Error("Bob sees Alice's connection, or cannot connect his own")
	}

	// It cannot be edited into an address, and its test and delete work like any store's. The
	// secret key is set here so that the handler's own guard is the one that answers.
	a.StoreBox, _ = stores.NewBox("0123456789abcdef")
	id := map[string]string{"id": strconv.FormatInt(list[0].ID, 10)}
	do(a.handleStoresSave, http.MethodPost, "/settings/stores/x", url.Values{"name": {"Mine"}, "url": {"https://evil.example"}, "token": {"x"}}, id, alice.ID)
	if p, _ := a.StorePlugins.Get(ctx, alice.ID, list[0].ID); p.BaseURL != stores.BuiltinURL("conad") || p.Name != "Conad" {
		t.Errorf("an integrated store was edited: %+v", p)
	}
	if err := a.StorePlugins.Update(ctx, alice.ID, list[0].ID, "Mine", "https://evil.example", "x"); !errors.Is(err, db.ErrStoreNotFound) {
		t.Errorf("the repository edited an integrated store: %v", err)
	}
	if p, _ := a.StorePlugins.Get(ctx, alice.ID, list[0].ID); p.BaseURL != stores.BuiltinURL("conad") {
		t.Errorf("the address changed: %+v", p)
	}
	if w := do(a.handleStoresTest, http.MethodPost, "/settings/stores/x/test", nil, id, alice.ID); w.Code != http.StatusSeeOther {
		t.Errorf("test: %d", w.Code)
	}

	// Searching and importing go through it, with no token.
	uctx := contextWithUser(ctx, alice)
	a.Cfg.StoresAllowPrivate = true
	_ = a.StorePlugins.SetOFFEnabled(ctx, alice.ID, false)
	rows, warn, err := a.searchPicker(uctx, "avena", true)
	if err != nil || warn != "" || len(rows) != 1 || rows[0].SourceLabel() != "Conad" || rows[0].Price() != 189 {
		t.Fatalf("search = %v, %q, %v", rows, warn, err)
	}
	if rows, _, _ := a.searchPicker(contextWithUser(ctx, bob), "avena", true); len(rows) != 0 {
		t.Errorf("Bob found %d products in Alice's integrated store", len(rows))
	}
	ing, err := a.resolveOFFBarcode(uctx, rows[0].Barcode())
	if err != nil || ing == 0 {
		t.Fatalf("import: %d, %v", ing, err)
	}

	// The list prices it, under a notice that can be closed for good.
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	plan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, alice.ID, monday)
	meal, _ := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 0, "Breakfast")
	if _, err := a.WeeklyPlans.AddItem(ctx, meal, alice.ID, nil, &ing, 100, "g"); err != nil {
		t.Fatal(err)
	}
	shop := func() string {
		return do(a.handleShoppingList, http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil, nil, alice.ID).Body.String()
	}
	body := shop()
	if !strings.Contains(body, "data-prices-notice") || !strings.Contains(body, "indicativi") || !strings.Contains(body, `name="forever" value="1"`) {
		t.Errorf("the list has no notice about indicative prices:\n%s", body)
	}
	if w := do(a.handlePricesNotice, http.MethodPost, "/shopping-list/prices-notice?week=2026-10-12&weeks=1", url.Values{"forever": {"0"}}, nil, alice.ID); w.Code != http.StatusSeeOther {
		t.Errorf("close for now: %d", w.Code)
	}
	if hidden, _ := a.StorePlugins.PricesNoticeHidden(ctx, alice.ID); hidden {
		t.Error("closing for now hid it for good")
	}
	if strings.Contains(shop(), "data-prices-notice") {
		t.Error("the notice shows again in the same session after «Close»")
	}
	jars[alice.ID] = nil // a new sign-in: the notice is back
	if !strings.Contains(shop(), "data-prices-notice") {
		t.Error("«Close» hid the notice for good, not for the visit")
	}
	do(a.handlePricesNotice, http.MethodPost, "/shopping-list/prices-notice?week=2026-10-12&weeks=1", url.Values{"forever": {"1"}}, nil, alice.ID)
	if hidden, _ := a.StorePlugins.PricesNoticeHidden(ctx, alice.ID); !hidden {
		t.Error("closing for good did not stick")
	}
	if strings.Contains(shop(), "data-prices-notice") {
		t.Error("the notice shows after it was closed for good")
	}
	if hidden, _ := a.StorePlugins.PricesNoticeHidden(ctx, bob.ID); hidden {
		t.Error("Alice's choice hid Bob's notice")
	}
}

// Two clicks on «Connect» at once make one connection.
func TestAddBuiltinRace(t *testing.T) {
	a := newTestApp(t)
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	ids := make([]int64, 8)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], _ = a.StorePlugins.AddBuiltin(context.Background(), alice.ID, "conad", "Conad")
		}()
	}
	wg.Wait()
	list, _ := a.StorePlugins.List(context.Background(), alice.ID)
	if len(list) != 1 {
		t.Fatalf("%d connections from 8 clicks: %+v", len(list), list)
	}
	for _, id := range ids {
		if id != list[0].ID {
			t.Errorf("a click got %d, the connection is %d", id, list[0].ID)
		}
	}
}
