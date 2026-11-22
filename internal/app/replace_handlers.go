package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// Replace the ingredient of a row (#227): from a meal's row in the week, or from
// a row of a recipe. The dialog shows the current row, a search and the
// suggestions; each answer carries the difference in kcal and macros at the
// row's weight, so the user sees what changes before confirming. The row keeps
// its place and its weight (macros.ReplacementQuantity).

const replaceSuggestions = 8

// replaceTarget is the row being replaced.
type replaceTarget struct {
	ing     *domain.Ingredient // the current ingredient
	qty     float64            // the weight the row keeps, in unit
	unit    string             // "g" or "ml"
	context string             // "in Breakfast" / the recipe's name
	cur     macros.Macros      // the row's macros now
}

func newReplaceTarget(ing *domain.Ingredient, quantity float64, unit, context string) *replaceTarget {
	qty, u := macros.ReplacementQuantity(quantity, unit, ing)
	return &replaceTarget{ing: ing, qty: qty, unit: u, context: context, cur: macros.FromIngredient(*ing, qty)}
}

// replaceDialogData is the view model of the dialog.
type replaceDialogData struct {
	authPageData
	Name, Brand string
	Context     string
	Qty         float64
	Unit        string
	Cur         macros.Macros
	ResultsURL  string // the list, without the query
	Extra       string // fixed parameters of the list, as a query string
	Action      string // where the form posts
	HTMX        bool   // the form updates the day of the week (a meal row) instead of leaving the page
}

// replaceRow is a suggestion or a result.
type replaceRow struct {
	Choice  string // "ing:<id>" or "off:<barcode>"
	Name    string
	Sub     string // brand, and «same category» when it is
	Macros  macros.Macros
	Diff    macros.Macros
	Confirm string // the text of the button once the row is chosen
}

// replaceResultsData is the view model of the list.
type replaceResultsData struct {
	Rows       []replaceRow
	Query      string
	Suggested  bool
	OFFWarning string
}

func (a *App) renderReplaceDialog(w http.ResponseWriter, r *http.Request, t *replaceTarget, resultsURL, extra, action string, htmx bool) {
	data := replaceDialogData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Name:         t.ing.Name,
		Context:      t.context,
		Qty:          t.qty,
		Unit:         t.unit,
		Cur:          t.cur,
		ResultsURL:   resultsURL,
		Extra:        extra,
		Action:       action,
		HTMX:         htmx,
	}
	if t.ing.Brand != nil {
		data.Brand = *t.ing.Brand
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "replace_dialog", data); err != nil {
		a.serverError(w, r, err)
	}
}

// renderReplaceResults answers the list: the suggestions when q is empty, else
// the search (the catalog, and OpenFoodFacts when asked) with the same
// differences.
func (a *App) renderReplaceResults(w http.ResponseWriter, r *http.Request, user *domain.User, t *replaceTarget) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	data := replaceResultsData{Query: q, Suggested: q == ""}
	if q == "" {
		rows, err := a.replaceSuggestions(r.Context(), user.ID, r, t)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Rows = rows
	} else {
		found, warn, err := a.searchPicker(r.Context(), q, includeOFFParam(r))
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.OFFWarning = warn
		for _, fr := range found {
			if fr.Local != nil && fr.Local.ID == t.ing.ID {
				continue
			}
			ing, choice := rowIngredient(fr)
			if ing == nil {
				continue
			}
			data.Rows = append(data.Rows, a.replaceRowOf(r, t, ing, choice, false))
		}
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "replace_results", data); err != nil {
		a.serverError(w, r, err)
	}
}

// replaceSuggestions are the ingredients nearest to the row (see
// macros.RankReplacements), the ones the user put in the same category first.
func (a *App) replaceSuggestions(ctx context.Context, userID int64, r *http.Request, t *replaceTarget) ([]replaceRow, error) {
	cands, err := a.Ingredients.ListAllActive(ctx, userID)
	if err != nil {
		return nil, err
	}
	same, err := a.Categories.SameCategoryIDs(ctx, userID, t.ing.ID)
	if err != nil {
		return nil, err
	}
	ranked := macros.RankReplacements(t.cur, t.qty, t.ing.ID, cands, same, replaceSuggestions)
	rows := make([]replaceRow, 0, len(ranked))
	for _, rp := range ranked {
		rows = append(rows, a.replaceRowOf(r, t, rp.Ingredient, "ing:"+strconv.FormatInt(rp.Ingredient.ID, 10), rp.SameCategory))
	}
	return rows, nil
}

// rowIngredient turns a search row into the ingredient to compare and the value
// of the choice: «ing:<id>» for one in the catalog, «off:<barcode>» for an
// OpenFoodFacts product not imported yet (it is imported when chosen).
func rowIngredient(fr ingredientRow) (*domain.Ingredient, string) {
	if fr.Local != nil {
		return fr.Local, "ing:" + strconv.FormatInt(fr.Local.ID, 10)
	}
	if fr.OFF == nil || !fr.Importable() {
		return nil, ""
	}
	brand := fr.OFF.Brand
	return &domain.Ingredient{
		Name: fr.OFF.Name, Brand: &brand, KcalPer100: fr.OFF.Kcal,
		ProteinPer100: fr.OFF.Protein, CarbsPer100: fr.OFF.Carbs, FatPer100: fr.OFF.Fat,
	}, "off:" + fr.OFF.Code
}

func (a *App) replaceRowOf(r *http.Request, t *replaceTarget, ing *domain.Ingredient, choice string, same bool) replaceRow {
	m, d := macros.DiffOf(t.cur, *ing, t.qty)
	var parts []string
	if ing.Brand != nil && *ing.Brand != "" {
		parts = append(parts, *ing.Brand)
	}
	if same {
		parts = append(parts, a.t(r, "replace.same_category"))
	}
	return replaceRow{
		Choice:  choice,
		Name:    ing.Name,
		Sub:     strings.Join(parts, " · "),
		Macros:  m,
		Diff:    d,
		Confirm: a.t(r, "replace.confirm_with", "Name", ing.Name),
	}
}

// resolveReplaceChoice turns the form's «ing:<id>» or «off:<barcode>» into an
// ingredient id, importing the OpenFoodFacts product when it is one.
func (a *App) resolveReplaceChoice(ctx context.Context, choice string) (int64, error) {
	kind, value, _ := strings.Cut(choice, ":")
	switch kind {
	case "ing":
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return 0, errBadChoice
		}
		return id, nil
	case "off":
		if value == "" {
			return 0, errBadChoice
		}
		return a.resolveOFFBarcode(ctx, value)
	}
	return 0, errBadChoice
}

var errBadChoice = errors.New("no ingredient chosen")

// ---- a meal's row -----------------------------------------------------------

// mealReplaceTarget loads the row and its ingredient; db.ErrMealItemNotFound when
// the row is not an ingredient row of the user's.
func (a *App) mealReplaceTarget(r *http.Request, userID, itemID int64) (*replaceTarget, *db.MealIngredientItem, error) {
	item, err := a.WeeklyPlans.IngredientItem(r.Context(), itemID, userID)
	if err != nil {
		return nil, nil, err
	}
	ing, err := a.Ingredients.GetByID(r.Context(), *item.IngredientID)
	if err != nil {
		return nil, nil, err
	}
	return newReplaceTarget(ing, item.Quantity, item.QuantityUnit, a.t(r, "replace.in_meal", "Meal", item.MealName)), item, nil
}

func (a *App) replaceMealError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, db.ErrMealItemNotFound) || errors.Is(err, db.ErrIngredientNotFound) {
		http.Error(w, a.t(r, "week.error.item_not_found"), http.StatusNotFound)
		return
	}
	a.serverError(w, r, err)
}

// handleMealItemReplaceForm GET /meal-items/{itemID}/replace?start=YYYY-MM-DD:
// the dialog (HTMX into #modal-host).
func (a *App) handleMealItemReplaceForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	t, _, err := a.mealReplaceTarget(r, user.ID, itemID)
	if err != nil {
		a.replaceMealError(w, r, err)
		return
	}
	id := strconv.FormatInt(itemID, 10)
	extra := "start=" + url.QueryEscape(formatDate(weekStart))
	a.renderReplaceDialog(w, r, t, "/meal-items/"+id+"/replace/results", extra, "/meal-items/"+id+"/replace?"+extra, true)
}

// handleMealItemReplaceResults GET /meal-items/{itemID}/replace/results?q=&off=
func (a *App) handleMealItemReplaceResults(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	t, _, err := a.mealReplaceTarget(r, user.ID, itemID)
	if err != nil {
		a.replaceMealError(w, r, err)
		return
	}
	a.renderReplaceResults(w, r, user, t)
}

// handleMealItemReplace POST /meal-items/{itemID}/replace?start=YYYY-MM-DD,
// field choice. It answers like the other item actions (the day card).
func (a *App) handleMealItemReplace(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	t, item, err := a.mealReplaceTarget(r, user.ID, itemID)
	if err != nil {
		a.replaceMealError(w, r, err)
		return
	}
	newID, err := a.resolveReplaceChoice(r.Context(), r.PostFormValue("choice"))
	if errors.Is(err, errBadChoice) {
		a.flashAndBack(w, r, weekStart, "week.flash.select_ingredient")
		return
	}
	if err != nil {
		a.flashAndBack(w, r, weekStart, a.t(r, "week.flash.off_import_failed", "Error", a.errText(r, err)))
		return
	}
	if newID != t.ing.ID {
		if err := a.WeeklyPlans.ReplaceItemIngredient(r.Context(), itemID, user.ID, newID, t.qty, t.unit); err != nil {
			if errors.Is(err, db.ErrIngredientNotFound) {
				a.flashAndBack(w, r, weekStart, "week.flash.select_ingredient")
				return
			}
			a.replaceMealError(w, r, err)
			return
		}
	}
	a.respondMutationOrRedirect(w, r, weekStart, item.DayOfWeek)
}

// ---- a recipe's row ---------------------------------------------------------

// recipeReplaceTarget loads the recipe and the row. The recipe must be the
// user's: another author's is a 403, a row that is not in it a 404.
func (a *App) recipeReplaceTarget(w http.ResponseWriter, r *http.Request, user *domain.User) (*replaceTarget, *domain.RecipeWithItems, *domain.RecipeItemFull, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return nil, nil, nil, false
	}
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return nil, nil, nil, false
	}
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return nil, nil, nil, false
	}
	if rec.UserID != user.ID {
		http.Error(w, a.t(r, "recipes.error.not_author"), http.StatusForbidden)
		return nil, nil, nil, false
	}
	for i := range rec.Items {
		if rec.Items[i].ID == itemID {
			it := &rec.Items[i]
			return newReplaceTarget(&it.Ingredient, it.Quantity, it.QuantityUnit, rec.Name), rec, it, true
		}
	}
	http.Error(w, a.t(r, "recipes.error.item_not_found"), http.StatusNotFound)
	return nil, nil, nil, false
}

// handleRecipeReplaceForm GET /recipes/{id}/items/{itemID}/replace: the dialog.
func (a *App) handleRecipeReplaceForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	t, rec, item, ok := a.recipeReplaceTarget(w, r, user)
	if !ok {
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/recipes/"+strconv.FormatInt(rec.ID, 10), http.StatusSeeOther)
		return
	}
	base := "/recipes/" + strconv.FormatInt(rec.ID, 10) + "/items/" + strconv.FormatInt(item.ID, 10) + "/replace"
	a.renderReplaceDialog(w, r, t, base+"/results", "", base, false)
}

// handleRecipeReplaceResults GET /recipes/{id}/items/{itemID}/replace/results
func (a *App) handleRecipeReplaceResults(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	t, _, _, ok := a.recipeReplaceTarget(w, r, user)
	if !ok {
		return
	}
	a.renderReplaceResults(w, r, user, t)
}

// handleRecipeReplaceItem POST /recipes/{id}/items/{itemID}/replace, field
// choice. Back to the recipe with the row's quantity focused; the same checks as
// the other edits of a recipe's rows (a recipe cannot end up with the content of
// another, a draft stops being one).
func (a *App) handleRecipeReplaceItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	t, rec, item, ok := a.recipeReplaceTarget(w, r, user)
	if !ok {
		return
	}
	saved := "/recipes/" + strconv.FormatInt(rec.ID, 10) + "?saved=" + strconv.FormatInt(item.ID, 10)
	newID, err := a.resolveReplaceChoice(r.Context(), r.PostFormValue("choice"))
	if errors.Is(err, errBadChoice) {
		http.Redirect(w, r, saved, http.StatusSeeOther)
		return
	}
	if err != nil {
		a.reRenderEdit(w, r, rec.ID, "", a.t(r, "week.flash.off_import_failed", "Error", a.errText(r, err)))
		return
	}
	if newID == t.ing.ID {
		http.Redirect(w, r, saved, http.StatusSeeOther)
		return
	}
	unit, err := a.Recipes.StoredUnit(r.Context(), t.unit, newID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	simulated := make([]domain.RecipeItem, 0, len(rec.Items))
	for _, it := range rec.Items {
		if it.ID == item.ID {
			it.IngredientID, it.Quantity, it.QuantityUnit = newID, t.qty, unit
		}
		simulated = append(simulated, it.RecipeItem)
	}
	if other, err := a.Recipes.IdenticalContentRecipe(r.Context(), user.ID, simulated, rec.ID); err != nil {
		a.serverError(w, r, err)
		return
	} else if other != "" {
		a.reRenderEdit(w, r, rec.ID, "", a.t(r, "replace.identical", "Other", other))
		return
	}
	if err := a.Recipes.ReplaceItem(r.Context(), rec.ID, item.ID, user.ID, newID, t.qty, t.unit); err != nil {
		switch {
		case errors.Is(err, db.ErrRecipeItemNotFound):
			http.Error(w, a.t(r, "recipes.error.item_not_found"), http.StatusNotFound)
		case errors.Is(err, db.ErrIngredientNotFound):
			http.Redirect(w, r, saved, http.StatusSeeOther)
		default:
			a.serverError(w, r, err)
		}
		return
	}
	if rec.IsDraft {
		if err := a.Recipes.PromoteDraft(r.Context(), rec.ID, user.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, saved, http.StatusSeeOther)
}
