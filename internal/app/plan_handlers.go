package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// ---- view models -----------------------------------------------------------

type dashboardData struct {
	authPageData
	Plan         *domain.WeeklyPlanFull
	Days         []dayView
	WeekStart    time.Time
	PrevWeek     string // YYYY-MM-DD
	NextWeek     string
	WeekTotal    macros.Macros
	HasMeals     bool
	Templates    []*domain.MealTemplate // per il picker "applica template"
	ApplyError   string
	NewMealForm  newMealForm
	NewMealError string
}

type dayView struct {
	Index    int       // 0..6
	Label    string    // "Lunedi'" etc.
	Date     time.Time // data effettiva
	Meals    []mealView
	DayTotal macros.Macros
}

type mealView struct {
	Meal       domain.MealWithItems
	MealTotal  macros.Macros
	ItemViews  []mealItemView
}

type mealItemView struct {
	Item   domain.MealItemFull
	Macros macros.Macros
}

type newMealForm struct {
	DayOfWeek string
	Name      string
}

var dayLabelsFull = [7]string{
	"Lunedi'", "Martedi'", "Mercoledi'", "Giovedi'", "Venerdi'", "Sabato", "Domenica",
}

// ---- helpers ---------------------------------------------------------------

// parseWeekStart legge ?start=YYYY-MM-DD e normalizza a lunedi'. Se mancante
// o invalida, ritorna il lunedi' della settimana corrente.
func parseWeekStart(r *http.Request) time.Time {
	q := strings.TrimSpace(r.URL.Query().Get("start"))
	if q != "" {
		if t, err := time.Parse("2006-01-02", q); err == nil {
			return db.MondayOf(t)
		}
	}
	return db.MondayOf(time.Now())
}

func formatDate(t time.Time) string { return t.Format("2006-01-02") }

// ---- handlers --------------------------------------------------------------

// handleDashboard gestisce GET / per utenti loggati: mostra la settimana
// corrente o quella indicata da ?start=. Per utenti non loggati delega a
// handleHome (landing page).
func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		a.handleHome(w, r)
		return
	}
	weekStart := parseWeekStart(r)

	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	full, err := a.WeeklyPlans.LoadFull(r.Context(), plan)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	tpls, err := a.MealTemplates.ListByUser(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	a.renderDashboard(w, r, full, weekStart, tpls, "", newMealForm{}, "")
}

func (a *App) renderDashboard(
	w http.ResponseWriter, r *http.Request,
	full *domain.WeeklyPlanFull,
	weekStart time.Time,
	tpls []*domain.MealTemplate,
	applyErr string,
	mealForm newMealForm, mealErr string,
) {
	days := make([]dayView, 7)
	hasMeals := false
	for d := 0; d < 7; d++ {
		dv := dayView{
			Index: d,
			Label: dayLabelsFull[d],
			Date:  weekStart.AddDate(0, 0, d),
		}
		for _, m := range full.Days[d] {
			hasMeals = true
			mv := mealView{Meal: m}
			for _, it := range m.Items {
				macroIt := macros.ForMealItem(it)
				mv.ItemViews = append(mv.ItemViews, mealItemView{Item: it, Macros: macroIt})
				mv.MealTotal = mv.MealTotal.Add(macroIt)
			}
			dv.Meals = append(dv.Meals, mv)
			dv.DayTotal = dv.DayTotal.Add(mv.MealTotal)
		}
		days[d] = dv
	}
	weekTotal := macros.ForWeek(full.Days)

	data := dashboardData{
		authPageData: a.baseData(r),
		Plan:         full,
		Days:         days,
		WeekStart:    weekStart,
		PrevWeek:     formatDate(weekStart.AddDate(0, 0, -7)),
		NextWeek:     formatDate(weekStart.AddDate(0, 0, 7)),
		WeekTotal:    weekTotal,
		HasMeals:     hasMeals,
		Templates:    tpls,
		ApplyError:   applyErr,
		NewMealForm:  mealForm,
		NewMealError: mealErr,
	}
	if mealForm.DayOfWeek == "" {
		data.NewMealForm.DayOfWeek = "0"
	}
	status := http.StatusOK
	if applyErr != "" || mealErr != "" {
		status = http.StatusUnprocessableEntity
	}
	if err := a.Render.Render(w, status, "dashboard", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleApplyTemplate POST /week/{start}/apply-template — istanzia i meals.
func (a *App) handleApplyTemplate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r) // legge ?start ANCHE da form via redirect
	if v := r.PostFormValue("start"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	tplIDStr := r.PostFormValue("template_id")
	tplID, err := strconv.ParseInt(tplIDStr, 10, 64)
	if err != nil || tplID <= 0 {
		a.flashAndBack(w, r, weekStart, "Seleziona un template.")
		return
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	hasMeals, err := a.WeeklyPlans.HasMeals(r.Context(), plan.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if hasMeals {
		a.flashAndBack(w, r, weekStart, "La settimana ha gia' dei pasti: cancellali prima di applicare un template.")
		return
	}
	if err := a.WeeklyPlans.ApplyTemplate(r.Context(), plan.ID, user.ID, tplID); err != nil {
		if errors.Is(err, db.ErrMealTemplateForbidden) || errors.Is(err, db.ErrMealTemplateNotFound) {
			a.flashAndBack(w, r, weekStart, "Template non valido.")
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Template applicato alla settimana.")
	a.redirectToWeek(w, r, weekStart)
}

func (a *App) flashAndBack(w http.ResponseWriter, r *http.Request, weekStart time.Time, msg string) {
	a.Sessions.Put(r.Context(), "flash", msg)
	a.redirectToWeek(w, r, weekStart)
}

func (a *App) redirectToWeek(w http.ResponseWriter, r *http.Request, weekStart time.Time) {
	http.Redirect(w, r, "/?start="+formatDate(weekStart), http.StatusSeeOther)
}

// handleAddMeal POST /meals — aggiunge un pasto manuale al piano corrente.
func (a *App) handleAddMeal(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	if v := r.PostFormValue("start"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	dayStr := strings.TrimSpace(r.PostFormValue("day_of_week"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	day, parseErr := strconv.Atoi(dayStr)
	if parseErr != nil || day < 0 || day > 6 {
		a.flashAndBack(w, r, weekStart, "Giorno non valido.")
		return
	}
	if name == "" {
		a.flashAndBack(w, r, weekStart, "Nome pasto obbligatorio.")
		return
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.WeeklyPlans.AddMeal(r.Context(), plan.ID, user.ID, day, name); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectToWeek(w, r, weekStart)
}

// handleRemoveMeal POST /meals/{mealID}/delete?start=YYYY-MM-DD
func (a *App) handleRemoveMeal(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.WeeklyPlans.RemoveMeal(r.Context(), mealID, user.ID); err != nil {
		if errors.Is(err, db.ErrMealNotFound) {
			http.Error(w, "pasto non trovato", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.redirectToWeek(w, r, weekStart)
}

// handleAddMealItem POST /meals/{mealID}/items — recipe XOR ingredient.
func (a *App) handleAddMealItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	kind := r.PostFormValue("kind") // "recipe" | "ingredient"
	idStr := r.PostFormValue("ref_id")
	qtyStr := r.PostFormValue("quantity")
	refID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || refID <= 0 {
		a.flashAndBack(w, r, weekStart, "Seleziona una ricetta o un ingrediente.")
		return
	}
	qty, err := parseNonNegFloat(qtyStr)
	if err != nil || qty <= 0 {
		a.flashAndBack(w, r, weekStart, "Quantita' non valida.")
		return
	}
	var rPtr, iPtr *int64
	switch kind {
	case "recipe":
		rPtr = &refID
	case "ingredient":
		iPtr = &refID
	default:
		a.flashAndBack(w, r, weekStart, "Tipo non valido.")
		return
	}
	if _, err := a.WeeklyPlans.AddItem(r.Context(), mealID, user.ID, rPtr, iPtr, qty); err != nil {
		a.flashAndBack(w, r, weekStart, "Impossibile aggiungere la voce: "+err.Error())
		return
	}
	a.redirectToWeek(w, r, weekStart)
}

// handleRemoveMealItem POST /meal-items/{itemID}/delete?start=YYYY-MM-DD
func (a *App) handleRemoveMealItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	itemID, err := parseID(r, "itemID")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.WeeklyPlans.RemoveItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrMealItemNotFound) {
			http.Error(w, "voce non trovata", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.redirectToWeek(w, r, weekStart)
}

// handleAddItemForm GET /meals/{mealID}/items/new — form picker per aggiungere
// ricetta o ingrediente al pasto.
type addItemFormData struct {
	authPageData
	MealID      int64
	WeekStart   string
	Recipes     []*domain.Recipe
	Ingredients []*domain.Ingredient
	Error       string
}

func (a *App) handleAddItemForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mealID, err := parseID(r, "mealID")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	recipes, err := a.Recipes.ListVisible(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	ings, err := a.Ingredients.ListActive(r.Context(), 0, 200)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := addItemFormData{
		authPageData: a.baseData(r),
		MealID:       mealID,
		WeekStart:    formatDate(weekStart),
		Recipes:      recipes,
		Ingredients:  ings,
	}
	if err := a.Render.Render(w, http.StatusOK, "meal_item_new", data); err != nil {
		a.serverError(w, r, err)
	}
}
