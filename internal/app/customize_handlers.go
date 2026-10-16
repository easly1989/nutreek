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

// customizeFormData is the view model of the dialog that customizes a recipe
// item in a meal.
type customizeFormData struct {
	authPageData
	MealItemID int64
	RecipeName string
	WeekStart  string
	Rows       []customizeRow
	Error      string
}

// customizeRow is one row of the form: ingredient + quantity.
type customizeRow struct {
	IngredientID int64
	Name         string
	Unit         string
	Quantity     float64
}

// handleMealItemCustomizeForm GET /meal-items/{itemID}/customize?start=YYYY-MM-DD
// Renders the customization dialog for a recipe item.
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
// Saves the item's custom variations. Answers with HX-Redirect to the
// current week.
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

	// Read every qty_{ingredientID} field of the form.
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
			continue // skip ingredients with a 0 or invalid quantity
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
// Removes every customization, restoring the base recipe.
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

// loadMealItemFull loads a single meal_item with all its data resolved
// (recipe + recipe_items + customizations). It returns nil when not found or
// not the user's.
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

	// GetWithItems works for the user's own recipes and for public ones (viewerID check).
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

// buildCustomizeForm builds the view model of the customization dialog.
// When the item already has customizations it preloads them; otherwise it
// prefills the scaled quantities of the base recipe.
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
		// Prefill from the existing customizations.
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
		// Prefill from the scaled quantities of the base recipe.
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
