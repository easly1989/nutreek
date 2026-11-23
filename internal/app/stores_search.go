package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/stores"
)

// A store product is offered in the pickers like an OpenFoodFacts one and imported on
// first use. The form carries its reference, "store:<plugin id>:<product id>", in the
// field the barcode has; resolveOFFBarcode hands it to resolveStoreProduct, which asks
// the plugin for the product again: a form is never trusted for what an import writes.

const storeRefPrefix = "store:"

func storeRef(pluginID int64, productID string) string {
	return storeRefPrefix + strconv.FormatInt(pluginID, 10) + ":" + productID
}

func parseStoreRef(ref string) (pluginID int64, productID string, ok bool) {
	rest, found := strings.CutPrefix(ref, storeRefPrefix)
	if !found {
		return 0, "", false
	}
	id, product, found := strings.Cut(rest, ":")
	pluginID, err := strconv.ParseInt(id, 10, 64)
	return pluginID, product, found && err == nil && pluginID > 0 && product != ""
}

// errStoreProduct is shown to the user through errText (reasons).
var errStoreProduct = simpleErr("il prodotto del negozio non è disponibile")

// pluginToken opens the token of a plugin.
func (a *App) pluginToken(p db.StorePlugin) (string, error) {
	if a.StoreBox == nil {
		return "", errors.New("NUTREEK_SECRET_KEY is not set")
	}
	return a.StoreBox.Open(p.TokenSealed)
}

// searchStore asks a plugin for the products matching q.
func (a *App) searchStore(ctx context.Context, p db.StorePlugin, q string) ([]stores.Product, error) {
	token, err := a.pluginToken(p)
	if err != nil {
		return nil, err
	}
	return a.StoreClient.Search(ctx, p.BaseURL, token, q)
}

// searchSources asks every enabled source of a search, in parallel, and returns the
// answers in the order of the sources.
func (a *App) searchSources(ctx context.Context, q string, viewer int64, sources []db.SearchSource, known map[string]bool, knownIDs map[int64]bool) []sourceResult {
	out := make([]sourceResult, len(sources))
	var wg sync.WaitGroup
	for i, s := range sources {
		if !s.Enabled {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.OFF {
				out[i] = a.searchOFFRows(ctx, q, viewer, known)
				return
			}
			out[i] = a.searchPluginRows(ctx, q, viewer, s.Plugin, knownIDs)
		}()
	}
	wg.Wait()
	return out
}

// searchPluginRows is the store part of a search. A product the viewer already made an
// ingredient of is shown as that ingredient, unless the search found it already.
func (a *App) searchPluginRows(ctx context.Context, q string, viewer int64, p db.StorePlugin, knownIDs map[int64]bool) sourceResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	products, err := a.searchStore(ctx, p, q)
	if err != nil {
		a.Logger.Warn("store search failed", "plugin", p.ID, "err", err)
		return sourceResult{warn: i18n.T(i18n.FromContext(ctx), "stores.search_failed", "Name", p.Name)}
	}
	var res sourceResult
	for i := range products {
		prod := products[i]
		if id, err := a.StorePlugins.LinkedIngredient(ctx, viewer, p.ID, prod.ID); err == nil && id > 0 {
			if knownIDs[id] {
				continue
			}
			if ing, err := a.Ingredients.GetVisible(ctx, id, viewer); err == nil {
				res.extra = append(res.extra, ingredientRow{Local: ing})
				continue
			}
		}
		res.rows = append(res.rows, ingredientRow{Store: &prod, StoreName: p.Name, StoreID: p.ID})
	}
	return res
}

// storeImportMu serializes the creation of an ingredient from a store product, so a
// double click or two tabs make one ingredient, not two (the link is unique).
var storeImportMu sync.Mutex

// resolveStoreProduct makes an ingredient of a store product (idempotent) and links it
// to the product, so the shopping list can price it. It returns the ingredient's id.
// Whatever goes wrong with the plugin is logged and answered with errStoreProduct: the
// text of a network error would tell a user what their server can reach.
func (a *App) resolveStoreProduct(ctx context.Context, ref string) (int64, error) {
	pluginID, productID, ok := parseStoreRef(ref)
	user := userFromContext(ctx)
	if !ok || user == nil {
		return 0, errStoreProduct
	}
	fail := func(err error) (int64, error) {
		a.Logger.Warn("store product import failed", "plugin", pluginID, "err", err)
		return 0, errStoreProduct
	}
	if id, err := a.StorePlugins.LinkedIngredient(ctx, user.ID, pluginID, productID); err != nil {
		return fail(err)
	} else if id > 0 {
		return id, nil
	}
	p, err := a.StorePlugins.Get(ctx, user.ID, pluginID)
	if err != nil {
		return fail(err)
	}
	token, err := a.pluginToken(p)
	if err != nil {
		return fail(err)
	}
	fetch, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	prod, err := a.StoreClient.Product(fetch, p.BaseURL, token, productID)
	if err != nil || !prod.HasNutrition {
		return fail(err)
	}
	unit := domain.UnitGram
	if prod.SizeUnit == stores.UnitMl {
		unit = domain.UnitMilliliter
	}
	var brand *string
	if b := strings.TrimSpace(prod.Brand); b != "" {
		brand = &b
	}

	storeImportMu.Lock()
	defer storeImportMu.Unlock()
	if id, err := a.StorePlugins.LinkedIngredient(ctx, user.ID, pluginID, productID); err != nil {
		return fail(err)
	} else if id > 0 { // the other click got here first
		return id, nil
	}
	id, err := a.Ingredients.CreateLocal(ctx, prod.Name, brand, unit, prod.Kcal, prod.Protein, prod.Carbs, prod.Fat,
		&user.ID, prod.UnitWeightG, nil, prod.Fiber, prod.Sugars, prod.SaturatedFat, prod.Sodium)
	if err != nil {
		return fail(err)
	}
	err = a.StorePlugins.SaveLink(ctx, db.StoreLink{
		UserID: user.ID, IngredientID: id, PluginID: p.ID, ProductID: prod.ID, ProductName: prod.Name,
		SizeQty: prod.SizeQty, SizeUnit: prod.SizeUnit, PriceCents: prod.PriceCents, HasPrice: prod.HasPrice,
		Currency: prod.Currency, Available: prod.Available,
	})
	if err != nil {
		return fail(err)
	}
	return id, nil
}
