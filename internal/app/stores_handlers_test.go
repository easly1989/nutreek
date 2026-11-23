package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/stores"
)

const oatsProduct = `{"id":"p1","name":"Fiocchi d'avena","brand":"Conad","size":{"quantity":500,"unit":"g"},"price":1.89,"available":true,
 "nutrition":{"kcal":372,"protein":13.5,"carbs":59,"fat":7,"fiber":10}}`

// fakePlugin is a store plugin: the token is "secret"; the price an availability call
// answers can be changed, and calls are counted.
func fakePlugin(t *testing.T) (srv *httptest.Server, price *atomic.Int64, calls *atomic.Int64) {
	t.Helper()
	price, calls = new(atomic.Int64), new(atomic.Int64)
	price.Store(199)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/search":
			w.Write([]byte(`{"products":[` + oatsProduct + `,{"id":"p2","name":"No macros"}]}`))
		case "/product":
			if r.URL.Query().Get("id") == "p1" {
				w.Write([]byte(oatsProduct))
				return
			}
			http.NotFound(w, r)
		case "/availability":
			w.Write([]byte(`{"products":[{"id":"p1","price":` + strconv.FormatFloat(float64(price.Load())/100, 'f', 2, 64) + `,"available":false}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, price, calls
}

// Store plugins (#239): connect, search through them, make an ingredient of a product,
// price a shopping list with it, refresh the price; everything scoped to the user.
func TestStorePluginsFlow(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	bob := mustCreateTestUser(t, a, "bob@x.com", "password-di-prova")
	srv, price, _ := fakePlugin(t)
	a.Cfg.StoresAllowPrivate = true // the fake plugin is http on a loopback address

	call := func(h http.HandlerFunc, method, target string, v url.Values, params map[string]string, who *domain.User) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(v.Encode()))
		if method == http.MethodPost {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		rctx := chi.NewRouteContext()
		for k, val := range params {
			rctx.URLParams.Add(k, val)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		a.Sessions.LoadAndSave(h).ServeHTTP(w, requestWithUser(r, who))
		return w
	}
	plugins := func(u *domain.User) []string {
		list, _ := a.StorePlugins.List(ctx, u.ID)
		var names []string
		for _, p := range list {
			names = append(names, p.Name)
		}
		return names
	}
	form := func(name, addr, token string) url.Values {
		return url.Values{"name": {name}, "url": {addr}, "token": {token}}
	}

	// Without NUTREEK_SECRET_KEY nothing can be connected.
	call(a.handleStoresSave, http.MethodPost, "/settings/stores", form("Conad", srv.URL, "secret"), nil, alice)
	if len(plugins(alice)) != 0 {
		t.Fatal("a store was connected without a secret key")
	}
	box, err := stores.NewBox("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	a.StoreBox = box

	// Connect: the address is checked, the token is stored sealed, the page shows no token.
	call(a.handleStoresSave, http.MethodPost, "/settings/stores", form("Conad", "ftp://x", "secret"), nil, alice)
	call(a.handleStoresSave, http.MethodPost, "/settings/stores", form("Conad", srv.URL, ""), nil, alice)
	if len(plugins(alice)) != 0 {
		t.Fatalf("a bad address or an empty token was accepted: %v", plugins(alice))
	}
	if w := call(a.handleStoresSave, http.MethodPost, "/settings/stores", form("Conad Mestre", srv.URL+"/", "secret"), nil, alice); w.Code != http.StatusSeeOther {
		t.Fatalf("connect: %d", w.Code)
	}
	list, _ := a.StorePlugins.List(ctx, alice.ID)
	if len(list) != 1 || list[0].BaseURL != srv.URL || strings.Contains(list[0].TokenSealed, "secret") {
		t.Fatalf("plugins = %+v", list)
	}
	conad := list[0]
	page := call(a.handleStoresPage, http.MethodGet, "/settings/stores", nil, nil, alice).Body.String()
	if !strings.Contains(page, "Conad Mestre") || strings.Contains(page, list[0].TokenSealed) || strings.Contains(page, "secret") {
		t.Errorf("page: %s", page)
	}
	id := map[string]string{"id": strconv.FormatInt(conad.ID, 10)}
	if w := call(a.handleStoresSave, http.MethodPost, "/settings/stores/x", form("Mine", srv.URL, "x"), id, bob); w.Code != http.StatusNotFound {
		t.Errorf("Bob changing Alice's store: %d, want 404", w.Code)
	}
	call(a.handleStoresSave, http.MethodPost, "/settings/stores/x", form("Conad", srv.URL, ""), id, alice) // an empty token keeps it
	if p, _ := a.StorePlugins.Get(ctx, alice.ID, conad.ID); p.Name != "Conad" || p.TokenSealed != conad.TokenSealed {
		t.Errorf("after the update: %+v", p)
	}
	// A new address without a new token is refused: the old token would be sent there.
	call(a.handleStoresSave, http.MethodPost, "/settings/stores/x", form("Conad", "http://127.0.0.1:1", ""), id, alice)
	if p, _ := a.StorePlugins.Get(ctx, alice.ID, conad.ID); p.BaseURL != srv.URL {
		t.Errorf("the address changed without a token: %+v", p)
	}
	if w := call(a.handleStoresTest, http.MethodPost, "/settings/stores/x/test", nil, id, bob); w.Code != http.StatusNotFound {
		t.Errorf("Bob testing Alice's store: %d, want 404", w.Code)
	}
	call(a.handleStoresTest, http.MethodPost, "/settings/stores/x/test", nil, id, alice)

	// The search asks the store (OpenFoodFacts is off here) and offers its products,
	// with their price; a product without macros is left out.
	call(a.handleStoresEnabled, http.MethodPost, "/settings/stores/off/enabled", url.Values{"enabled": {"0"}}, map[string]string{"key": "off"}, alice)
	uctx := contextWithUser(ctx, alice)
	rows, warn, err := a.searchPicker(uctx, "avena", true)
	if err != nil || warn != "" || len(rows) != 1 {
		t.Fatalf("search = %v, %q, %v", rows, warn, err)
	}
	row := rows[0]
	if !row.IsStore() || !row.IsOFF() || row.Name() != "Fiocchi d'avena" || row.Price() != 189 || row.SourceLabel() != "Conad" || row.Unit() != "g" ||
		row.Barcode() != storeRef(conad.ID, "p1") || row.Kcal() != 372 || row.Quantity() != "500 g" {
		t.Errorf("row = %+v (barcode %q)", row.Store, row.Barcode())
	}
	// Another user's search never reaches Alice's store.
	if rows, _, _ := a.searchPicker(contextWithUser(ctx, bob), "avena", true); len(rows) != 0 {
		t.Errorf("Bob found %d products in Alice's store", len(rows))
	}
	// Without the external step (the local search that answers every keystroke) the store is not asked.
	if rows, _, _ := a.searchPicker(uctx, "avena", false); len(rows) != 0 {
		t.Errorf("the local search reached the store: %d", len(rows))
	}
	// A plugin that does not answer is a warning, not an error.
	call(a.handleStoresSave, http.MethodPost, "/settings/stores/x", form("Conad", "http://127.0.0.1:1", "secret"), id, alice)
	if rows, warn, err := a.searchPicker(uctx, "avena", true); err != nil || len(rows) != 0 || !strings.Contains(warn, "Conad") {
		t.Errorf("a dead store: %v, %q, %v", rows, warn, err)
	}
	call(a.handleStoresSave, http.MethodPost, "/settings/stores/x", form("Conad", srv.URL, "secret"), id, alice)

	// Importing makes an ingredient with the store's nutrition and links it; once only,
	// also when requests race.
	ids := make([]int64, 4)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], _ = a.resolveOFFBarcode(uctx, row.Barcode())
		}()
	}
	wg.Wait()
	ing := ids[0]
	for _, id := range ids {
		if id == 0 || id != ing {
			t.Fatalf("racing imports made different ingredients: %v", ids)
		}
	}
	var n int
	_ = a.Store.DB.QueryRow(`SELECT COUNT(*) FROM ingredients WHERE name = ?`, "Fiocchi d'avena").Scan(&n)
	if n != 1 {
		t.Errorf("%d ingredients made from one product", n)
	}
	got, _ := a.Ingredients.GetByID(ctx, ing)
	if got.Name != "Fiocchi d'avena" || got.KcalPer100 != 372 || got.Source != domain.IngredientSourceLocal || got.CreatedBy == nil || *got.CreatedBy != alice.ID {
		t.Errorf("ingredient = %+v", got)
	}
	if rows, _, _ := a.searchPicker(uctx, "avena", true); len(rows) != 1 || rows[0].IsOFF() || rows[0].LocalID() != ing {
		t.Errorf("an imported product must show as the ingredient, once: %+v", rows)
	}
	// Found under another name, it is added as the ingredient it is.
	if rows, _, _ := a.searchPicker(uctx, "fiocchi", true); len(rows) != 1 {
		t.Errorf("search by the ingredient's own name: %d rows", len(rows))
	}
	// A plugin that fails answers with a fixed reason, not its network error.
	call(a.handleStoresSave, http.MethodPost, "/settings/stores/x", form("Conad", "http://127.0.0.1:1", "secret"), id, alice)
	if _, err := a.resolveOFFBarcode(uctx, storeRef(conad.ID, "p9")); err != errStoreProduct {
		t.Errorf("a failing plugin = %v, want errStoreProduct", err)
	}
	call(a.handleStoresSave, http.MethodPost, "/settings/stores/x", form("Conad", srv.URL, "secret"), id, alice)
	for _, bad := range []struct {
		ref string
		who *domain.User
	}{{storeRef(conad.ID, "p1"), bob}, {storeRef(conad.ID, "p2"), alice}, {storeRef(999, "p1"), alice}, {"store:x:y", alice}, {"store:", alice}} {
		if _, err := a.resolveOFFBarcode(contextWithUser(ctx, bad.who), bad.ref); err == nil {
			t.Errorf("%q imported for %s", bad.ref, bad.who.Email)
		}
	}

	// The shopping list prices the ingredient: 100 g of a 500 g package is one package.
	monday, _ := time.Parse("2006-01-02", "2026-10-12")
	plan, _ := a.WeeklyPlans.GetOrCreateForWeek(ctx, alice.ID, monday)
	meal, _ := a.WeeklyPlans.AddMeal(ctx, plan.ID, alice.ID, 0, "Breakfast")
	if _, err := a.WeeklyPlans.AddItem(ctx, meal, alice.ID, nil, &ing, 100, "g"); err != nil {
		t.Fatal(err)
	}
	shop := func() string {
		return call(a.handleShoppingList, http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil, nil, alice).Body.String()
	}
	has := func(body, s string) bool {
		return strings.Contains(body, s) || strings.Contains(body, strings.Replace(s, ".", ",", 1))
	}
	body := shop()
	if !has(body, "1.89") || !strings.Contains(body, "Righe con prezzo: 1 su 1") {
		t.Errorf("the list is not priced:\n%s", body)
	}
	if body := call(a.handleShoppingList, http.MethodGet, "/shopping-list?week=2026-10-12&weeks=1", nil, nil, bob).Body.String(); has(body, "1.89") {
		t.Error("Bob's list shows Alice's price")
	}
	// More than the package holds: two packages.
	items, _ := a.ShoppingLists.LoadFull(ctx, mustList(t, a, alice, monday), alice.ID)
	if err := a.ShoppingLists.UpdateItemQuantity(ctx, items.Items[0].ID, alice.ID, 600, "g"); err != nil {
		t.Fatal(err)
	}
	if body := shop(); !has(body, "3.78") {
		t.Errorf("600 g are two packages of 500 g:\n%s", body)
	}

	// Refreshing asks the store again: a new price, and out of stock.
	listID := strconv.FormatInt(mustList(t, a, alice, monday), 10)
	if w := call(a.handleShoppingPrices, http.MethodPost, "/shopping-list/x/prices?week=2026-10-12&weeks=1", nil, map[string]string{"id": listID}, bob); w.Code != http.StatusNotFound {
		t.Errorf("Bob refreshing Alice's list: %d, want 404", w.Code)
	}
	if got, _ := a.StorePlugins.Links(ctx, alice.ID, []int64{ing}); got[ing].PriceCents != 189 {
		t.Fatalf("Bob's refresh changed the price: %+v", got)
	}
	price.Store(205)
	call(a.handleShoppingPrices, http.MethodPost, "/shopping-list/x/prices?week=2026-10-12&weeks=1", nil, map[string]string{"id": listID}, alice)
	if body := shop(); !has(body, "4.10") || !strings.Contains(body, "esaurito") {
		t.Errorf("after the refresh:\n%s", body)
	}

	// Disconnecting drops the link and the price; the ingredient stays.
	call(a.handleStoresDelete, http.MethodPost, "/settings/stores/x/delete", nil, id, bob)
	if len(plugins(alice)) != 1 {
		t.Fatal("Bob disconnected Alice's store")
	}
	call(a.handleStoresDelete, http.MethodPost, "/settings/stores/x/delete", nil, id, alice)
	if body := shop(); has(body, "4.10") || strings.Contains(body, "Stima ·") {
		t.Error("the price stays after the store is disconnected")
	}
	if _, err := a.Ingredients.GetByID(ctx, ing); err != nil {
		t.Errorf("the ingredient went with the store: %v", err)
	}
}

// The order of the sources is saved from a drag, and the answer is the list as saved.
func TestStoreSourcesOrder(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	one, _ := a.StorePlugins.Add(ctx, alice.ID, "One", "https://one.example", "s")
	two, _ := a.StorePlugins.Add(ctx, alice.ID, "Two", "https://two.example", "s")

	r := httptest.NewRequest(http.MethodPost, "/settings/stores/order", strings.NewReader(url.Values{"ids": {"off," + strconv.FormatInt(two, 10) + "," + strconv.FormatInt(one, 10)}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleStoresOrder)).ServeHTTP(w, requestWithUser(r, alice))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(body), "<ul") ||
		!(strings.Index(body, "Open Food Facts") < strings.Index(body, "Two") && strings.Index(body, "Two") < strings.Index(body, "One")) {
		t.Errorf("order answer: %d\n%s", w.Code, body)
	}
	if src, _ := a.StorePlugins.Sources(ctx, alice.ID); !src[0].OFF || src[1].Plugin.ID != two || src[2].Plugin.ID != one {
		t.Errorf("Sources = %+v", src)
	}
}

// mustList is the id of the user's shopping list for a week, made from the plan when it is new.
func mustList(t *testing.T, a *App, u *domain.User, monday time.Time) int64 {
	t.Helper()
	list, err := a.ShoppingLists.GetOrCreateList(context.Background(), u.ID, monday, 1)
	if err != nil {
		t.Fatal(err)
	}
	return list.ID
}
