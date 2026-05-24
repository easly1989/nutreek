package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// customizeFormData e' il view-model del modal di personalizzazione di un
// item ricetta in un pasto.
type customizeFormData struct {
	authPageData
	MealItemID int64
	RecipeName string
	WeekStart  string
	Rows       []customizeRow
	Error      string
}

// customizeRow e' una riga del form: ingrediente + quantita'.
type customizeRow struct {
	IngredientID int64
	Name         string
	Unit         string
	Quantity     float64
}

// handleMealItemCustomizeForm GET /meal-items/{itemID}/customize?start=YYYY-MM-DD
// Renderizza il modal di personalizzazione per un item ricetta.
func (a *App) handleMealItemCustomizeForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	item, err := a.loadMealItemFull(r, user.ID, itemID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if item == nil || item.Recipe == nil {
		a.notFound(w, r)
		return
	}
	data := a.buildCustomizeForm(r, item, formatDate(weekStart), "")
	a.renderCustomizeModal(w, r, data)
}

// handleMealItemCustomizeSave POST /meal-items/{itemID}/customize?start=YYYY-MM-DD
// Salva le variazioni personalizzate per l'item. Risponde con HX-Redirect alla
// settimana corrente.
func (a *App) handleMealItemCustomizeSave(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form invalido", http.StatusBadRequest)
		return
	}

	// Legge tutti i campi qty_{ingredientID} dal form.
	var rows []db.CustomizationInput
	for key, vals := range r.PostForm {
		if !strings.HasPrefix(key, "qty_") {
			continue
		}
		ingIDStr := strings.TrimPrefix(key, "qty_")
		ingID, err := strconv.ParseInt(ingIDStr, 10, 64)
		if err != nil || ingID <= 0 {
			continue
		}
		if len(vals) == 0 {
			continue
		}
		qty, err := parseNonNegFloat(strings.TrimSpace(vals[0]))
		if err != nil || qty <= 0 {
			continue // salta ingredienti con quantità 0 o invalida
		}
		rows = append(rows, db.CustomizationInput{IngredientID: ingID, Quantity: qty})
	}

	if err := a.MealItemCustomizations.Replace(r.Context(), itemID, user.ID, rows); err != nil {
		if err == db.ErrMealItemNotFound {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}

	w.Header().Set("HX-Redirect", "/?start="+formatDate(weekStart))
	w.WriteHeader(http.StatusNoContent)
}

// handleMealItemCustomizeReset POST /meal-items/{itemID}/customize/reset?start=
// Rimuove tutte le personalizzazioni, ripristinando la ricetta base.
func (a *App) handleMealItemCustomizeReset(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	if err := a.MealItemCustomizations.Replace(r.Context(), itemID, user.ID, nil); err != nil {
		if err == db.ErrMealItemNotFound {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("HX-Redirect", "/?start="+formatDate(weekStart))
	w.WriteHeader(http.StatusNoContent)
}

// loadMealItemFull carica un singolo meal_item con tutti i dati risolti
// (recipe + recipe_items + customizations). Ritorna nil se non trovato o
// non appartenente all'utente.
func (a *App) loadMealItemFull(r *http.Request, userID, itemID int64) (*domain.MealItemFull, error) {
	row := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT mi.id, mi.meal_id, mi.recipe_id, mi.ingredient_id, mi.quantity, mi.sort_order
		FROM meal_items mi
		JOIN meals m ON m.id = mi.meal_id
		JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE mi.id = ? AND wp.user_id = ?
	`, itemID, userID)
	var it domain.MealItem
	if err := row.Scan(&it.ID, &it.MealID, &it.RecipeID, &it.IngredientID, &it.Quantity, &it.SortOrder); err != nil {
		return nil, nil
	}
	full := &domain.MealItemFull{MealItem: it}
	if it.RecipeID == nil {
		return full, nil
	}

	// GetWithItems funziona sia per ricette proprie sia per pubbliche (viewerID check).
	rec, err := a.Recipes.GetWithItems(r.Context(), *it.RecipeID, userID)
	if err == nil {
		full.Recipe = &rec.Recipe
		full.RecipeItems = rec.Items
	}

	custMap, err := a.MealItemCustomizations.ListForItems(r.Context(), []int64{it.ID})
	if err != nil {
		return nil, fmt.Errorf("load customizations: %w", err)
	}
	full.Customizations = custMap[it.ID]
	return full, nil
}

// buildCustomizeForm costruisce il view-model per il modal di personalizzazione.
// Se l'item ha già personalizzazioni, le pre-carica; altrimenti pre-popola con
// le quantità scalate della ricetta base.
func (a *App) buildCustomizeForm(r *http.Request, item *domain.MealItemFull, weekStart, errMsg string) customizeFormData {
	data := customizeFormData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		MealItemID:   item.ID,
		WeekStart:    weekStart,
		Error:        errMsg,
	}
	if item.Recipe != nil {
		data.RecipeName = item.Recipe.Name
	}

	if item.IsCustomized() {
		// Pre-popola dalle personalizzazioni esistenti.
		for _, c := range item.Customizations {
			if c.Ingredient == nil {
				continue
			}
			data.Rows = append(data.Rows, customizeRow{
				IngredientID: c.IngredientID,
				Name:         c.Ingredient.Name,
				Unit:         string(c.Ingredient.Unit),
				Quantity:     c.Quantity,
			})
		}
	} else if item.Recipe != nil && len(item.RecipeItems) > 0 {
		// Pre-popola dalle quantità scalate della ricetta base.
		scale := 1.0
		if item.Recipe.Servings > 0 {
			scale = item.Quantity / float64(item.Recipe.Servings)
		}
		for _, ri := range item.RecipeItems {
			data.Rows = append(data.Rows, customizeRow{
				IngredientID: ri.IngredientID,
				Name:         ri.Ingredient.Name,
				Unit:         string(ri.Ingredient.Unit),
				Quantity:     ri.Quantity * scale,
			})
		}
	}
	return data
}

func (a *App) renderCustomizeModal(w http.ResponseWriter, r *http.Request, data customizeFormData) {
	mdata := modalDialogData{
		Title:     "Personalizza ingredienti",
		Inner:     "meal_item_customize_form",
		InnerData: data,
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "dashboard", "modal_dialog", mdata); err != nil {
		a.serverError(w, r, err)
	}
}

