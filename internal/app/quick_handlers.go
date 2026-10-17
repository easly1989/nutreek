package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/off"
)

// Quick handlers: "create on the fly" modal forms for a local ingredient and
// an empty recipe. Rendered inside <dialog> in #modal-host (see base.html).
// On success they return 204 + HX-Trigger=ingredient-created (or
// recipe-created): the client (modal.js) closes the dialog and dispatches the
// event to the sortables/pickers that can reload.

// ---- ingredient on the fly ------------------------------------------------

type quickIngredientData struct {
	authPageData
	Form  ingredientForm
	Error string
}

type modalDialogData struct {
	Title     string
	Inner     string
	InnerData any
}

// handleQuickIngredientForm GET /quick/ingredient — returns the dialog with
// the empty form (or filled with errors after a failed POST).
// ?barcode= comes from the scanner's «Crea l'ingrediente»: the code stays
// linked to the new ingredient.
func (a *App) handleQuickIngredientForm(w http.ResponseWriter, r *http.Request) {
	form := ingredientForm{Unit: "g"}
	form.Barcode, _ = off.NormalizeBarcode(r.URL.Query().Get("barcode"))
	a.renderQuickIngredientModal(w, r, form, "")
}

func (a *App) renderQuickIngredientModal(w http.ResponseWriter, r *http.Request, form ingredientForm, errMsg string) {
	data := modalDialogData{
		Title: a.t(r, "ingredients.new"),
		Inner: "quick_ingredient_form",
		InnerData: quickIngredientData{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			Form:         form,
			Error:        errMsg,
		},
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "ingredients_list", "modal_dialog", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) renderQuickIngredientFormOnly(w http.ResponseWriter, r *http.Request, form ingredientForm, errMsg string) {
	// Re-render after a validation error: return only the form
	// (hx-target=this hx-swap=outerHTML replaces it inside the dialog, which
	// stays open). 200 and not 422: HTMX 1.x swaps only on 2xx.
	data := quickIngredientData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Form:         form,
		Error:        errMsg,
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "ingredients_list", "quick_ingredient_form", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleQuickIngredientCreate POST /quick/ingredient — creates the ingredient.
// On success it sets HX-Trigger=ingredient-created and returns 204 (no body):
// modal.js closes the dialog and the registered pickers reload their results.
func (a *App) handleQuickIngredientCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	form := ingredientForm{
		Name:    strings.TrimSpace(r.PostFormValue("name")),
		Brand:   strings.TrimSpace(r.PostFormValue("brand")),
		Unit:    strings.TrimSpace(r.PostFormValue("unit")),
		Kcal:    strings.TrimSpace(r.PostFormValue("kcal")),
		Protein: strings.TrimSpace(r.PostFormValue("protein")),
		Carbs:   strings.TrimSpace(r.PostFormValue("carbs")),
		Fat:     strings.TrimSpace(r.PostFormValue("fat")),

		UnitWeightG: strings.TrimSpace(r.PostFormValue("unit_weight_g")),
	}
	// An invalid code is ignored: the ingredient is created anyway.
	form.Barcode, _ = off.NormalizeBarcode(r.PostFormValue("barcode"))
	if form.Barcode != "" {
		if got, err := a.Ingredients.GetByBarcode(r.Context(), form.Barcode); err == nil {
			a.renderQuickIngredientFormOnly(w, r, form, a.t(r, "ingredients.quick.barcode_taken", "Code", form.Barcode, "Name", got.Name))
			return
		}
	}
	vals, msg := a.validateIngredient(r, form)
	if msg != "" {
		a.renderQuickIngredientFormOnly(w, r, form, msg)
		return
	}

	var brandPtr *string
	if form.Brand != "" {
		brandPtr = &form.Brand
	}
	// Optional weight of one piece: lets it be added right away in «pz».
	var unitWeightG *float64
	if form.UnitWeightG != "" {
		v, err := parseNonNegFloat(form.UnitWeightG)
		if err != nil || v == 0 {
			a.renderQuickIngredientFormOnly(w, r, form, a.t(r, "ingredients.error.piece_weight"))
			return
		}
		unitWeightG = &v
	}
	creator := user.ID
	id, err := a.Ingredients.CreateLocal(r.Context(), form.Name, brandPtr, vals.Unit, vals.Kcal, vals.Protein, vals.Carbs, vals.Fat, &creator, unitWeightG, nil, nil, nil, nil, nil)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if form.Barcode != "" {
		// The check above covers the normal case; what remains here is the race
		// with another import of the same code: the ingredient exists anyway.
		if err := a.Ingredients.SetLocalBarcode(r.Context(), id, form.Barcode); err != nil {
			a.Logger.Warn("linking the code to the new ingredient failed", "err", err, "barcode", form.Barcode)
		}
	}
	w.Header().Set("HX-Trigger", `{"ingredient-created":{"id":`+strconv.FormatInt(id, 10)+`,"name":`+jsonString(form.Name)+`}}`)
	w.WriteHeader(http.StatusNoContent)
}

// ---- recipe on the fly ----------------------------------------------------

type quickRecipeData struct {
	authPageData
	Form  recipeForm
	Error string
}

func (a *App) handleQuickRecipeForm(w http.ResponseWriter, r *http.Request) {
	form := recipeForm{Servings: "1"}
	a.renderQuickRecipeModal(w, r, form, "")
}

func (a *App) renderQuickRecipeModal(w http.ResponseWriter, r *http.Request, form recipeForm, errMsg string) {
	data := modalDialogData{
		Title: "Nuova ricetta",
		Inner: "quick_recipe_form",
		InnerData: quickRecipeData{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			Form:         form,
			Error:        errMsg,
		},
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "ingredients_list", "modal_dialog", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) renderQuickRecipeFormOnly(w http.ResponseWriter, r *http.Request, form recipeForm, errMsg string) {
	data := quickRecipeData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Form:         form,
		Error:        errMsg,
	}
	// 200 and not 422: HTMX 1.x swaps only on 2xx.
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "ingredients_list", "quick_recipe_form", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleQuickRecipeCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	form := recipeForm{
		Name:     strings.TrimSpace(r.PostFormValue("name")),
		Servings: strings.TrimSpace(r.PostFormValue("servings")),
	}
	if form.Name == "" || !withinLen(form.Name, MaxNameLen) {
		a.renderQuickRecipeFormOnly(w, r, form, "Nome ricetta non valido.")
		return
	}
	servings, err := strconv.Atoi(form.Servings)
	if err != nil || servings <= 0 {
		a.renderQuickRecipeFormOnly(w, r, form, "Porzioni non valide.")
		return
	}
	id, err := a.Recipes.Create(r.Context(), user.ID, form.Name, nil, servings, false)
	if err != nil {
		if errors.Is(err, db.ErrRecipeNameTaken) {
			a.renderQuickRecipeFormOnly(w, r, form, "Esiste già una tua ricetta con questo nome.")
			return
		}
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("HX-Trigger", `{"recipe-created":{"id":`+strconv.FormatInt(id, 10)+`,"name":`+jsonString(form.Name)+`}}`)
	w.WriteHeader(http.StatusNoContent)
}

// jsonString does a minimal encoding of a JSON string (escaping only quotes
// and backslashes). Used for the HX-Trigger payload; for more complex texts
// we would use encoding/json.
func jsonString(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			if c < 0x20 {
				continue
			}
			out = append(out, c)
		}
	}
	out = append(out, '"')
	return string(out)
}
