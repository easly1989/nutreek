package app

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/domain"
)

func (a *App) handleNutritionistPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusFound)
}

// slotMealType abbina uno slot name del template al corrispondente meal type del piano.
type slotMealType struct {
	SlotName string
	MealType *domain.NutritionistMealTypeFull
}

// pianoNutrizionaleData è il view model della pagina unificata /piano-nutrizionale.
type pianoNutrizionaleData struct {
	authPageData
	Plan              *domain.NutritionistPlanFull
	Templates         []*domain.MealTemplate
	ActiveTab         string // "piano" | "template"
	SlotMealTypes     []slotMealType
	HasActiveTemplate bool
	ReadOnly          bool   // true se il piano mostrato e' assegnato dal coach
	CoachName         string // nome del coach che ha assegnato il piano
}

func (a *App) handlePianoNutrizionale(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	// Se il coach ha assegnato un piano al cliente, quello e' il piano
	// effettivo: viene mostrato in sola lettura.
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

	// Costruisce la lista slot→meal-type dal template attivo (solo per il
	// proprio piano: quello assegnato dal coach non va modificato).
	var slotMealTypes []slotMealType
	var hasActiveTemplate bool
	if !readOnly && len(tpls) > 0 && tpls[0].IsActive {
		hasActiveTemplate = true
		activeWithSlots, err := a.MealTemplates.GetWithSlots(ctx, tpls[0].ID, user.ID)
		if err == nil {
			// Unique slot names in ordine di prima apparizione.
			seen := make(map[string]bool)
			var slotNames []string
			for _, s := range activeWithSlots.Slots {
				key := strings.ToLower(s.Name)
				if !seen[key] {
					seen[key] = true
					slotNames = append(slotNames, s.Name)
				}
			}
			// Auto-crea i meal type mancanti.
			for _, name := range slotNames {
				_ = a.NutritionistPlans.EnsureMealType(ctx, plan.ID, name)
			}
			// Ricarica dopo gli upsert.
			plan, _ = a.NutritionistPlans.LoadPlanFull(ctx, user.ID)
			// Indice per nome.
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

	data := pianoNutrizionaleData{
		authPageData:      a.baseData(r),
		Plan:              plan,
		Templates:         tpls,
		ActiveTab:         tab,
		SlotMealTypes:     slotMealTypes,
		HasActiveTemplate: hasActiveTemplate,
		ReadOnly:          readOnly,
		CoachName:         coachName,
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
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

func (a *App) handleNutritionistCreateVariation(w http.ResponseWriter, r *http.Request) {
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

	if _, err := a.NutritionistPlans.CreateVariation(r.Context(), mealTypeID, namePtr); err != nil {
		a.serverError(w, r, err)
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
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

// handleNutritionistPicker GET /nutrizionista/picker — live search per il picker
// inline nel tab Piano Pasti. Supporta kind=ingredient|recipe.
func (a *App) handleNutritionistPicker(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
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
		}
		if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	case "recipe":
		recipes, err := a.Recipes.ListVisible(r.Context(), user.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		if q != "" {
			ql := strings.ToLower(q)
			filtered := make([]*domain.Recipe, 0, len(recipes))
			for _, rc := range recipes {
				if strings.Contains(strings.ToLower(rc.Name), ql) {
					filtered = append(filtered, rc)
				}
			}
			recipes = filtered
		}
		data := struct {
			authPageData
			Recipes     []*domain.Recipe
			Query       string
			VariationID int64
			Cheat       bool
			MealID      int64
			WeekStart   string
		}{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			Recipes:      recipes,
			Query:        q,
			VariationID:  variationID,
		}
		if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "recipe_picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	default:
		http.Error(w, "kind non valido", http.StatusBadRequest)
	}
}

func (a *App) handleNutritionistAddItem(w http.ResponseWriter, r *http.Request) {
	variationIDStr := strings.TrimSpace(r.PostFormValue("variation_id"))
	quantityStr := strings.TrimSpace(r.PostFormValue("quantity"))
	// Accetta sia "unit" (vecchio form) sia "qty_unit" (nuovo picker).
	unit := strings.TrimSpace(r.PostFormValue("qty_unit"))
	if unit == "" {
		unit = strings.TrimSpace(r.PostFormValue("unit"))
	}
	// Accetta sia "ingredient_id" sia "ref_id" (pattern picker).
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

	if _, err := a.NutritionistPlans.AddItem(r.Context(), variationID, recipeIDPtr, ingIDPtr, quantity, unit); err != nil {
		a.serverError(w, r, err)
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
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}
