package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// nutritionistWriteError answers a write error on the plan: 404 when the
// element (or the recipe) is not the user's, 500 otherwise.
func (a *App) nutritionistWriteError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, db.ErrNutritionistItemNotFound) || errors.Is(err, db.ErrRecipeNotFound) {
		a.notFound(w, r)
		return
	}
	a.serverError(w, r, err)
}

func (a *App) handleNutritionistPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusFound)
}

// slotMealType pairs a template slot name with the matching meal type of the plan.
type slotMealType struct {
	SlotName string
	MealType *domain.NutritionistMealTypeFull
}

// pianoNutrizionaleData is the view model of the unified /piano-nutrizionale page.
type pianoNutrizionaleData struct {
	authPageData
	Plan              *domain.NutritionistPlanFull
	Templates         []*domain.MealTemplate
	ActiveTab         string // "piano" | "template"
	SlotMealTypes     []slotMealType
	HasActiveTemplate bool
	ReadOnly          bool   // true when the plan shown was assigned by the coach
	CoachName         string // name of the coach who assigned the plan
	ActiveTemplate    string // name of the active template the meals come from

	// Kcal of the variations and of the single items, by id.
	VariationKcal map[int64]float64
	ItemKcal      map[int64]float64
}

// planKcal computes the kcal of every variation and every item of the plan.
// Recipes count servings × kcal per serving (perServing, by recipe id).
func planKcal(plan *domain.NutritionistPlanFull, perServing map[int64]macros.Macros) (variations, items map[int64]float64) {
	variations = make(map[int64]float64)
	items = make(map[int64]float64)
	if plan == nil {
		return variations, items
	}
	for _, mt := range plan.MealTypes {
		for _, v := range mt.Variations {
			variations[v.ID] = macros.ForNutritionistVariation(v.Items, perServing).Kcal
			for _, it := range v.Items {
				items[it.ID] = macros.ForNutritionistVariation([]domain.NutritionistItem{it}, perServing).Kcal
			}
		}
	}
	return variations, items
}

func (a *App) handlePianoNutrizionale(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	// When the coach assigned a plan to the client, that is the effective
	// plan: it is shown read-only.
	var plan *domain.NutritionistPlanFull
	var readOnly bool
	var coachName string
	assignment, err := a.Coach.GetAssignment(ctx, user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if assignment != nil && assignment.AssignedPlanID != nil {
		plan, err = a.NutritionistPlans.LoadPlanFullByID(ctx, *assignment.AssignedPlanID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		readOnly = true
		coachName = assignment.CoachName
	} else {
		plan, err = a.NutritionistPlans.LoadPlanFull(ctx, user.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	tpls, err := a.MealTemplates.ListByUser(ctx, user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	tab := r.URL.Query().Get("tab")
	if tab != "template" {
		tab = "piano"
	}

	// Build the slot→meal-type list from the active template (only for the
	// user's own plan: the one assigned by the coach must not be changed).
	var slotMealTypes []slotMealType
	var hasActiveTemplate bool
	if !readOnly && len(tpls) > 0 && tpls[0].IsActive {
		hasActiveTemplate = true
		activeWithSlots, err := a.MealTemplates.GetWithSlots(ctx, tpls[0].ID, user.ID)
		if err == nil {
			// Unique slot names in order of first appearance.
			seen := make(map[string]bool)
			var slotNames []string
			for _, s := range activeWithSlots.Slots {
				key := strings.ToLower(s.Name)
				if !seen[key] {
					seen[key] = true
					slotNames = append(slotNames, s.Name)
				}
			}
			// Auto-create the missing meal types.
			for _, name := range slotNames {
				_ = a.NutritionistPlans.EnsureMealType(ctx, plan.ID, name)
			}
			// Reload after the upserts.
			plan, _ = a.NutritionistPlans.LoadPlanFull(ctx, user.ID)
			// Index by name.
			mtByName := make(map[string]*domain.NutritionistMealTypeFull)
			for i := range plan.MealTypes {
				mtByName[strings.ToLower(plan.MealTypes[i].Name)] = &plan.MealTypes[i]
			}
			for _, name := range slotNames {
				slotMealTypes = append(slotMealTypes, slotMealType{
					SlotName: name,
					MealType: mtByName[strings.ToLower(name)],
				})
			}
		}
	}

	perServing, err := a.Recipes.PerServingByRecipe(ctx, user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The coach's plan may use the coach's private recipes: without them
	// those items would count 0 kcal.
	if readOnly {
		coachRecipes, err := a.Recipes.PerServingByRecipe(ctx, assignment.CoachID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		for id, m := range coachRecipes {
			if _, ok := perServing[id]; !ok {
				perServing[id] = m
			}
		}
	}
	varKcal, itemKcal := planKcal(plan, perServing)
	data := pianoNutrizionaleData{
		authPageData:      a.baseData(r),
		Plan:              plan,
		Templates:         tpls,
		ActiveTab:         tab,
		SlotMealTypes:     slotMealTypes,
		HasActiveTemplate: hasActiveTemplate,
		ReadOnly:          readOnly,
		CoachName:         coachName,
		VariationKcal:     varKcal,
		ItemKcal:          itemKcal,
	}
	if hasActiveTemplate {
		data.ActiveTemplate = tpls[0].Name
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
		http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.NutritionistPlans.CreateMealType(r.Context(), plan.ID, name); err != nil {
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

func (a *App) handleNutritionistDeleteMealType(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.NutritionistPlans.DeleteMealType(r.Context(), id, plan.ID); err != nil {
		a.nutritionistWriteError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

func (a *App) handleNutritionistCreateVariation(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mealTypeIDStr := strings.TrimSpace(r.PostFormValue("meal_type_id"))
	nameStr := strings.TrimSpace(r.PostFormValue("name"))

	mealTypeID, err := strconv.ParseInt(mealTypeIDStr, 10, 64)
	if err != nil || mealTypeID <= 0 {
		a.Sessions.Put(r.Context(), "flash", "Tipo pasto non valido.")
		http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
		return
	}

	var namePtr *string
	if nameStr != "" {
		namePtr = &nameStr
	}

	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.NutritionistPlans.CreateVariation(r.Context(), plan.ID, mealTypeID, namePtr); err != nil {
		a.nutritionistWriteError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

func (a *App) handleNutritionistDeleteVariation(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.NutritionistPlans.DeleteVariation(r.Context(), id, plan.ID); err != nil {
		a.nutritionistWriteError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

// handleNutritionistPicker GET /nutrizionista/picker — live search for the
// inline picker in the Piano Pasti tab. Supports kind=ingredient|recipe.
func (a *App) handleNutritionistPicker(w http.ResponseWriter, r *http.Request) {
	variationIDStr := r.URL.Query().Get("variation_id")
	variationID, err := strconv.ParseInt(variationIDStr, 10, 64)
	if err != nil || variationID <= 0 {
		http.Error(w, "variation_id non valido", http.StatusBadRequest)
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "ingredient"
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	switch kind {
	case "ingredient":
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
			PickerKind:   "nutritionist_item",
			VariationID:  variationID,
			SearchedOFF:  includeOFFParam(r),
		}
		if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	case "recipe":
		data, err := a.recipePicker(r, q)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.VariationID = variationID
		if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "recipe_picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	default:
		http.Error(w, "kind non valido", http.StatusBadRequest)
	}
}

func (a *App) handleNutritionistAddItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	variationIDStr := strings.TrimSpace(r.PostFormValue("variation_id"))
	quantityStr := strings.TrimSpace(r.PostFormValue("quantity"))
	// Accepts both "unit" (old form) and "qty_unit" (new picker).
	unit := strings.TrimSpace(r.PostFormValue("qty_unit"))
	if unit == "" {
		unit = strings.TrimSpace(r.PostFormValue("unit"))
	}
	// Accepts both "ingredient_id" and "ref_id" (picker pattern).
	ingredientIDStr := strings.TrimSpace(r.PostFormValue("ingredient_id"))
	if ingredientIDStr == "" {
		ingredientIDStr = strings.TrimSpace(r.PostFormValue("ref_id"))
	}
	recipeIDStr := strings.TrimSpace(r.PostFormValue("recipe_id"))
	barcode := strings.TrimSpace(r.PostFormValue("barcode"))

	variationID, err := strconv.ParseInt(variationIDStr, 10, 64)
	if err != nil || variationID <= 0 {
		a.Sessions.Put(r.Context(), "flash", "Variante non valida.")
		http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
		return
	}

	quantityStr = strings.ReplaceAll(quantityStr, ",", ".")
	quantity, err := strconv.ParseFloat(quantityStr, 64)
	if err != nil || quantity <= 0 {
		a.Sessions.Put(r.Context(), "flash", "Quantita' non valida.")
		http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
		return
	}

	if unit == "" {
		unit = "g"
	}

	var ingIDPtr, recipeIDPtr *int64
	if barcode != "" {
		ingID, err := a.resolveOFFBarcode(r.Context(), barcode)
		if err != nil {
			a.Sessions.Put(r.Context(), "flash", "Import OFF fallito: "+err.Error())
			http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
			return
		}
		ingIDPtr = &ingID
	} else if ingredientIDStr != "" {
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
		http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
		return
	}

	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.NutritionistPlans.AddItem(r.Context(), plan.ID, variationID, recipeIDPtr, ingIDPtr, quantity, unit); err != nil {
		a.nutritionistWriteError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

func (a *App) handleNutritionistDeleteItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	plan, err := a.NutritionistPlans.GetOrCreateActivePlan(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.NutritionistPlans.DeleteItem(r.Context(), id, plan.ID); err != nil {
		a.nutritionistWriteError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}
