package app

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// shoppingListData is the view model passed to shopping_list.html.
type shoppingListData struct {
	authPageData
	List      *domain.ShoppingListFull
	View      shoppingView
	WeekStart string // YYYY-MM-DD
	WeekCount int
	NextWeek  string
	PrevWeek  string
}

// shoppingUserItemView enriches ShoppingUserItem with the "duplicate" flag:
// true when the description matches (case-insensitive) the name of an auto
// ingredient in the list.
type shoppingUserItemView struct {
	Item      domain.ShoppingUserItem
	Duplicate bool
}

// parseShoppingWeek normalizes the `week` param (default = next Monday) and
// `weeks` (1..4, default 1).
func parseShoppingWeek(r *http.Request) (time.Time, int) {
	weekStart := nextMonday(time.Now())
	if v := strings.TrimSpace(r.URL.Query().Get("week")); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	if v := strings.TrimSpace(r.PostFormValue("week")); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	weeks := 1
	for _, v := range []string{r.URL.Query().Get("weeks"), r.PostFormValue("weeks")} {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 && n <= 4 {
			weeks = n
		}
	}
	return weekStart, weeks
}

// nextMonday returns the Monday of the current or next week.
// When today is Monday it stays today.
func nextMonday(t time.Time) time.Time {
	m := db.MondayOf(t)
	// When today is in the same week as the computed Monday, that is fine.
	return m
}

// handleShoppingList GET /shopping-list?week=YYYY-MM-DD&weeks=N
func (a *App) handleShoppingList(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart, weeks := parseShoppingWeek(r)
	list, err := a.ShoppingLists.GetOrCreateList(r.Context(), user.ID, weekStart, weeks)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// When the list has no auto items yet, generate it on demand on the first
	// load. Also when the user changes the weeks dropdown.
	full, err := a.ShoppingLists.LoadFull(r.Context(), list.ID, user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(full.Items) == 0 || full.WeekCount != weeks {
		if err := a.ShoppingLists.Generate(r.Context(), list.ID, user.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		full, err = a.ShoppingLists.LoadFull(r.Context(), list.ID, user.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	// Compute the duplicate flags: a user item whose description matches
	// (case-insensitive) the name of an auto item in the list.
	autoNames := make(map[string]bool, len(full.Items))
	for _, it := range full.Items {
		if it.Ingredient != nil {
			autoNames[strings.ToLower(strings.TrimSpace(it.Ingredient.Name))] = true
		}
	}
	userViews := make([]shoppingUserItemView, 0, len(full.UserItems))
	for _, ui := range full.UserItems {
		userViews = append(userViews, shoppingUserItemView{
			Item:      ui,
			Duplicate: autoNames[strings.ToLower(strings.TrimSpace(ui.Description))],
		})
	}

	ingIDs := make([]int64, 0, len(full.Items))
	for _, it := range full.Items {
		ingIDs = append(ingIDs, it.IngredientID)
	}
	cats, err := a.Categories.CategoriesForIngredients(r.Context(), user.ID, ingIDs)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	data := shoppingListData{
		authPageData: a.baseData(r),
		List:         full,
		View:         buildShoppingView(full.Items, userViews, cats),
		WeekStart:    weekStart.Format("2006-01-02"),
		WeekCount:    weeks,
		PrevWeek:     weekStart.AddDate(0, 0, -7).Format("2006-01-02"),
		NextWeek:     weekStart.AddDate(0, 0, 7).Format("2006-01-02"),
	}
	if err := a.Render.Render(w, r, http.StatusOK, "shopping_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleAddShoppingUserItem POST /shopping-list/{id}/user-items
// Body: description, quantity_text. Adds a manual item.
// HX-Request: the redirect to the list is followed by htmx, which swaps #shop-list;
// an error leaves a flash and reloads the page (shoppingFlashBack).
func (a *App) handleAddShoppingUserItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	listID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	desc := strings.TrimSpace(r.PostFormValue("description"))
	qty, ok := parseShoppingQuantity(r.PostFormValue("quantity"))
	unit := r.PostFormValue("unit")
	if unit != "pz" && unit != "ml" {
		unit = "g"
	}
	back := shoppingListURL(r)
	if !ok {
		a.shoppingFlashBack(w, r, back, "shopping.flash.quantity_invalid")
		return
	}
	if desc == "" {
		a.shoppingFlashBack(w, r, back, "shopping.flash.description_required")
		return
	}
	if !withinLen(desc, MaxNameLen) {
		a.shoppingFlashBack(w, r, back, "shopping.flash.description_too_long")
		return
	}
	if _, err := a.ShoppingLists.AddManualItem(r.Context(), listID, user.ID, desc, qty, unit); err != nil {
		if errors.Is(err, db.ErrShoppingListNotFound) {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// shoppingFlashBack leaves a flash message and sends the user back to the
// list. An htmx request gets HX-Redirect, so the whole page loads and shows
// the message; the swap that follows a success only takes #shop-list.
func (a *App) shoppingFlashBack(w http.ResponseWriter, r *http.Request, back, key string) {
	a.Sessions.Put(r.Context(), "flash", key)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", back)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// parseShoppingQuantity reads an optional positive number (comma or dot).
// Empty is valid and means 0 (no quantity).
func parseShoppingQuantity(raw string) (float64, bool) {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), ",", ".")
	if raw == "" {
		return 0, true
	}
	q, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(q) || math.IsInf(q, 0) || q <= 0 || q > 100000 {
		return 0, false
	}
	return q, true
}

// handleShoppingItemQuantity POST /shopping-list/items/{id}/quantity
// Body: quantity, qty_unit (g, ml or pz). Corrects the quantity computed from
// the meals; pieces are turned into the ingredient's unit.
func (a *App) handleShoppingItemQuantity(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	back := shoppingListURL(r)
	qty, ok := parseShoppingQuantity(r.PostFormValue("quantity"))
	if !ok || qty == 0 {
		a.shoppingFlashBack(w, r, back, "shopping.flash.quantity_invalid")
		return
	}
	if err := a.ShoppingLists.UpdateItemQuantity(r.Context(), itemID, user.ID, qty, shoppingUnit(r.PostFormValue("qty_unit"))); err != nil {
		switch {
		case errors.Is(err, db.ErrShoppingItemNotFound):
			http.Error(w, a.t(r, "shopping.item_not_found"), http.StatusNotFound)
		case errors.Is(err, db.ErrShoppingQuantityInvalid):
			a.shoppingFlashBack(w, r, back, "shopping.flash.quantity_invalid")
		default:
			a.serverError(w, r, err)
		}
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// shoppingUnit keeps a unit the list knows (g, ml or pz) and reads anything
// else as grams.
func shoppingUnit(u string) string {
	if u == "pz" || u == "ml" {
		return u
	}
	return "g"
}

// handleShoppingUserItemEdit POST /shopping-list/user-items/{id}/edit
// Body: description, quantity, qty_unit; or quantity_text for an item whose
// quantity is free text. Changes an item added by hand.
func (a *App) handleShoppingUserItemEdit(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	back := shoppingListURL(r)
	desc := strings.TrimSpace(r.PostFormValue("description"))
	if desc == "" {
		a.shoppingFlashBack(w, r, back, "shopping.flash.description_required")
		return
	}
	if !withinLen(desc, MaxNameLen) {
		a.shoppingFlashBack(w, r, back, "shopping.flash.description_too_long")
		return
	}
	var (
		qty      float64
		unit     string
		freeText string
	)
	if r.PostForm.Has("quantity_text") {
		freeText = strings.TrimSpace(r.PostFormValue("quantity_text"))
		if !withinLen(freeText, MaxNameLen) {
			a.shoppingFlashBack(w, r, back, "shopping.flash.description_too_long")
			return
		}
	} else {
		var ok bool
		if qty, ok = parseShoppingQuantity(r.PostFormValue("quantity")); !ok {
			a.shoppingFlashBack(w, r, back, "shopping.flash.quantity_invalid")
			return
		}
		unit = shoppingUnit(r.PostFormValue("qty_unit"))
	}
	if err := a.ShoppingLists.UpdateUserItem(r.Context(), itemID, user.ID, desc, qty, unit, freeText); err != nil {
		if errors.Is(err, db.ErrShoppingItemNotFound) {
			http.Error(w, a.t(r, "shopping.item_not_found"), http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// handleToggleShoppingUserItem POST /shopping-list/user-items/{id}/toggle
func (a *App) handleToggleShoppingUserItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.ShoppingLists.ToggleUserItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrShoppingItemNotFound) {
			http.Error(w, a.t(r, "shopping.item_not_found"), http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// handleRemoveShoppingUserItem POST /shopping-list/user-items/{id}/delete
func (a *App) handleRemoveShoppingUserItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.ShoppingLists.RemoveUserItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrShoppingItemNotFound) {
			http.Error(w, a.t(r, "shopping.item_not_found"), http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// handleShoppingGenerate POST /shopping-list/generate?week=YYYY-MM-DD&weeks=N
func (a *App) handleShoppingGenerate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart, weeks := parseShoppingWeek(r)
	list, err := a.ShoppingLists.GetOrCreateList(r.Context(), user.ID, weekStart, weeks)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.ShoppingLists.Generate(r.Context(), list.ID, user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "shopping.flash.regenerated")
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// handleShoppingToggle POST /shopping-list/items/{id}/toggle
// Always goes back to the list: the checked item changes group and the
// progress changes, so HTMX follows the redirect and takes its list (hx-select).
func (a *App) handleShoppingToggle(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.ShoppingLists.ToggleItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrShoppingItemNotFound) {
			http.Error(w, a.t(r, "shopping.item_not_found"), http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// shoppingListURL is the address of the list the request comes from,
// rebuilt from the validated week/weeks (never copied from the input).
func shoppingListURL(r *http.Request) string {
	weekStart, weeks := parseShoppingWeek(r)
	return "/shopping-list?week=" + weekStart.Format("2006-01-02") + "&weeks=" + strconv.Itoa(weeks)
}
