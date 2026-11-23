package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/stores"
)

// Store plugins (#239): the settings page where stores are connected and the search
// sources put in order, the search through them (stores_search.go) and the prices on
// the shopping list (shopping_handlers.go). The contract is in internal/stores.

// storeSourceRow is a search source on the settings page.
type storeSourceRow struct {
	Key     string
	Name    string // empty for OpenFoodFacts: the template translates it
	Host    string
	OFF     bool
	Enabled bool
	Color   int
}

// storesPageData is the view model of /settings/stores.
type storesPageData struct {
	authPageData
	Sources    []storeSourceRow
	Plugins    []db.StorePlugin
	CanConnect bool // NUTREEK_SECRET_KEY is set
	CanAdd     bool
}

func (a *App) storesData(r *http.Request) (storesPageData, error) {
	user := userFromContext(r.Context())
	sources, err := a.StorePlugins.Sources(r.Context(), user.ID)
	if err != nil {
		return storesPageData{}, err
	}
	plugins, err := a.StorePlugins.List(r.Context(), user.ID)
	if err != nil {
		return storesPageData{}, err
	}
	data := storesPageData{authPageData: a.baseData(r), Plugins: plugins, CanConnect: a.StoreBox != nil, CanAdd: len(plugins) < db.MaxStorePlugins}
	data.ActiveNav = "/settings"
	for i, s := range sources {
		row := storeSourceRow{Key: s.Key(), OFF: s.OFF, Enabled: s.Enabled}
		if !s.OFF {
			row.Name, row.Host = s.Plugin.Name, hostOf(s.Plugin.BaseURL)
			row.Color = 1 + i%5
		}
		data.Sources = append(data.Sources, row)
	}
	return data, nil
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return raw
}

func (a *App) handleStoresPage(w http.ResponseWriter, r *http.Request) {
	data, err := a.storesData(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.Render.Render(w, r, http.StatusOK, "stores", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleStoresSave POST /settings/stores (connect) and /settings/stores/{id} (change):
// fields name, url and token; on a change an empty token keeps the one stored.
func (a *App) handleStoresSave(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
		return
	}
	back := func(flash string) {
		a.Sessions.Put(r.Context(), "flash", flash)
		http.Redirect(w, r, "/settings/stores", http.StatusSeeOther)
	}
	if a.StoreBox == nil {
		back("stores.error.no_key")
		return
	}
	name := r.PostFormValue("name")
	base, err := stores.CleanURL(r.PostFormValue("url"), a.Cfg.StoresAllowPrivate)
	if err != nil {
		back("stores.error.url")
		return
	}
	token := strings.TrimSpace(r.PostFormValue("token"))
	sealed := ""
	if token != "" {
		if len(token) > 500 {
			back("stores.error.invalid")
			return
		}
		if sealed, err = a.StoreBox.Seal(token); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if chi.URLParam(r, "id") == "" {
		_, err = a.StorePlugins.Add(r.Context(), user.ID, name, base, sealed)
	} else {
		var id int64
		if id, err = parseID(r, "id"); err != nil {
			a.notFound(w, r)
			return
		}
		// The token never leaves for an address the user did not give it with: a new
		// address needs a new token.
		if cur, gerr := a.StorePlugins.Get(r.Context(), user.ID, id); gerr == nil && cur.BaseURL != base && sealed == "" {
			back("stores.error.token_needed")
			return
		}
		err = a.StorePlugins.Update(r.Context(), user.ID, id, name, base, sealed)
	}
	switch {
	case errors.Is(err, db.ErrInvalidStore):
		back("stores.error.invalid")
	case errors.Is(err, db.ErrStoreNotFound):
		a.notFound(w, r)
	case err != nil:
		a.serverError(w, r, err)
	default:
		back("stores.flash.saved")
	}
}

// handleStoresDelete POST /settings/stores/{id}/delete.
func (a *App) handleStoresDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.StorePlugins.Delete(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, db.ErrStoreNotFound) {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "stores.flash.deleted")
	http.Redirect(w, r, "/settings/stores", http.StatusSeeOther)
}

// handleStoresEnabled POST /settings/stores/{key}/enabled: turns a source on or off;
// the key is a plugin's id or "off".
func (a *App) handleStoresEnabled(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	on := r.PostFormValue("enabled") == "1"
	var err error
	if key := chi.URLParam(r, "key"); key == db.OFFKey {
		err = a.StorePlugins.SetOFFEnabled(r.Context(), user.ID, on)
	} else {
		var id int64
		if id, err = parseID(r, "key"); err != nil {
			a.notFound(w, r)
			return
		}
		err = a.StorePlugins.SetEnabled(r.Context(), user.ID, id, on)
	}
	switch {
	case errors.Is(err, db.ErrStoreNotFound):
		a.notFound(w, r)
	case err != nil:
		a.serverError(w, r, err)
	default:
		http.Redirect(w, r, "/settings/stores", http.StatusSeeOther)
	}
}

// handleStoresOrder POST /settings/stores/order: the sources in a new order (ids=off,3,2,
// from the drag and the arrows); the answer is the list as saved.
func (a *App) handleStoresOrder(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
		return
	}
	if err := a.StorePlugins.Reorder(r.Context(), user.ID, strings.Split(r.PostFormValue("ids"), ",")); err != nil {
		a.serverError(w, r, err)
		return
	}
	data, err := a.storesData(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data.CSRFToken = nosurf.Token(r)
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "stores", "store_sources", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleStoresTest POST /settings/stores/{id}/test: searches the plugin for a common
// word and says what came back, so a wrong address or token shows at once.
func (a *App) handleStoresTest(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	p, err := a.StorePlugins.Get(r.Context(), user.ID, id)
	if err != nil {
		a.notFound(w, r)
		return
	}
	var flash string
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	found, err := a.searchStore(ctx, p, "pasta")
	switch {
	case err != nil:
		a.Logger.Warn("store plugin test failed", "plugin", p.ID, "err", err)
		flash = a.t(r, "stores.test.failed", "Name", p.Name)
	default:
		flash = a.t(r, "stores.test.ok", "Name", p.Name, "Count", len(found))
	}
	a.Sessions.Put(r.Context(), "flash", flash)
	http.Redirect(w, r, "/settings/stores", http.StatusSeeOther)
}

// handleShoppingPrices POST /shopping-list/{id}/prices: asks the stores the current price
// and availability of the products behind the list's ingredients, and goes back to it.
func (a *App) handleShoppingPrices(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	listID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	full, err := a.ShoppingLists.LoadFull(r.Context(), listID, user.ID)
	if err != nil {
		a.notFound(w, r)
		return
	}
	ids := make([]int64, 0, len(full.Items))
	for _, it := range full.Items {
		ids = append(ids, it.IngredientID)
	}
	links, err := a.StorePlugins.Links(r.Context(), user.ID, ids)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	byPlugin := map[int64][]db.StoreLink{}
	for _, l := range links {
		byPlugin[l.PluginID] = append(byPlugin[l.PluginID], l)
	}
	var updated int
	var failed []string
	for pluginID, ls := range byPlugin {
		p, err := a.StorePlugins.Get(r.Context(), user.ID, pluginID)
		if err != nil {
			continue
		}
		products := make([]string, 0, len(ls))
		for _, l := range ls {
			products = append(products, l.ProductID)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		token, err := a.pluginToken(p)
		var status map[string]stores.Status
		if err == nil {
			status, err = a.StoreClient.Availability(ctx, p.BaseURL, token, products)
		}
		cancel()
		if err != nil {
			a.Logger.Warn("store price refresh failed", "plugin", p.ID, "err", err)
			failed = append(failed, p.Name)
			continue
		}
		for _, l := range ls {
			if s, ok := status[l.ProductID]; ok {
				if err := a.StorePlugins.SetStatus(r.Context(), user.ID, pluginID, l.ProductID, s.PriceCents, s.HasPrice, s.Available); err != nil {
					a.serverError(w, r, err)
					return
				}
				updated++
			}
		}
	}
	flash := a.t(r, "stores.prices.updated", "Count", updated)
	if len(failed) > 0 {
		flash = a.t(r, "stores.prices.failed", "Names", strings.Join(failed, ", "))
	}
	a.Sessions.Put(r.Context(), "flash", flash)
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}
