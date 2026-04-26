package app

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
)

// ---- pages -----------------------------------------------------------------

type ingredientsListData struct {
	authPageData
	Ingredients []*domain.Ingredient
	Query       string
}

type ingredientsNewData struct {
	authPageData
	Form  ingredientForm
	Error string
}

// ingredientForm cattura i campi del form "nuovo ingrediente" per poter
// rifare il render in caso di errore senza perdere i valori inseriti.
type ingredientForm struct {
	Name    string
	Brand   string
	Unit    string
	Kcal    string
	Protein string
	Carbs   string
	Fat     string
}

func (a *App) handleIngredientsList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var (
		list []*domain.Ingredient
		err  error
	)
	if q != "" {
		list, err = a.Ingredients.SearchActive(r.Context(), q, 50)
	} else {
		list, err = a.Ingredients.ListActive(r.Context(), 0, 50)
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := ingredientsListData{
		authPageData: a.baseData(r),
		Ingredients:  list,
		Query:        q,
	}
	if err := a.Render.Render(w, http.StatusOK, "ingredients_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleIngredientsSearchPartial risponde a HTMX con il solo partial
// dei risultati. Usato dall'input live-search.
func (a *App) handleIngredientsSearchPartial(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	list, err := a.Ingredients.SearchActive(r.Context(), q, 20)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := ingredientsListData{Ingredients: list, Query: q}
	if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "ingredient_results", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleIngredientNewForm(w http.ResponseWriter, r *http.Request) {
	data := ingredientsNewData{
		authPageData: a.baseData(r),
		Form:         ingredientForm{Unit: "g"},
	}
	if err := a.Render.Render(w, http.StatusOK, "ingredient_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleIngredientCreate(w http.ResponseWriter, r *http.Request) {
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
		a.renderIngredientError(w, r, form, "Il nome e' obbligatorio.")
		return
	}
	if len(form.Name) > 200 {
		a.renderIngredientError(w, r, form, "Nome troppo lungo (max 200 caratteri).")
		return
	}
	unit := domain.IngredientUnit(form.Unit)
	if unit != domain.UnitGram && unit != domain.UnitMilliliter {
		a.renderIngredientError(w, r, form, "Unita' non valida (usare g o ml).")
		return
	}

	kcal, err := parseNonNegFloat(form.Kcal)
	if err != nil {
		a.renderIngredientError(w, r, form, "Kcal/100 non valida: "+err.Error())
		return
	}
	protein, err := parseNonNegFloat(form.Protein)
	if err != nil {
		a.renderIngredientError(w, r, form, "Proteine/100 non valide: "+err.Error())
		return
	}
	carbs, err := parseNonNegFloat(form.Carbs)
	if err != nil {
		a.renderIngredientError(w, r, form, "Carbo/100 non validi: "+err.Error())
		return
	}
	fat, err := parseNonNegFloat(form.Fat)
	if err != nil {
		a.renderIngredientError(w, r, form, "Grassi/100 non validi: "+err.Error())
		return
	}

	var brandPtr *string
	if form.Brand != "" {
		brandPtr = &form.Brand
	}
	user := userFromContext(r.Context())
	var creatorID *int64
	if user != nil {
		id := user.ID
		creatorID = &id
	}

	if _, err := a.Ingredients.CreateLocal(r.Context(), form.Name, brandPtr, unit, kcal, protein, carbs, fat, creatorID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ingrediente creato.")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}

func (a *App) renderIngredientError(w http.ResponseWriter, r *http.Request, form ingredientForm, msg string) {
	data := ingredientsNewData{
		authPageData: a.baseData(r),
		Form:         form,
		Error:        msg,
	}
	if err := a.Render.Render(w, http.StatusUnprocessableEntity, "ingredient_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

// parseNonNegFloat accetta numeri con virgola o punto come separatore
// decimale (i form italiani spesso usano la virgola).
func parseNonNegFloat(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, errEmpty
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, errNegative
	}
	return v, nil
}

// errori esposti come var per dare messaggi parlanti senza pkg errors.
var (
	errEmpty    = simpleErr("valore mancante")
	errNegative = simpleErr("valore negativo")
)

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
