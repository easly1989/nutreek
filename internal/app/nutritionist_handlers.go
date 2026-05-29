package app

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/domain"
)

type nutritionistPageData struct {
	authPageData
	Plan *domain.NutritionistPlanFull
}

func (a *App) handleNutritionistPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusFound)
}

// pianoNutrizionaleData è il view model della pagina unificata /piano-nutrizionale.
type pianoNutrizionaleData struct {
	authPageData
	Plan      *domain.NutritionistPlanFull
	Templates []*domain.MealTemplate
	ActiveTab string // "piano" | "template"
}

func (a *App) handlePianoNutrizionale(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	plan, err := a.NutritionistPlans.LoadPlanFull(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	tpls, err := a.MealTemplates.ListByUser(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	tab := r.URL.Query().Get("tab")
	if tab != "template" {
		tab = "piano"
	}
	data := pianoNutrizionaleData{
		authPageData: a.baseData(r),
		Plan:         plan,
		Templates:    tpls,
		ActiveTab:    tab,
	}
	if err := a.Render.Render(w, http.StatusOK, "piano_nutrizionale", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleNutritionistCreateMealType(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		a.Sessions.Put(r.Context(), "flash", "Il nome del tipo pasto e' obbligatorio.")
		http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreatePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.NutritionistPlans.CreateMealType(r.Context(), plan.ID, name); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
}

func (a *App) handleNutritionistDeleteMealType(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreatePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.NutritionistPlans.DeleteMealType(r.Context(), id, plan.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
}

func (a *App) handleNutritionistCreateVariation(w http.ResponseWriter, r *http.Request) {
	mealTypeIDStr := strings.TrimSpace(r.PostFormValue("meal_type_id"))
	nameStr := strings.TrimSpace(r.PostFormValue("name"))

	mealTypeID, err := strconv.ParseInt(mealTypeIDStr, 10, 64)
	if err != nil || mealTypeID <= 0 {
		a.Sessions.Put(r.Context(), "flash", "Tipo pasto non valido.")
		http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
		return
	}

	var namePtr *string
	if nameStr != "" {
		namePtr = &nameStr
	}

	if _, err := a.NutritionistPlans.CreateVariation(r.Context(), mealTypeID, namePtr); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
}

func (a *App) handleNutritionistDeleteVariation(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreatePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.NutritionistPlans.DeleteVariation(r.Context(), id, plan.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
}

func (a *App) handleNutritionistAddItem(w http.ResponseWriter, r *http.Request) {
	variationIDStr := strings.TrimSpace(r.PostFormValue("variation_id"))
	quantityStr := strings.TrimSpace(r.PostFormValue("quantity"))
	unit := strings.TrimSpace(r.PostFormValue("unit"))
	ingredientIDStr := strings.TrimSpace(r.PostFormValue("ingredient_id"))
	recipeIDStr := strings.TrimSpace(r.PostFormValue("recipe_id"))

	variationID, err := strconv.ParseInt(variationIDStr, 10, 64)
	if err != nil || variationID <= 0 {
		a.Sessions.Put(r.Context(), "flash", "Variante non valida.")
		http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
		return
	}

	quantityStr = strings.ReplaceAll(quantityStr, ",", ".")
	quantity, err := strconv.ParseFloat(quantityStr, 64)
	if err != nil || quantity <= 0 {
		a.Sessions.Put(r.Context(), "flash", "Quantita' non valida.")
		http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
		return
	}

	if unit == "" {
		unit = "g"
	}

	var ingIDPtr, recipeIDPtr *int64
	if ingredientIDStr != "" {
		if id, err2 := strconv.ParseInt(ingredientIDStr, 10, 64); err2 == nil {
			ingIDPtr = &id
		}
	}
	if recipeIDStr != "" {
		if id, err2 := strconv.ParseInt(recipeIDStr, 10, 64); err2 == nil {
			recipeIDPtr = &id
		}
	}

	if ingIDPtr == nil && recipeIDPtr == nil {
		a.Sessions.Put(r.Context(), "flash", "Seleziona un ingrediente o una ricetta.")
		http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
		return
	}

	if _, err := a.NutritionistPlans.AddItem(r.Context(), variationID, recipeIDPtr, ingIDPtr, quantity, unit); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
}

func (a *App) handleNutritionistDeleteItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreatePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.NutritionistPlans.DeleteItem(r.Context(), id, plan.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/nutrizionista", http.StatusSeeOther)
}
