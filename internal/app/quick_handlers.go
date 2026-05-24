package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// Quick handlers: form modal "crea al volo" per ingrediente locale e
// ricetta vuota. Renderizzati dentro <dialog> in #modal-host (vedi
// base.html). Su success ritornano 204 + HX-Trigger=ingredient-created
// (o recipe-created): il client (modal.js) chiude il dialog e dispatcha
// l'evento sui sortable/picker che possono ricaricarsi.

// ---- ingrediente al volo --------------------------------------------------

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

// handleQuickIngredientForm GET /quick/ingredient — restituisce il dialog con
// il form vuoto (o riempito di errori dopo un POST fallito).
func (a *App) handleQuickIngredientForm(w http.ResponseWriter, r *http.Request) {
	form := ingredientForm{Unit: "g"}
	a.renderQuickIngredientModal(w, r, form, "")
}

func (a *App) renderQuickIngredientModal(w http.ResponseWriter, r *http.Request, form ingredientForm, errMsg string) {
	data := modalDialogData{
		Title: "Nuovo ingrediente",
		Inner: "quick_ingredient_form",
		InnerData: quickIngredientData{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			Form:         form,
			Error:        errMsg,
		},
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "modal_dialog", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) renderQuickIngredientFormOnly(w http.ResponseWriter, r *http.Request, form ingredientForm, errMsg string) {
	// Per re-render dopo errore di validazione: ritorniamo solo il form
	// (hx-target=this hx-swap=outerHTML lo sostituisce dentro il dialog,
	// che resta aperto).
	data := quickIngredientData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Form:         form,
		Error:        errMsg,
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := a.Render.RenderPartial(w, http.StatusUnprocessableEntity, "ingredients_list", "quick_ingredient_form", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleQuickIngredientCreate POST /quick/ingredient — crea l'ingrediente.
// Su success setta HX-Trigger=ingredient-created e ritorna 204 (no body):
// modal.js chiude il dialog e i picker registrati ricaricano i risultati.
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
	}
	if form.Name == "" {
		a.renderQuickIngredientFormOnly(w, r, form, "Nome obbligatorio.")
		return
	}
	if !withinLen(form.Name, MaxNameLen) {
		a.renderQuickIngredientFormOnly(w, r, form, "Nome troppo lungo.")
		return
	}
	unit := domain.IngredientUnit(form.Unit)
	if unit != domain.UnitGram && unit != domain.UnitMilliliter {
		a.renderQuickIngredientFormOnly(w, r, form, "Unita' non valida.")
		return
	}
	kcal, err := parseNonNegFloat(form.Kcal)
	if err != nil {
		a.renderQuickIngredientFormOnly(w, r, form, "Kcal non valida.")
		return
	}
	protein, err := parseNonNegFloat(form.Protein)
	if err != nil {
		a.renderQuickIngredientFormOnly(w, r, form, "Proteine non valide.")
		return
	}
	carbs, err := parseNonNegFloat(form.Carbs)
	if err != nil {
		a.renderQuickIngredientFormOnly(w, r, form, "Carboidrati non validi.")
		return
	}
	fat, err := parseNonNegFloat(form.Fat)
	if err != nil {
		a.renderQuickIngredientFormOnly(w, r, form, "Grassi non validi.")
		return
	}

	var brandPtr *string
	if form.Brand != "" {
		brandPtr = &form.Brand
	}
	creator := user.ID
	id, err := a.Ingredients.CreateLocal(r.Context(), form.Name, brandPtr, unit, kcal, protein, carbs, fat, &creator)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("HX-Trigger", `{"ingredient-created":{"id":`+strconv.FormatInt(id, 10)+`,"name":`+jsonString(form.Name)+`}}`)
	w.WriteHeader(http.StatusNoContent)
}

// ---- ricetta al volo ------------------------------------------------------

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
	if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "modal_dialog", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) renderQuickRecipeFormOnly(w http.ResponseWriter, r *http.Request, form recipeForm, errMsg string) {
	data := quickRecipeData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Form:         form,
		Error:        errMsg,
	}
	if err := a.Render.RenderPartial(w, http.StatusUnprocessableEntity, "ingredients_list", "quick_recipe_form", data); err != nil {
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

// jsonString fa un encoding minimale di una stringa JSON (escape solo
// quote e backslash). Usato per il payload di HX-Trigger; per testi piu'
// complessi useremmo encoding/json.
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
