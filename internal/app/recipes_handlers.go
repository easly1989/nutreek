package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// ---- view models -----------------------------------------------------------

type recipesListData struct {
	authPageData
	Mine   []*domain.Recipe
	Others []*domain.Recipe // pubbliche di altri utenti
}

type recipeNewData struct {
	authPageData
	Form  recipeForm
	Error string
}

type recipeForm struct {
	Name        string
	Description string
	Servings    string
	IsPublic    bool
}

type recipeEditData struct {
	authPageData
	Recipe       *domain.RecipeWithItems
	Form         recipeForm
	Error        string
	ItemError    string
	CanEdit      bool                  // true se l'utente loggato e' l'autore
	Total        macros.Macros         // totale ricetta
	PerServing   macros.Macros         // per-serving
	Ingredients  []*domain.Ingredient  // pool per il select del nuovo item
	NewItemQty   string                // valore nel form item dopo errore
	NewItemIng   string                // ingredient_id selezionato
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
	data := recipesListData{authPageData: a.baseData(r)}
	for _, rec := range all {
		if rec.UserID == user.ID {
			data.Mine = append(data.Mine, rec)
		} else {
			data.Others = append(data.Others, rec)
		}
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
		http.NotFound(w, r)
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

	// Calcolo macro: somma item per item, poi per-serving.
	items := make([]macros.IngredientItem, 0, len(rec.Items))
	for _, it := range rec.Items {
		items = append(items, macros.IngredientItem{Ingredient: it.Ingredient, Quantity: it.Quantity})
	}
	total := macros.ForRecipe(items)

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
		PerServing: macros.PerServing(total, rec.Servings),
		NewItemQty: newItemQty,
		NewItemIng: newItemIng,
	}
	if canEdit {
		// Carichiamo gli ingredienti per il select solo se l'utente puo' editare.
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
		http.NotFound(w, r)
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
	servings, err := strconv.Atoi(form.Servings)
	if err != nil || servings <= 0 {
		a.reRenderEdit(w, r, id, "Porzioni: serve un intero > 0.", "")
		return
	}
	var descPtr *string
	if form.Description != "" {
		descPtr = &form.Description
	}
	if err := a.Recipes.Update(r.Context(), id, user.ID, form.Name, descPtr, servings, form.IsPublic); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta aggiornata.")
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipeAddItem POST /recipes/{id}/items.
func (a *App) handleRecipeAddItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ingIDStr := r.PostFormValue("ingredient_id")
	qtyStr := strings.TrimSpace(r.PostFormValue("quantity"))
	ingID, err := strconv.ParseInt(ingIDStr, 10, 64)
	if err != nil || ingID <= 0 {
		a.reRenderEditItem(w, r, id, "Seleziona un ingrediente.", qtyStr, ingIDStr)
		return
	}
	qty, err := parseNonNegFloat(qtyStr)
	if err != nil || qty <= 0 {
		a.reRenderEditItem(w, r, id, "Quantita' non valida.", qtyStr, ingIDStr)
		return
	}
	if _, err := a.Recipes.AddItem(r.Context(), id, user.ID, ingID, qty); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipeRemoveItem POST /recipes/{id}/items/{itemID}/delete (form-driven).
// HTMX swap della pagina: redirect 303 alla edit page.
func (a *App) handleRecipeRemoveItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	itemID, err := parseID(r, "itemID")
	if err != nil {
		http.NotFound(w, r)
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
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipeDelete POST /recipes/{id}/delete.
func (a *App) handleRecipeDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.Recipes.Delete(r.Context(), id, user.ID); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta eliminata.")
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
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
		http.NotFound(w, r)
	case errors.Is(err, db.ErrRecipeForbidden):
		http.Error(w, "operazione non consentita", http.StatusForbidden)
	default:
		a.serverError(w, r, err)
	}
}

func parseID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, key), 10, 64)
}
