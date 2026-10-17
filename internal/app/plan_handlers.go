package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/profile"
)

// ---- view models -----------------------------------------------------------

type dashboardData struct {
	authPageData
	Plan                *domain.WeeklyPlanFull
	Days                []dayView     // legacy: used by tests, if any
	AllCards            []dayCardData // 7 days, in order: the cards at the top
	Selected            dayCardData   // the day open under the cards
	WeekStart           time.Time
	WeekStartStr        string // YYYY-MM-DD for the forms
	PrevWeek            string // YYYY-MM-DD
	NextWeek            string
	Week                weekSummary
	HasMeals            bool
	Templates           []*domain.MealTemplate // for the "apply template" picker
	HasNutritionistPlan bool                   // true when the user has variations to generate from
	ApplyError          string
	NewMealForm         newMealForm
	NewMealError        string
}

type dayView struct {
	Index    int       // 0..6
	Label    string    // "Lunedi'" etc.
	Date     time.Time // data effettiva
	Meals    []mealView
	DayTotal macros.Macros
}

// dayCardData is the view model passed to the day_pill.html partials (the
// day's card at the top) and day_card.html (the open day). Self-contained so
// it can be rendered both inline (dashboard) and alone (HTMX swap after a
// mutation or a card change).
//
// `State` describes the calendar day (past/today/future). `Selected` says
// whether it is the open day: only one per page.
type dayCardData struct {
	Day         dayView
	DateStr     string // YYYY-MM-DD for the data-day attribute
	State       string // "past" | "today" | "future"
	Selected    bool   // true = the day open in #day-panel
	OOB         bool   // true = the card travels as an out-of-band swap
	WeekStart   string // YYYY-MM-DD to hide/pass in the forms
	CSRFToken   string
	Progress    profile.MacroProgress
	WorkoutKcal float64                  // kcal burned by workouts that day
	Workouts    []domain.WorkoutLog      // the day's workouts
	Activities  []domain.WorkoutActivity // for the select in the add form
	Water       waterWidgetData          // the day's hydration widget
	Adherence   macros.AdherenceScore    // the day's plan vs actual adherence
}

// HasMeals says whether the day has at least one planned meal (to decide
// whether to show the adherence badge).
func (d dayCardData) HasMeals() bool { return len(d.Day.Meals) > 0 }

// waterWidgetData feeds the water_widget.html partial, renderable both inside
// the day card and alone (HTMX swap after a quick log).
type waterWidgetData struct {
	DateStr       string
	WaterML       int
	WaterTargetML int
	CSRFToken     string
}

// Pct returns the progress percentage (0-100, clamped) for the bar.
func (d waterWidgetData) Pct() int {
	if d.WaterTargetML <= 0 {
		return 0
	}
	p := d.WaterML * 100 / d.WaterTargetML
	if p > 100 {
		return 100
	}
	return p
}

// Reached says whether the target has been reached (to highlight the bar).
func (d waterWidgetData) Reached() bool {
	return d.WaterTargetML > 0 && d.WaterML >= d.WaterTargetML
}

type mealView struct {
	Meal       domain.MealWithItems
	MealTotal  macros.Macros // actual: cheats when present, otherwise items
	ItemViews  []mealItemView
	CheatViews []mealItemView // filled when HasCheats
	HasCheats  bool
}

// scaledIngredient is an ingredient of a recipe with its quantity scaled to
// the meal_item's number of servings. E.g. a 4-serving recipe with 200g pasta,
// in a meal_item with quantity=2 → scaled quantity = 100g.
type scaledIngredient struct {
	Name     string
	Quantity float64
	Unit     string
}

type mealItemView struct {
	Item   domain.MealItemFull
	Macros macros.Macros
	// ScaledRecipeItems: for a recipe item, the list of ingredients with
	// scaled quantity = recipeItem.Quantity * (item.Quantity /
	// recipe.Servings). Empty for plain ingredient items.
	ScaledRecipeItems []scaledIngredient
	// EffectiveGrams: the actual grams/ml when QuantityUnit=="pz"
	// (quantity * UnitWeightG). Precomputed because Go templates cannot
	// multiply two values.
	EffectiveGrams float64
}

// effectiveGramsFor returns the actual grams/ml of a meal item:
// quantity * unit_weight_g when QuantityUnit=="pz" and the ingredient has a
// known weight per unit, otherwise the raw quantity.
func effectiveGramsFor(it domain.MealItemFull) float64 {
	return macros.Grams(it.Quantity, it.QuantityUnit, it.Ingredient)
}

type newMealForm struct {
	DayOfWeek string
	Name      string
}

var dayLabelsFull = [7]string{
	"Lunedi'", "Martedi'", "Mercoledi'", "Giovedi'", "Venerdi'", "Sabato", "Domenica",
}

// ---- helpers ---------------------------------------------------------------

func formatDate(t time.Time) string { return t.Format("2006-01-02") }

// todayDate returns today's date normalized to local midnight.
func todayDate() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// dayState classifies a day as "past", "today" or "future" relative to
// today. It compares the calendar representation (YYYY-MM-DD) to be robust
// to timezone/DST/clock drift of the internal time.Time values.
func dayState(date, today time.Time) string {
	a, b := date.Format("2006-01-02"), today.Format("2006-01-02")
	switch {
	case a < b:
		return "past"
	case a == b:
		return "today"
	default:
		return "future"
	}
}

// buildDayViews fills a []dayView from the full plan, computing the macro
// totals per day and the items structure.
func buildDayViews(full *domain.WeeklyPlanFull, weekStart time.Time) []dayView {
	out := make([]dayView, 7)
	for d := 0; d < 7; d++ {
		dv := dayView{
			Index: d,
			Label: dayLabelsFull[d],
			Date:  weekStart.AddDate(0, 0, d),
		}
		for _, m := range full.Days[d] {
			mv := mealView{Meal: m, HasCheats: m.HasCheats()}
			// "Official" items always filled (the UI greys them out on a cheat).
			for _, it := range m.Items {
				macroIt := macros.ForMealItem(it)
				mv.ItemViews = append(mv.ItemViews, mealItemView{
					Item:              it,
					Macros:            macroIt,
					ScaledRecipeItems: scaleRecipeItems(it),
					EffectiveGrams:    effectiveGramsFor(it),
				})
			}
			// Cheat views filled when there are cheats.
			for _, ch := range m.Cheats {
				macroIt := macros.ForMealItem(ch)
				mv.CheatViews = append(mv.CheatViews, mealItemView{
					Item:              ch,
					Macros:            macroIt,
					ScaledRecipeItems: scaleRecipeItems(ch),
					EffectiveGrams:    effectiveGramsFor(ch),
				})
			}
			// Actual total: cheats replace items when present.
			mv.MealTotal = macros.ForMealEffective(m)
			dv.Meals = append(dv.Meals, mv)
			dv.DayTotal = dv.DayTotal.Add(mv.MealTotal)
		}
		out[d] = dv
	}
	return out
}

// scaleRecipeItems expands a recipe's ingredients, scaling the quantities to
// the requested number of servings. When the item has custom variations, it
// uses them instead of the base ingredients (quantities already absolute, no
// extra scaling). Returns nil when the item is not a recipe or has no
// ingredients loaded.
func scaleRecipeItems(it domain.MealItemFull) []scaledIngredient {
	if it.Recipe == nil {
		return nil
	}
	if it.IsCustomized() {
		out := make([]scaledIngredient, 0, len(it.Customizations))
		for _, c := range it.Customizations {
			if c.Ingredient == nil {
				continue
			}
			out = append(out, scaledIngredient{
				Name:     c.Ingredient.Name,
				Quantity: c.Quantity,
				Unit:     string(c.Ingredient.Unit),
			})
		}
		return out
	}
	if it.Recipe.Servings <= 0 || len(it.RecipeItems) == 0 {
		return nil
	}
	scale := it.Quantity / float64(it.Recipe.Servings)
	out := make([]scaledIngredient, 0, len(it.RecipeItems))
	for _, ri := range it.RecipeItems {
		out = append(out, scaledIngredient{
			Name:     ri.Ingredient.Name,
			Quantity: ri.Quantity * scale,
			Unit:     string(ri.Ingredient.Unit),
		})
	}
	return out
}

// adherenceForDay computes the daily adherence from the meals of a dayView
// (it extracts the domain.MealWithItems from the mealViews already built).
func adherenceForDay(dv dayView) macros.AdherenceScore {
	meals := make([]domain.MealWithItems, 0, len(dv.Meals))
	for _, mv := range dv.Meals {
		meals = append(meals, mv.Meal)
	}
	return macros.ScoreDay(meals)
}

// makeDayCard builds dayCardData from a dayView and the context.
// `state` is the calendar day; `selected` says whether it is the open day.
func makeDayCard(dv dayView, state string, selected bool, weekStartStr, csrfToken string, progress profile.MacroProgress, workoutKcal float64, workouts []domain.WorkoutLog, activities []domain.WorkoutActivity) dayCardData {
	return dayCardData{
		Day:         dv,
		DateStr:     formatDate(dv.Date),
		State:       state,
		Selected:    selected,
		WeekStart:   weekStartStr,
		CSRFToken:   csrfToken,
		Progress:    progress,
		WorkoutKcal: workoutKcal,
		Workouts:    workouts,
		Activities:  activities,
	}
}

// buildDayCards builds the 7 dayCardData; `selected` (YYYY-MM-DD) is the
// open day.
func buildDayCards(days []dayView, today time.Time, selected, weekStartStr, csrfToken string, user *domain.User, workoutKcals [7]float64, workoutLogs [7][]domain.WorkoutLog, activities []domain.WorkoutActivity, waterMLs [7]int) []dayCardData {
	out := make([]dayCardData, 0, 7)
	target := waterTargetFor(user)
	for i, dv := range days {
		state := dayState(dv.Date, today)
		var progress profile.MacroProgress
		if user != nil {
			progress = profile.BuildMacroProgress(dv.DayTotal, user.TargetKcal, user.TargetProtein, user.TargetCarbs, user.TargetFat)
		}
		card := makeDayCard(dv, state, formatDate(dv.Date) == selected, weekStartStr, csrfToken, progress, workoutKcals[i], workoutLogs[i], activities)
		card.Water = waterWidgetData{DateStr: card.DateStr, WaterML: waterMLs[i], WaterTargetML: target, CSRFToken: csrfToken}
		card.Adherence = adherenceForDay(dv)
		out = append(out, card)
	}
	return out
}

// waterTargetFor returns the user's hydration target, falling back to
// 2000 ml when unset (pre-migration users or nil).
func waterTargetFor(user *domain.User) int {
	if user != nil && user.WaterTargetML > 0 {
		return user.WaterTargetML
	}
	return 2000
}

// weekSummary is the week's summary in the dashboard header (#week-totals),
// updated out-of-band after every mutation.
type weekSummary struct {
	Total       macros.Macros
	PlannedDays int  // days with at least one meal
	OOB         bool // true = viaggia come swap out-of-band
}

// AvgKcal is the daily average over the planned days only.
func (s weekSummary) AvgKcal() float64 {
	if s.PlannedDays == 0 {
		return 0
	}
	return s.Total.Kcal / float64(s.PlannedDays)
}

func summarizeWeek(full *domain.WeeklyPlanFull) weekSummary {
	s := weekSummary{Total: macros.ForWeek(full.Days)}
	for _, meals := range full.Days {
		if len(meals) > 0 {
			s.PlannedDays++
		}
	}
	return s
}

// dayMutationResponse is the view model for day_card_with_pill_oob: card +
// the day's card OOB + the weekly summary OOB. It updates in a single
// response every element that depends on the macros after a mutation (add,
// remove, reorder) or a card change.
type dayMutationResponse struct {
	Card dayCardData
	Week weekSummary
}

// renderDayCard writes the day_card partial plus the matching card and the
// weekly summary with HX-OOB swaps, in a single response. The rendered day
// becomes the selected one.
func (a *App) renderDayCard(w http.ResponseWriter, r *http.Request, dv dayView, week weekSummary, weekStart time.Time, state string) {
	user := userFromContext(r.Context())
	var progress profile.MacroProgress
	if user != nil {
		progress = profile.BuildMacroProgress(dv.DayTotal, user.TargetKcal, user.TargetProtein, user.TargetCarbs, user.TargetFat)
	}
	// Load the workouts for the day (bug fix: it was always 0 after an HTMX mutation).
	var wkcal float64
	var wlogs []domain.WorkoutLog
	var acts []domain.WorkoutActivity
	if a.WorkoutLogs != nil && user != nil {
		wkcal, _ = a.WorkoutLogs.WorkoutKcalForDay(r.Context(), user.ID, dv.Date)
		wlogs, _ = a.WorkoutLogs.ListWorkoutLogs(r.Context(), user.ID, dv.Date, dv.Date)
		acts, _ = a.WorkoutLogs.ListActivities(r.Context())
	}
	card := makeDayCard(dv, state, true, formatDate(weekStart), nosurfTokenOrEmpty(r), progress, wkcal, wlogs, acts)
	card.Adherence = adherenceForDay(dv)
	card.OOB = true
	if a.WaterLogs != nil && user != nil {
		ml, _ := a.WaterLogs.TotalForDate(r.Context(), user.ID, dv.Date)
		card.Water = waterWidgetData{DateStr: card.DateStr, WaterML: ml, WaterTargetML: waterTargetFor(user), CSRFToken: card.CSRFToken}
	}
	week.OOB = true
	resp := dayMutationResponse{Card: card, Week: week}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "day_card_with_pill_oob", resp); err != nil {
		a.serverError(w, r, err)
	}
}

// loadDayCard loads the user's plan for the given week, builds the dayView of
// the requested day and also returns the weekly summary, so the header can be
// updated OOB.
func (a *App) loadDayCard(r *http.Request, userID int64, weekStart time.Time, dayIndex int) (*dayView, weekSummary, error) {
	if dayIndex < 0 || dayIndex > 6 {
		return nil, weekSummary{}, fmt.Errorf("day_index fuori range")
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), userID, weekStart)
	if err != nil {
		return nil, weekSummary{}, err
	}
	full, err := a.WeeklyPlans.LoadFull(r.Context(), plan)
	if err != nil {
		return nil, weekSummary{}, err
	}
	days := buildDayViews(full, weekStart)
	dv := days[dayIndex]
	return &dv, summarizeWeek(full), nil
}

// nosurfTokenOrEmpty is a thin wrapper to read the CSRF token; used by the
// partial renderers where there is no prebuilt baseData().
func nosurfTokenOrEmpty(r *http.Request) string {
	return nosurf.Token(r)
}

// respondMutationOrRedirect handles the dual mode (HTMX vs full navigation)
// uniformly after a mutation on a meal/item. For HX-Request it returns the
// full card + the OOB day card; otherwise it redirects to the week, on the
// day touched.
func (a *App) respondMutationOrRedirect(w http.ResponseWriter, r *http.Request, weekStart time.Time, dayIndex int) {
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToDay(w, r, weekStart, dayIndex)
		return
	}
	user := userFromContext(r.Context())
	dv, week, err := a.loadDayCard(r, user.ID, weekStart, dayIndex)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	state := dayState(dv.Date, todayDate())
	a.renderDayCard(w, r, *dv, week, weekStart, state)
}

// dayIndexFromMealID resolves a meal's day_of_week from its id.
// Needed to know which card to swap after remove/add-item, since the client
// does not always pass the index explicitly.
func (a *App) dayIndexFromMealID(r *http.Request, mealID int64) (int, error) {
	var day int
	err := a.Store.DB.QueryRowContext(r.Context(),
		`SELECT day_of_week FROM meals WHERE id = ?`, mealID).Scan(&day)
	return day, err
}

// dayIndexFromItemID resolves the day_of_week of the meal holding the item.
func (a *App) dayIndexFromItemID(r *http.Request, itemID int64) (int, error) {
	var day int
	err := a.Store.DB.QueryRowContext(r.Context(),
		`SELECT m.day_of_week FROM meals m
		 JOIN meal_items mi ON mi.meal_id = m.id
		 WHERE mi.id = ?`, itemID).Scan(&day)
	return day, err
}

// ---- handlers --------------------------------------------------------------

// handleDashboard handles GET / for logged-in users: it shows the current
// week or the one given by ?start=. For anonymous users it delegates to
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
	user := userFromContext(r.Context())
	base := a.baseData(r)
	days := buildDayViews(full, weekStart)
	hasMeals := false
	for _, dv := range days {
		if len(dv.Meals) > 0 {
			hasMeals = true
			break
		}
	}
	today := todayDate()
	weekStartStr := formatDate(weekStart)
	var workoutKcals [7]float64
	var workoutLogs [7][]domain.WorkoutLog
	var activities []domain.WorkoutActivity
	if a.WorkoutLogs != nil && user != nil {
		activities, _ = a.WorkoutLogs.ListActivities(r.Context())
		for i := 0; i < 7; i++ {
			d := weekStart.AddDate(0, 0, i)
			workoutKcals[i], _ = a.WorkoutLogs.WorkoutKcalForDay(r.Context(), user.ID, d)
			workoutLogs[i], _ = a.WorkoutLogs.ListWorkoutLogs(r.Context(), user.ID, d, d)
		}
	}
	// Hydration: a single query for the week, then spread over the 7 days.
	var waterMLs [7]int
	if a.WaterLogs != nil && user != nil {
		totals, _ := a.WaterLogs.ForDateRange(r.Context(), user.ID, weekStart, weekStart.AddDate(0, 0, 6))
		for i := 0; i < 7; i++ {
			waterMLs[i] = totals[formatDate(weekStart.AddDate(0, 0, i))]
		}
	}
	selected := selectedDay(r.URL.Query().Get("day"), weekStart, today)
	allCards := buildDayCards(days, today, formatDate(selected), weekStartStr, base.CSRFToken, user, workoutKcals, workoutLogs, activities, waterMLs)
	selIdx := 0
	for i, c := range allCards {
		if c.Selected {
			selIdx = i
		}
	}

	hasNutritionistPlan := false
	if a.NutritionistPlans != nil && user != nil {
		hasNutritionistPlan, _ = a.NutritionistPlans.HasVariations(r.Context(), user.ID)
	}

	data := dashboardData{
		authPageData:        base,
		Plan:                full,
		Days:                days,
		AllCards:            allCards,
		Selected:            allCards[selIdx],
		WeekStart:           weekStart,
		WeekStartStr:        weekStartStr,
		PrevWeek:            formatDate(weekStart.AddDate(0, 0, -7)),
		NextWeek:            formatDate(weekStart.AddDate(0, 0, 7)),
		Week:                summarizeWeek(full),
		HasMeals:            hasMeals,
		Templates:           tpls,
		HasNutritionistPlan: hasNutritionistPlan,
		ApplyError:          applyErr,
		NewMealForm:         mealForm,
		NewMealError:        mealErr,
	}
	if mealForm.DayOfWeek == "" {
		data.NewMealForm.DayOfWeek = "0"
	}
	status := http.StatusOK
	if applyErr != "" || mealErr != "" {
		status = http.StatusUnprocessableEntity
	}
	if err := a.Render.Render(w, r, status, "dashboard", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleApplyTemplate POST /week/{start}/apply-template — instantiates the meals.
func (a *App) handleApplyTemplate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r) // reads ?start from a form via redirect TOO
	if v := r.PostFormValue("start"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	tplIDStr := r.PostFormValue("template_id")
	tplID, err := strconv.ParseInt(tplIDStr, 10, 64)
	if err != nil || tplID <= 0 {
		a.flashAndBack(w, r, weekStart, "week.flash.select_template")
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
		a.flashAndBack(w, r, weekStart, "week.flash.has_meals_apply")
		return
	}
	if err := a.WeeklyPlans.ApplyTemplate(r.Context(), plan.ID, user.ID, tplID); err != nil {
		if errors.Is(err, db.ErrMealTemplateForbidden) || errors.Is(err, db.ErrMealTemplateNotFound) {
			a.flashAndBack(w, r, weekStart, "week.flash.template_invalid")
			return
		}
		a.serverError(w, r, err)
		return
	}
	// Historical freeze: record the nutrition plan in use when the template
	// was applied (best effort, does not block the flow).
	if effective, err := a.Planner.EffectivePlanFull(r.Context(), user.ID); err == nil {
		_ = a.WeeklyPlans.SetSourcePlan(r.Context(), plan.ID, effective.ID)
	}
	a.Sessions.Put(r.Context(), "flash", "week.flash.template_applied")
	a.redirectToWeek(w, r, weekStart)
}

// handleCopyPrevious POST /week/copy-previous?start=YYYY-MM-DD
// Clones meals + meal_items from the previous week's plan into the current
// plan. Refuses when the current week is not empty or the previous one has
// no meals.
func (a *App) handleCopyPrevious(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	if v := r.PostFormValue("start"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	prevWeekStart := weekStart.AddDate(0, 0, -7)
	dstPlan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	srcPlan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, prevWeekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.WeeklyPlans.CopyFrom(r.Context(), srcPlan.ID, dstPlan.ID, user.ID); err != nil {
		switch {
		case errors.Is(err, db.ErrSourcePlanEmpty):
			a.flashAndBack(w, r, weekStart, "week.flash.previous_empty")
			return
		case errors.Is(err, db.ErrPlanNotEmpty):
			a.flashAndBack(w, r, weekStart, "week.flash.has_meals_copy")
			return
		case errors.Is(err, db.ErrPlanNotFound):
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "week.flash.copied")
	a.redirectToWeek(w, r, weekStart)
}

// handleResetWeek POST /week/reset?start=YYYY-MM-DD — deletes every meal of
// the current week. Used to undo a copy or an apply-template done by
// mistake.
func (a *App) handleResetWeek(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	if v := r.PostFormValue("start"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.WeeklyPlans.ClearMeals(r.Context(), plan.ID, user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "week.flash.reset")
	a.redirectToWeek(w, r, weekStart)
}

func (a *App) flashAndBack(w http.ResponseWriter, r *http.Request, weekStart time.Time, msg string) {
	a.Sessions.Put(r.Context(), "flash", msg)
	// With HTMX a 303 would be followed inside the XHR and the whole week
	// would end up in the fragment to replace: reload the page instead.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/?start="+formatDate(weekStart))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	a.redirectToWeek(w, r, weekStart)
}

func (a *App) redirectToWeek(w http.ResponseWriter, r *http.Request, weekStart time.Time) {
	http.Redirect(w, r, "/?start="+formatDate(weekStart), http.StatusSeeOther)
}

// redirectToDay goes back to the week with day dayIndex open (0=Monday).
func (a *App) redirectToDay(w http.ResponseWriter, r *http.Request, weekStart time.Time, dayIndex int) {
	if dayIndex < 0 || dayIndex > 6 {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	http.Redirect(w, r, weekDayURL(weekStart, weekStart.AddDate(0, 0, dayIndex)), http.StatusSeeOther)
}

// weekDayURL is the address of the dashboard on week weekStart with day day
// open.
func weekDayURL(weekStart, day time.Time) string {
	return "/?start=" + formatDate(weekStart) + "&day=" + formatDate(day)
}

// selectedDay picks the day open in the dashboard: the one in ?day= when it
// falls in the week, otherwise today when it is in the week, otherwise
// Monday. It compares dates as YYYY-MM-DD strings (see dayState).
func selectedDay(param string, weekStart, today time.Time) time.Time {
	first, last := formatDate(weekStart), formatDate(weekStart.AddDate(0, 0, 6))
	in := func(s string) bool { return s >= first && s <= last }
	if d, err := time.Parse("2006-01-02", param); err == nil && in(param) {
		return d
	}
	if t := formatDate(today); in(t) {
		return today
	}
	return weekStart
}

// handleAddMeal POST /meals — adds a manual meal to the current plan.
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
		a.flashAndBack(w, r, weekStart, "week.flash.invalid_day")
		return
	}
	if name == "" {
		a.flashAndBack(w, r, weekStart, "week.flash.meal_name_required")
		return
	}
	if !withinLen(name, MaxNameLen) {
		a.flashAndBack(w, r, weekStart, "week.flash.name_too_long")
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
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleRemoveMeal POST /meals/{mealID}/delete?start=YYYY-MM-DD
func (a *App) handleRemoveMeal(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	day, dayErr := a.dayIndexFromMealID(r, mealID)
	if err := a.WeeklyPlans.RemoveMeal(r.Context(), mealID, user.ID); err != nil {
		if errors.Is(err, db.ErrMealNotFound) {
			http.Error(w, a.t(r, "week.error.meal_not_found"), http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleAddMealItem POST /meals/{mealID}/items — recipe XOR ingredient.
// For an ingredient it supports both `ref_id` (local, already in the DB) and
// `barcode` (OFF, idempotent auto-import via resolveOFFBarcode).
func (a *App) handleAddMealItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	kind := r.PostFormValue("kind") // "recipe" | "ingredient"
	qtyStr := r.PostFormValue("quantity")
	qty, err := parseNonNegFloat(qtyStr)
	if err != nil || qty <= 0 {
		a.flashAndBack(w, r, weekStart, "week.flash.invalid_quantity")
		return
	}

	var rPtr, iPtr *int64
	switch kind {
	case "recipe":
		idStr := r.PostFormValue("ref_id")
		refID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || refID <= 0 {
			a.flashAndBack(w, r, weekStart, "week.flash.select_recipe")
			return
		}
		rPtr = &refID
	case "ingredient":
		barcode := strings.TrimSpace(r.PostFormValue("barcode"))
		if barcode != "" {
			ingID, err := a.resolveOFFBarcode(r.Context(), barcode)
			if err != nil {
				a.flashAndBack(w, r, weekStart, a.t(r, "week.flash.off_import_failed", "Error", err.Error()))
				return
			}
			iPtr = &ingID
		} else {
			idStr := r.PostFormValue("ref_id")
			if idStr == "" {
				idStr = r.PostFormValue("ingredient_id")
			}
			refID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || refID <= 0 {
				a.flashAndBack(w, r, weekStart, "week.flash.select_ingredient")
				return
			}
			iPtr = &refID
		}
	default:
		a.flashAndBack(w, r, weekStart, "week.flash.invalid_type")
		return
	}

	// qty_unit: 'g', 'ml', or 'pz'; ignored for recipes.
	qtyUnit := strings.TrimSpace(r.PostFormValue("qty_unit"))
	if qtyUnit != "g" && qtyUnit != "ml" && qtyUnit != "pz" {
		qtyUnit = "g"
	}

	if _, err := a.WeeklyPlans.AddItem(r.Context(), mealID, user.ID, rPtr, iPtr, qty, qtyUnit); err != nil {
		a.flashAndBack(w, r, weekStart, a.t(r, "week.flash.item_add_failed", "Error", err.Error()))
		return
	}
	day, dayErr := a.dayIndexFromMealID(r, mealID)
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// splitSuggested keeps the nutrition plan's suggestions whose name contains
// q (all of them when q is empty) and removes them from the other rows, so in
// the picker every ingredient appears only once.
func splitSuggested(suggested []*domain.Ingredient, rows []ingredientRow, q string) ([]ingredientRow, []ingredientRow) {
	ql := strings.ToLower(q)
	var out []ingredientRow
	shown := make(map[int64]bool, len(suggested))
	for _, ing := range suggested {
		if strings.Contains(strings.ToLower(ing.Name), ql) {
			out = append(out, ingredientRow{Local: ing})
			shown[ing.ID] = true
		}
	}
	rest := make([]ingredientRow, 0, len(rows))
	for _, row := range rows {
		if row.Local == nil || !shown[row.Local.ID] {
			rest = append(rest, row)
		}
	}
	return out, rest
}

// handleMealPicker GET /meals/{mealID}/picker?kind=ingredient|recipe&q=...&start=
// Renders the picker partial (live HTMX) with the filtered rows. For
// kind=ingredient it uses searchMixed (DB + OFF merged). For kind=recipe it
// filters the recipes visible to the user by name.
func (a *App) handleMealPicker(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "ingredient"
	}
	asCheat := r.URL.Query().Get("as") == "cheat"
	// dialog=1: the picker is in the dialog over the week, so the forms
	// update the open day via HTMX instead of reloading the page.
	inDialog := r.URL.Query().Get("dialog") == "1"
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	weekStart := parseWeekStart(r)
	weekStartStr := formatDate(weekStart)

	switch kind {
	case "ingredient":
		// The OFF search is now local via FTS5: a single fast synchronous call
		// (<50ms) returns local + OFF merged.
		rows, warn, err := a.searchMixed(r.Context(), q, includeOFFParam(r))
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		pkind := "meal_item"
		if asCheat {
			pkind = "meal_cheat"
		}

		// Suggestions from the nutrition plan: active only when meal_name is
		// given and the picker is not in cheat mode (cheats have no suggestions).
		var nutrRows []ingredientRow
		if mealName := strings.TrimSpace(r.URL.Query().Get("meal_name")); mealName != "" && !asCheat {
			suggested, _ := a.NutritionistPlans.SearchSuggestedIngredients(r.Context(), user.ID, mealName)
			nutrRows, rows = splitSuggested(suggested, rows, q)
		}

		data := ingredientsListData{
			authPageData:      authPageData{CSRFToken: nosurf.Token(r)},
			Rows:              rows,
			Query:             q,
			OFFWarning:        warn,
			PickerKind:        pkind,
			MealID:            mealID,
			WeekStart:         weekStartStr,
			NutritionistItems: nutrRows,
			InDialog:          inDialog,
			SearchedOFF:       includeOFFParam(r),
		}
		if err := a.Render.RenderPartial(w, r, http.StatusOK, "ingredients_list", "picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	case "recipe":
		data, err := a.recipePicker(r, q)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.MealID, data.WeekStart, data.Cheat, data.InDialog = mealID, weekStartStr, asCheat, inDialog
		if err := a.Render.RenderPartial(w, r, http.StatusOK, "ingredients_list", "recipe_picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	default:
		http.Error(w, a.t(r, "week.error.invalid_kind"), http.StatusBadRequest)
	}
}

// handleRemoveMealItem POST /meal-items/{itemID}/delete?start=YYYY-MM-DD
func (a *App) handleRemoveMealItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	day, dayErr := a.dayIndexFromItemID(r, itemID)
	if err := a.WeeklyPlans.RemoveItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrMealItemNotFound) {
			http.Error(w, a.t(r, "week.error.item_not_found"), http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleReorderMeals POST /meals/reorder?day_of_week=N&start=YYYY-MM-DD
// Body: ids=12,9,15  (form-encoded). Persists the order of the meal_ids for
// that day and returns the updated day_card partial.
func (a *App) handleReorderMeals(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	dayStr := r.URL.Query().Get("day_of_week")
	day, err := strconv.Atoi(dayStr)
	if err != nil || day < 0 || day > 6 {
		http.Error(w, a.t(r, "week.error.missing_day"), http.StatusBadRequest)
		return
	}
	ids, err := parseIDList(r.PostFormValue("ids"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.WeeklyPlans.ReorderMeals(r.Context(), plan.ID, user.ID, day, ids); err != nil {
		if errors.Is(err, db.ErrReorderMismatch) {
			http.Error(w, a.t(r, "week.error.invalid_order"), http.StatusConflict)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleReorderMealItems POST /meals/{mealID}/items/reorder?start=YYYY-MM-DD
func (a *App) handleReorderMealItems(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	ids, err := parseIDList(r.PostFormValue("ids"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.WeeklyPlans.ReorderMealItems(r.Context(), mealID, user.ID, ids); err != nil {
		if errors.Is(err, db.ErrReorderMismatch) {
			http.Error(w, a.t(r, "week.error.invalid_order"), http.StatusConflict)
			return
		}
		if errors.Is(err, db.ErrMealNotFound) || errors.Is(err, db.ErrPlanNotFound) {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	day, dayErr := a.dayIndexFromMealID(r, mealID)
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleDayCard GET /days/{date} returns the full card (+ the OOB day card)
// for the given day. Called by the day cards (hx-get) when the user opens
// another day of the week.
func (a *App) handleDayCard(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	dateStr := chi.URLParam(r, "date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := db.MondayOf(date)
	dayIndex := int(date.Sub(weekStart).Hours() / 24)
	if dayIndex < 0 || dayIndex > 6 {
		a.notFound(w, r)
		return
	}
	dv, week, err := a.loadDayCard(r, user.ID, weekStart, dayIndex)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	state := dayState(dv.Date, todayDate())
	a.renderDayCard(w, r, *dv, week, weekStart, state)
}

// handleAddItemForm GET /meals/{mealID}/items/new — shell of the live HTMX
// picker. The lists load via /meals/{id}/picker, nothing is preloaded.
// Called by HTMX (the week's link) it answers with just the <dialog>, which
// modal.js opens over the week; without JS it is a page.
type addItemFormData struct {
	authPageData
	MealID    int64
	MealName  string
	Date      time.Time // the meal's day, for the subtitle
	WeekStart string
	Cheat     bool // true = add as a cheat (POSTs to /meals/{id}/cheat/items)
	InDialog  bool // true = additions update the day via HTMX
	Error     string
}

func (a *App) handleAddItemForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	mealName, date, err := a.WeeklyPlans.MealInfo(r.Context(), mealID, user.ID)
	if errors.Is(err, db.ErrMealNotFound) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := addItemFormData{
		authPageData: a.baseData(r),
		MealID:       mealID,
		MealName:     mealName,
		Date:         date,
		WeekStart:    formatDate(db.MondayOf(date)),
		Cheat:        r.URL.Query().Get("cheat") == "1",
		InDialog:     r.Header.Get("HX-Request") == "true",
	}
	if data.InDialog {
		err = a.Render.RenderPartial(w, r, http.StatusOK, "meal_item_new", "meal_picker_dialog", data)
	} else {
		err = a.Render.Render(w, r, http.StatusOK, "meal_item_new", data)
	}
	if err != nil {
		a.serverError(w, r, err)
	}
}
