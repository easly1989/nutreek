package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/recipeimport"
)

// ---- view models -----------------------------------------------------------

type recipesListData struct {
	authPageData
	Rows    []recipeListRow // rows of the current filter, already filtered by Query
	Filter  string          // mine | others | used | public
	Query   string
	Filters []recipeFilter
	MineN   int // totals for the subtitle
	OthersN int
}

// recipeFilter is a filter chip of the recipe list with its count.
type recipeFilter struct {
	Key, Label string
	Count      int
	On         bool
}

// recipeListRow is a row of the list: the recipe, the macros per serving and,
// for the user's own, the count of uses in the weekly plans (it shows an
// informed warning in the delete confirmation).
type recipeListRow struct {
	Recipe     *domain.Recipe
	Mine       bool
	Usages     int
	PerServing macros.Macros
}

// recipeFilters lists the filters of the recipe list, in chip order.
var recipeFilters = []struct{ Key, Label string }{
	{"mine", "Le mie"},
	{"others", "Pubbliche di altri"},
	{"used", "In uso nei piani"},
	{"public", "Rese pubbliche"},
}

// matchRecipeFilter says whether the row belongs to filter key.
func matchRecipeFilter(key string, row recipeListRow) bool {
	switch key {
	case "others":
		return !row.Mine
	case "used":
		return row.Mine && row.Usages > 0
	case "public":
		return row.Mine && row.Recipe.IsPublic
	default: // mine
		return row.Mine
	}
}

// buildRecipesList filters the rows by filter and search (case-insensitive on
// the name) and counts the recipes of every filter, search excluded.
func buildRecipesList(all []recipeListRow, filter, query string) recipesListData {
	valid := false
	for _, f := range recipeFilters {
		valid = valid || f.Key == filter
	}
	if !valid {
		filter = "mine"
	}
	data := recipesListData{Filter: filter, Query: query}
	q := strings.ToLower(query)
	for _, f := range recipeFilters {
		rf := recipeFilter{Key: f.Key, Label: f.Label, On: f.Key == filter}
		for _, row := range all {
			if matchRecipeFilter(f.Key, row) {
				rf.Count++
			}
		}
		data.Filters = append(data.Filters, rf)
	}
	for _, row := range all {
		if row.Mine {
			data.MineN++
		} else {
			data.OthersN++
		}
		if matchRecipeFilter(filter, row) && strings.Contains(strings.ToLower(row.Recipe.Name), q) {
			data.Rows = append(data.Rows, row)
		}
	}
	return data
}

type recipeNewData struct {
	authPageData
	Form  recipeForm
	Error string

	// Fields filled after an import from a URL (schema.org/Recipe).
	ImportError         string   // fetch/parse error, shown in the import panel
	ImportedIngredients []string // ingredient lines detected (to map by hand)
	ImportSource        string   // source URL, shown for reference
}

type recipeForm struct {
	Name        string
	Description string
	Servings    string
	IsPublic    bool
}

// recipeDuplicateData is the view model of the duplicate modal.
type recipeDuplicateData struct {
	authPageData
	SrcID   int64
	SrcName string
	NewName string // prefilled value (default "Copia di X")
	Error   string
}

type recipeEditData struct {
	authPageData
	Recipe      *domain.RecipeWithItems
	Form        recipeForm
	Error       string
	ItemError   string
	CanEdit     bool                 // true when the logged-in user is the author
	Total       macros.Macros        // recipe total
	PerServing  macros.Macros        // per serving
	Shares      macros.Shares        // energy split per serving
	ItemKcal    map[int64]float64    // kcal of every item (by item id)
	Usages      int                  // weekly plan items that use it
	Ingredients []*domain.Ingredient // pool for the new item's select
	NewItemQty  string               // value in the item form after an error
	NewItemIng  string               // selected ingredient_id
}

// ---- list ------------------------------------------------------------------

func (a *App) handleRecipesList(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	all, err := a.Recipes.ListVisible(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	usages, err := a.Recipes.UsagesByRecipe(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	perServing, err := a.Recipes.PerServingByRecipe(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows := make([]recipeListRow, 0, len(all))
	for _, rec := range all {
		rows = append(rows, recipeListRow{
			Recipe:     rec,
			Mine:       rec.UserID == user.ID,
			Usages:     usages[rec.ID],
			PerServing: perServing[rec.ID],
		})
	}
	q := r.URL.Query()
	data := buildRecipesList(rows, q.Get("f"), strings.TrimSpace(q.Get("q")))
	data.authPageData = a.baseData(r)
	// The live search (HTMX) replaces only the results.
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "recipe-results" {
		if err := a.Render.RenderPartial(w, http.StatusOK, "recipes_list", "recipe_results", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	if err := a.Render.Render(w, http.StatusOK, "recipes_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- new -------------------------------------------------------------------

func (a *App) handleRecipeNewForm(w http.ResponseWriter, r *http.Request) {
	data := recipeNewData{
		authPageData: a.baseData(r),
		Form:         recipeForm{Servings: "1"},
	}
	if err := a.Render.Render(w, http.StatusOK, "recipe_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleRecipeImport handles POST /recipes/import: it downloads the given
// page, extracts the schema.org/Recipe recipe and re-renders /recipes/new with
// the form prefilled. The detected ingredients are free text (schema.org does
// not structure them) and are shown as a reference to map by hand.
func (a *App) handleRecipeImport(w http.ResponseWriter, r *http.Request) {
	srcURL := strings.TrimSpace(r.PostFormValue("import_url"))
	renderImport := func(data recipeNewData) {
		if err := a.Render.Render(w, http.StatusOK, "recipe_new", data); err != nil {
			a.serverError(w, r, err)
		}
	}

	if srcURL == "" {
		renderImport(recipeNewData{
			authPageData: a.baseData(r),
			Form:         recipeForm{Servings: "1"},
			ImportError:  "Incolla l'URL di una ricetta da importare.",
		})
		return
	}

	parsed, err := recipeimport.NewFetcher().FetchAndParse(r.Context(), srcURL)
	if err != nil {
		a.Logger.Info("recipe import failed", "url", srcURL, "err", err)
		msg := "Impossibile importare la ricetta da questo URL."
		if errors.Is(err, recipeimport.ErrNoRecipe) {
			msg = "La pagina non contiene una ricetta in formato strutturato (schema.org/Recipe)."
		} else if errors.Is(err, recipeimport.ErrInvalidURL) {
			msg = "URL non valido: usa un link http(s) verso un sito pubblico."
		}
		renderImport(recipeNewData{
			authPageData: a.baseData(r),
			Form:         recipeForm{Servings: "1"},
			ImportError:  msg,
			ImportSource: srcURL,
		})
		return
	}

	// Normalize the fields within the form limits.
	name := parsed.Name
	if !withinLen(name, MaxNameLen) {
		name = truncateRunes(name, MaxNameLen)
	}
	desc := parsed.Description
	if !withinLen(desc, MaxDescriptionLen) {
		desc = truncateRunes(desc, MaxDescriptionLen)
	}
	servings := "1"
	if parsed.Servings > 0 {
		servings = strconv.Itoa(parsed.Servings)
	}

	renderImport(recipeNewData{
		authPageData: a.baseData(r),
		Form: recipeForm{
			Name:        name,
			Description: desc,
			Servings:    servings,
		},
		ImportedIngredients: parsed.Ingredients,
		ImportSource:        srcURL,
	})
}

func (a *App) handleRecipeCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	form := recipeForm{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Servings:    strings.TrimSpace(r.PostFormValue("servings")),
		IsPublic:    r.PostFormValue("is_public") == "on",
	}
	if form.Name == "" {
		a.renderRecipeNewError(w, r, form, "Il nome e' obbligatorio.")
		return
	}
	if !withinLen(form.Name, MaxNameLen) {
		a.renderRecipeNewError(w, r, form, "Nome troppo lungo (max 200 caratteri).")
		return
	}
	if !withinLen(form.Description, MaxDescriptionLen) {
		a.renderRecipeNewError(w, r, form, "Descrizione troppo lunga (max 2000 caratteri).")
		return
	}
	servings, err := strconv.Atoi(form.Servings)
	if err != nil || servings <= 0 {
		a.renderRecipeNewError(w, r, form, "Porzioni: serve un intero > 0.")
		return
	}
	var descPtr *string
	if form.Description != "" {
		descPtr = &form.Description
	}
	id, err := a.Recipes.Create(r.Context(), user.ID, form.Name, descPtr, servings, form.IsPublic)
	if err != nil {
		if errors.Is(err, db.ErrRecipeNameTaken) {
			a.renderRecipeNewError(w, r, form, "Esiste già una tua ricetta con questo nome.")
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta creata.")
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) renderRecipeNewError(w http.ResponseWriter, r *http.Request, form recipeForm, msg string) {
	data := recipeNewData{authPageData: a.baseData(r), Form: form, Error: msg}
	if err := a.Render.Render(w, http.StatusUnprocessableEntity, "recipe_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- edit / detail ---------------------------------------------------------

func (a *App) handleRecipeEdit(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeEdit(w, r, rec, "", "", "", "")
}

func (a *App) renderRecipeEdit(
	w http.ResponseWriter, r *http.Request,
	rec *domain.RecipeWithItems,
	formError, itemError, newItemQty, newItemIng string,
) {
	user := userFromContext(r.Context())
	canEdit := user != nil && user.ID == rec.UserID

	// Macro computation: sum item by item, then per serving.
	items := make([]macros.IngredientItem, 0, len(rec.Items))
	itemKcal := make(map[int64]float64, len(rec.Items))
	for _, it := range rec.Items {
		items = append(items, macros.IngredientItem{Ingredient: it.Ingredient, Quantity: it.Quantity})
		itemKcal[it.ID] = macros.FromIngredient(it.Ingredient, it.Quantity).Kcal
	}
	total := macros.ForRecipe(items)
	perServing := macros.PerServing(total, rec.Servings)

	desc := ""
	if rec.Description != nil {
		desc = *rec.Description
	}
	data := recipeEditData{
		authPageData: a.baseData(r),
		Recipe:       rec,
		Form: recipeForm{
			Name:        rec.Name,
			Description: desc,
			Servings:    strconv.Itoa(rec.Servings),
			IsPublic:    rec.IsPublic,
		},
		Error:      formError,
		ItemError:  itemError,
		CanEdit:    canEdit,
		Total:      total,
		PerServing: perServing,
		Shares:     perServing.EnergyShares(),
		ItemKcal:   itemKcal,
		NewItemQty: newItemQty,
		NewItemIng: newItemIng,
	}
	if canEdit {
		usages, err := a.Recipes.CountMealUsages(r.Context(), rec.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Usages = usages
		// Load the ingredients for the select only when the user may edit.
		ings, err := a.Ingredients.ListActive(r.Context(), 0, 200)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Ingredients = ings
	}
	status := http.StatusOK
	if formError != "" || itemError != "" {
		status = http.StatusUnprocessableEntity
	}
	if err := a.Render.Render(w, status, "recipe_edit", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleRecipeUpdate gestisce POST /recipes/{id} (modifica meta).
func (a *App) handleRecipeUpdate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	form := recipeForm{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Servings:    strings.TrimSpace(r.PostFormValue("servings")),
		IsPublic:    r.PostFormValue("is_public") == "on",
	}
	if form.Name == "" {
		a.reRenderEdit(w, r, id, "Il nome e' obbligatorio.", "")
		return
	}
	if !withinLen(form.Name, MaxNameLen) {
		a.reRenderEdit(w, r, id, "Nome troppo lungo (max 200 caratteri).", "")
		return
	}
	if !withinLen(form.Description, MaxDescriptionLen) {
		a.reRenderEdit(w, r, id, "Descrizione troppo lunga (max 2000 caratteri).", "")
		return
	}
	servings, err := strconv.Atoi(form.Servings)
	if err != nil || servings <= 0 {
		a.reRenderEdit(w, r, id, "Porzioni: serve un intero > 0.", "")
		return
	}
	// Draft: saving the metadata is blocked until the user changes the
	// ingredients (add/remove), making the content differ from the other
	// recipes. The message names the recipe it is still identical to, to
	// guide the user.
	if rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID); err == nil && rec.IsDraft {
		simulated := make([]domain.RecipeItem, 0, len(rec.Items))
		for _, it := range rec.Items {
			simulated = append(simulated, it.RecipeItem)
		}
		other, _ := a.Recipes.IdenticalContentRecipe(r.Context(), user.ID, simulated, id)
		msg := "Questa ricetta è una bozza. Modifica almeno un ingrediente (aggiungi, rimuovi o cambia quantità) prima di salvare."
		if other != "" {
			msg = "Questa bozza ha gli stessi ingredienti di \"" + other + "\". Modifica almeno un ingrediente prima di salvare."
		}
		a.reRenderEdit(w, r, id, msg, "")
		return
	}
	var descPtr *string
	if form.Description != "" {
		descPtr = &form.Description
	}
	if err := a.Recipes.Update(r.Context(), id, user.ID, form.Name, descPtr, servings, form.IsPublic); err != nil {
		if errors.Is(err, db.ErrRecipeNameTaken) {
			a.reRenderEdit(w, r, id, "Esiste già una tua ricetta con questo nome.", "")
			return
		}
		a.handleRecipeError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta aggiornata.")
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipeAddItem POST /recipes/{id}/items. Accepts one of:
//   - ingredient_id: id of an ingredient already in the local DB.
//   - barcode: OFF code of a product NOT imported yet; the server
//     auto-imports it (idempotent: when already there it fetches the existing one).
//
// The picker_results partial makes the distinction based on the kind of the
// row clicked.
func (a *App) handleRecipeAddItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	qtyStr := strings.TrimSpace(r.PostFormValue("quantity"))
	qty, err := parseNonNegFloat(qtyStr)
	if err != nil || qty <= 0 {
		a.reRenderEditItem(w, r, id, "Quantita' non valida.", qtyStr, r.PostFormValue("ingredient_id"))
		return
	}

	// Resolve the target ingredient. Two ways:
	barcode := strings.TrimSpace(r.PostFormValue("barcode"))
	var ingID int64
	if barcode != "" {
		// Idempotent OFF auto-import.
		resolved, err := a.resolveOFFBarcode(r.Context(), barcode)
		if err != nil {
			a.reRenderEditItem(w, r, id, "Import OFF fallito: "+err.Error(), qtyStr, "")
			return
		}
		ingID = resolved
	} else {
		ingIDStr := r.PostFormValue("ingredient_id")
		parsed, err := strconv.ParseInt(ingIDStr, 10, 64)
		if err != nil || parsed <= 0 {
			a.reRenderEditItem(w, r, id, "Seleziona un ingrediente.", qtyStr, ingIDStr)
			return
		}
		ingID = parsed
	}

	// recipe_ingredients are stored in grams/ml: when the user picked "pz" in
	// the picker, convert right away by multiplying by the unit's average
	// weight (UnitWeightG).
	if strings.TrimSpace(r.PostFormValue("qty_unit")) == "pz" {
		ing, gerr := a.Ingredients.GetByID(r.Context(), ingID)
		if gerr == nil && ing.UnitWeightG != nil && *ing.UnitWeightG > 0 {
			qty = qty * (*ing.UnitWeightG)
		}
	}

	// Identity check: make sure the resulting state is not identical to
	// another recipe of the user. Load the current items + simulate the add.
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	simulated := make([]domain.RecipeItem, 0, len(rec.Items)+1)
	for _, it := range rec.Items {
		simulated = append(simulated, it.RecipeItem)
	}
	simulated = append(simulated, domain.RecipeItem{IngredientID: ingID, Quantity: qty})
	if other, err := a.Recipes.IdenticalContentRecipe(r.Context(), user.ID, simulated, id); err != nil {
		a.serverError(w, r, err)
		return
	} else if other != "" {
		a.reRenderEditItem(w, r, id,
			"Aggiungendo questo ingrediente la ricetta diventerebbe identica a \""+other+"\". Modifica le quantità o cambia ingrediente.",
			qtyStr, strconv.FormatInt(ingID, 10))
		return
	}

	if _, err := a.Recipes.AddItem(r.Context(), id, user.ID, ingID, qty); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	// The identity check above guarantees the resulting content differs from
	// every other recipe. When it was a draft, now is the time to promote it.
	if rec.IsDraft {
		if err := a.Recipes.PromoteDraft(r.Context(), id, user.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipePicker GET /recipes/{id}/picker?q=...
// Renders the picker_results partial in recipe_item mode.
// Uses the same searchMixed as the /ingredients page (local + OFF).
func (a *App) handleRecipePicker(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	// Check that the user is the author (only the author may edit).
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	if rec.UserID != user.ID {
		http.Error(w, "non sei l'autore", http.StatusForbidden)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, warn, err := a.searchMixed(r.Context(), q, includeOFFParam(r))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := ingredientsListData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Rows:         rows,
		Query:        q,
		OFFWarning:   warn,
		PickerKind:   "recipe_item",
		RecipeID:     id,
		SearchedOFF:  includeOFFParam(r),
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "picker_results", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleRecipeRemoveItem POST /recipes/{id}/items/{itemID}/delete (form-driven).
// HTMX swap of the page: 303 redirect to the edit page.
func (a *App) handleRecipeRemoveItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}

	// Identity check: simulate the removal and make sure the resulting state
	// is not identical to another recipe of the user.
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	simulated := make([]domain.RecipeItem, 0, len(rec.Items))
	for _, it := range rec.Items {
		if it.ID == itemID {
			continue
		}
		simulated = append(simulated, it.RecipeItem)
	}
	if other, err := a.Recipes.IdenticalContentRecipe(r.Context(), user.ID, simulated, id); err != nil {
		a.serverError(w, r, err)
		return
	} else if other != "" {
		a.reRenderEdit(w, r, id, "",
			"Rimuovendo questo ingrediente la ricetta diventerebbe identica a \""+other+"\". Mantieni la voce o modifica altre quantità.")
		return
	}

	if err := a.Recipes.RemoveItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrRecipeItemNotFound) {
			http.Error(w, "voce non trovata", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	// Draft promotion, as in handleRecipeAddItem.
	if rec.IsDraft {
		if err := a.Recipes.PromoteDraft(r.Context(), id, user.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipeDelete POST /recipes/{id}/delete. Counts the occurrences in the
// weekly plans before the delete, because the FK ON DELETE CASCADE
// (migration 0008) removes them silently: the flash tells the user.
func (a *App) handleRecipeDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	usages, _ := a.Recipes.CountMealUsages(r.Context(), id)
	if err := a.Recipes.Delete(r.Context(), id, user.ID); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	msg := "Ricetta eliminata."
	if usages > 0 {
		msg = fmt.Sprintf("Ricetta eliminata. Rimossa anche da %d %s nei piani settimanali.",
			usages, pluralize(usages, "voce", "voci"))
	}
	a.Sessions.Put(r.Context(), "flash", msg)
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

// handleRecipesBulkDelete POST /recipes/bulk-delete. Deletes several recipes
// in a single call. Recipes the user does not own are silently ignored
// (BulkDelete filters by user_id).
func (a *App) handleRecipesBulkDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form invalido", http.StatusBadRequest)
		return
	}
	raw := r.PostForm["recipe_id"]
	ids := make([]int64, 0, len(raw))
	for _, s := range raw {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		a.Sessions.Put(r.Context(), "flash", "Nessuna ricetta selezionata.")
		http.Redirect(w, r, "/recipes", http.StatusSeeOther)
		return
	}
	usages, _ := a.Recipes.CountMealUsagesMulti(r.Context(), ids)
	n, err := a.Recipes.BulkDelete(r.Context(), ids, user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var msg string
	switch {
	case n == 0:
		msg = "Nessuna ricetta eliminata."
	case n == 1:
		msg = "1 ricetta eliminata."
	default:
		msg = fmt.Sprintf("%d ricette eliminate.", n)
	}
	if usages > 0 && n > 0 {
		msg += fmt.Sprintf(" Rimosse %d %s nei piani settimanali.",
			usages, pluralize(usages, "voce", "voci"))
	}
	a.Sessions.Put(r.Context(), "flash", msg)
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

// handleRecipeDuplicateForm GET /recipes/{id}/duplicate
// Renders the modal with the "new name" field prefilled to "Copia di X".
func (a *App) handleRecipeDuplicateForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	src, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeDuplicateModal(w, r, recipeDuplicateData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		SrcID:        src.ID,
		SrcName:      src.Name,
		NewName:      proposeDuplicateName(src.Name),
	})
}

// handleRecipeDuplicate POST /recipes/{id}/duplicate
// Validates the new name (required, different from the original, unique) and
// makes a deep copy. Answers with HX-Redirect to the new recipe's edit page.
func (a *App) handleRecipeDuplicate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	srcID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	// Load src first of all so renderErr can pass SrcName and the text
	// "Stai duplicando X" stays visible on errors too.
	src, err := a.Recipes.GetWithItems(r.Context(), srcID, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}

	newName := strings.TrimSpace(r.PostFormValue("new_name"))

	renderErr := func(msg string) {
		a.renderRecipeDuplicateFormOnly(w, r, recipeDuplicateData{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			SrcID:        srcID,
			SrcName:      src.Name,
			NewName:      newName,
			Error:        msg,
		})
	}

	if newName == "" {
		renderErr("Inserisci un nome per la copia.")
		return
	}
	if !withinLen(newName, MaxNameLen) {
		renderErr("Nome troppo lungo (max 200 caratteri).")
		return
	}
	if strings.EqualFold(newName, src.Name) {
		renderErr("Il nome della copia deve essere diverso dall'originale.")
		return
	}

	newID, err := a.Recipes.Duplicate(r.Context(), srcID, user.ID, newName)
	if err != nil {
		if errors.Is(err, db.ErrRecipeNameTaken) {
			renderErr("Esiste già una tua ricetta con questo nome.")
			return
		}
		a.handleRecipeError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta duplicata. Modifica gli ingredienti per personalizzarla.")
	// HTMX honors HX-Redirect and does a full reload of the given URL.
	w.Header().Set("HX-Redirect", "/recipes/"+strconv.FormatInt(newID, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) renderRecipeDuplicateModal(w http.ResponseWriter, r *http.Request, data recipeDuplicateData) {
	mdata := modalDialogData{
		Title:     "Duplica ricetta",
		Inner:     "recipe_duplicate_form",
		InnerData: data,
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "recipes_list", "modal_dialog", mdata); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) renderRecipeDuplicateFormOnly(w http.ResponseWriter, r *http.Request, data recipeDuplicateData) {
	// 200 (not 422) because HTMX 1.x by default swaps only on 2xx: with 422
	// the response would be ignored and the user would not see the inline warning.
	if err := a.Render.RenderPartial(w, http.StatusOK, "recipes_list", "recipe_duplicate_form", data); err != nil {
		a.serverError(w, r, err)
	}
}

// proposeDuplicateName returns a plausible name for the copy. When the user
// then confirms and that name is taken, the UNIQUE constraint sends it back
// with an explicit error.
func proposeDuplicateName(name string) string {
	return "Copia di " + name
}

// pluralize picks singular/plural based on the count.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// ---- picker ricette --------------------------------------------------------

// recipePickerData is the view model of recipe_picker_results, the recipe
// picker of meals, cheats and nutrition plan variations.
type recipePickerData struct {
	authPageData
	Recipes     []*domain.Recipe
	Query       string
	Kcal        map[int64]float64 // kcal per serving, by recipe id
	VariationID int64             // > 0: a nutrition plan variation
	Cheat       bool              // a cheat instead of a meal item
	MealID      int64
	WeekStart   string
	InDialog    bool // picker in the dialog over the week (HTMX form)
}

// recipePicker loads the recipes visible to the user whose name contains q
// (case-insensitive), with the kcal per serving.
func (a *App) recipePicker(r *http.Request, q string) (recipePickerData, error) {
	user := userFromContext(r.Context())
	recipes, err := a.Recipes.ListVisible(r.Context(), user.ID)
	if err != nil {
		return recipePickerData{}, err
	}
	perServing, err := a.Recipes.PerServingByRecipe(r.Context(), user.ID)
	if err != nil {
		return recipePickerData{}, err
	}
	ql := strings.ToLower(q)
	data := recipePickerData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Query:        q,
		Kcal:         make(map[int64]float64, len(recipes)),
	}
	for _, rc := range recipes {
		if strings.Contains(strings.ToLower(rc.Name), ql) {
			data.Recipes = append(data.Recipes, rc)
			data.Kcal[rc.ID] = perServing[rc.ID].Kcal
		}
	}
	return data, nil
}

// ---- helpers ---------------------------------------------------------------

func (a *App) reRenderEdit(w http.ResponseWriter, r *http.Request, id int64, formErr, itemErr string) {
	user := userFromContext(r.Context())
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeEdit(w, r, rec, formErr, itemErr, "", "")
}

func (a *App) reRenderEditItem(w http.ResponseWriter, r *http.Request, id int64, itemErr, qty, ingID string) {
	user := userFromContext(r.Context())
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeEdit(w, r, rec, "", itemErr, qty, ingID)
}

func (a *App) handleRecipeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrRecipeNotFound):
		a.notFound(w, r)
	case errors.Is(err, db.ErrRecipeForbidden):
		http.Error(w, "operazione non consentita", http.StatusForbidden)
	default:
		a.serverError(w, r, err)
	}
}
