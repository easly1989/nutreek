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
	Days                []dayView     // legacy: usato nei test eventuali
	AllCards            []dayCardData // 7 giorni, in ordine: le schede in alto
	Selected            dayCardData   // il giorno aperto sotto le schede
	WeekStart           time.Time
	WeekStartStr        string // YYYY-MM-DD per i form
	PrevWeek            string // YYYY-MM-DD
	NextWeek            string
	Week                weekSummary
	HasMeals            bool
	Templates           []*domain.MealTemplate // per il picker "applica template"
	HasNutritionistPlan bool                   // true se l'utente ha varianti da cui generare
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

// dayCardData e' il view-model passato ai partial day_pill.html (scheda del
// giorno in alto) e day_card.html (il giorno aperto). Self-contained per
// essere renderizzabile sia inline (dashboard) sia da solo (HTMX swap dopo
// mutation o cambio di scheda).
//
// `State` descrive il giorno calendariale (past/today/future). `Selected`
// dice se e' il giorno aperto: uno solo per pagina.
type dayCardData struct {
	Day         dayView
	DateStr     string // YYYY-MM-DD per data-day attribute
	State       string // "past" | "today" | "future"
	Selected    bool   // true = giorno aperto in #day-panel
	OOB         bool   // true = la scheda viaggia come swap out-of-band
	WeekStart   string // YYYY-MM-DD per nascondere/passare nei form
	CSRFToken   string
	Progress    profile.MacroProgress
	WorkoutKcal float64                  // kcal bruciate con allenamenti in quel giorno
	Workouts    []domain.WorkoutLog      // allenamenti del giorno
	Activities  []domain.WorkoutActivity // per il select nel form di aggiunta
	Water       waterWidgetData          // widget idratazione del giorno
	Adherence   macros.AdherenceScore    // aderenza piano vs effettivo del giorno
}

// HasMeals indica se il giorno ha almeno un pasto pianificato (per decidere
// se mostrare il badge aderenza).
func (d dayCardData) HasMeals() bool { return len(d.Day.Meals) > 0 }

// waterWidgetData alimenta il partial water_widget.html, renderizzabile sia
// dentro la day card sia da solo (HTMX swap dopo quick-log).
type waterWidgetData struct {
	DateStr       string
	WaterML       int
	WaterTargetML int
	CSRFToken     string
}

// Pct ritorna la percentuale di avanzamento (0-100, clamp) per la barra.
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

// Reached indica se il target e' stato raggiunto (per evidenziare la barra).
func (d waterWidgetData) Reached() bool {
	return d.WaterTargetML > 0 && d.WaterML >= d.WaterTargetML
}

type mealView struct {
	Meal       domain.MealWithItems
	MealTotal  macros.Macros // effettivo: cheats se presenti, altrimenti items
	ItemViews  []mealItemView
	CheatViews []mealItemView // popolato se HasCheats
	HasCheats  bool
}

// scaledIngredient e' un ingrediente di una ricetta con quantita' scalata
// rispetto al numero di porzioni del meal_item. Es. ricetta da 4 porzioni
// con 200g pasta, in un meal_item con quantity=2 → quantity scalata = 100g.
type scaledIngredient struct {
	Name     string
	Quantity float64
	Unit     string
}

type mealItemView struct {
	Item   domain.MealItemFull
	Macros macros.Macros
	// ScaledRecipeItems: per item ricetta, lista degli ingredienti con
	// quantita' scalata = recipeItem.Quantity * (item.Quantity /
	// recipe.Servings). Vuoto per gli item ingrediente puro.
	ScaledRecipeItems []scaledIngredient
	// EffectiveGrams: grammi/ml effettivi quando QuantityUnit=="pz"
	// (quantity * UnitWeightG). Pre-calcolato perche' i template Go
	// non possono moltiplicare due valori.
	EffectiveGrams float64
}

// effectiveGramsFor restituisce i grammi/ml effettivi di un meal item:
// quantity * unit_weight_g se QuantityUnit=="pz" e l'ingrediente ha un
// peso per unita' noto, altrimenti la quantity grezza.
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

// todayDate ritorna la data odierna normalizzata a mezzanotte locale.
func todayDate() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// dayState classifica un giorno come "past", "today" o "future" rispetto a
// today. Usa il confronto sulla rappresentazione calendariale (YYYY-MM-DD)
// per essere robusto a timezone/DST/clock_drift dei time.Time interni.
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

// buildDayViews popola un []dayView a partire dal piano completo, calcolando
// i totali macro per giorno e la struttura items.
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
			// Items "ufficiali" sempre popolati (UI mostra greyed se sgarro).
			for _, it := range m.Items {
				macroIt := macros.ForMealItem(it)
				mv.ItemViews = append(mv.ItemViews, mealItemView{
					Item:              it,
					Macros:            macroIt,
					ScaledRecipeItems: scaleRecipeItems(it),
					EffectiveGrams:    effectiveGramsFor(it),
				})
			}
			// Cheat views popolati se ci sono sgarri.
			for _, ch := range m.Cheats {
				macroIt := macros.ForMealItem(ch)
				mv.CheatViews = append(mv.CheatViews, mealItemView{
					Item:              ch,
					Macros:            macroIt,
					ScaledRecipeItems: scaleRecipeItems(ch),
					EffectiveGrams:    effectiveGramsFor(ch),
				})
			}
			// Totale effettivo: cheats sostituiscono items se presenti.
			mv.MealTotal = macros.ForMealEffective(m)
			dv.Meals = append(dv.Meals, mv)
			dv.DayTotal = dv.DayTotal.Add(mv.MealTotal)
		}
		out[d] = dv
	}
	return out
}

// scaleRecipeItems espande gli ingredienti di una ricetta scalando le
// quantita' per il numero di porzioni richieste. Se l'item ha variazioni
// personalizzate, le usa al posto degli ingredienti base (quantita' gia'
// assolute, nessuno scaling aggiuntivo). Ritorna nil se l'item non e'
// una ricetta o se non ha ingredienti caricati.
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

// adherenceForDay calcola l'aderenza giornaliera a partire dai pasti di una
// dayView (estrae i domain.MealWithItems dalle mealView gia' costruite).
func adherenceForDay(dv dayView) macros.AdherenceScore {
	meals := make([]domain.MealWithItems, 0, len(dv.Meals))
	for _, mv := range dv.Meals {
		meals = append(meals, mv.Meal)
	}
	return macros.ScoreDay(meals)
}

// makeDayCard costruisce dayCardData a partire da una dayView e dal contesto.
// `state` e' il giorno calendariale; `selected` dice se e' il giorno aperto.
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

// buildDayCards costruisce le 7 dayCardData; `selected` (YYYY-MM-DD) e' il
// giorno aperto.
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

// waterTargetFor ritorna il target idratazione dell'utente, con fallback a
// 2000 ml se non valorizzato (utenti pre-migrazione o nil).
func waterTargetFor(user *domain.User) int {
	if user != nil && user.WaterTargetML > 0 {
		return user.WaterTargetML
	}
	return 2000
}

// weekSummary e' il riepilogo della settimana nell'intestazione della
// dashboard (#week-totals), aggiornato out-of-band dopo ogni mutazione.
type weekSummary struct {
	Total       macros.Macros
	PlannedDays int  // giorni con almeno un pasto
	OOB         bool // true = viaggia come swap out-of-band
}

// AvgKcal e' la media giornaliera sui soli giorni pianificati.
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

// dayMutationResponse e' il view-model per day_card_with_pill_oob: card +
// scheda del giorno OOB + riepilogo settimanale OOB. Serve per aggiornare in
// unica risposta tutti gli elementi che dipendono dai macro dopo una
// mutazione (add, remove, reorder) o un cambio di scheda.
type dayMutationResponse struct {
	Card dayCardData
	Week weekSummary
}

// renderDayCard scrive il partial day_card piu' la scheda corrispondente e
// il riepilogo settimanale con HX-OOB swap, in un unico response. Il giorno
// renderizzato diventa quello selezionato.
func (a *App) renderDayCard(w http.ResponseWriter, r *http.Request, dv dayView, week weekSummary, weekStart time.Time, state string) {
	user := userFromContext(r.Context())
	var progress profile.MacroProgress
	if user != nil {
		progress = profile.BuildMacroProgress(dv.DayTotal, user.TargetKcal, user.TargetProtein, user.TargetCarbs, user.TargetFat)
	}
	// Carica workout per il giorno (fix bug: prima era sempre 0 dopo mutation HTMX).
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
	if err := a.Render.RenderPartial(w, http.StatusOK, "dashboard", "day_card_with_pill_oob", resp); err != nil {
		a.serverError(w, r, err)
	}
}

// loadDayCard carica il piano dell'utente per la settimana data, costruisce
// la dayView del giorno richiesto e ritorna anche il riepilogo settimanale
// per permettere l'aggiornamento OOB dell'intestazione.
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

// nosurfTokenOrEmpty e' un thin wrapper per leggere il CSRF token; usato dai
// renderer di partial dove non c'e' un baseData() pre-costruito.
func nosurfTokenOrEmpty(r *http.Request) string {
	return nosurf.Token(r)
}

// respondMutationOrRedirect gestisce in modo uniforme il dual-mode (HTMX
// vs full navigation) dopo una mutazione su un meal/item. Per HX-Request
// ritorna la card piena + la scheda OOB; altrimenti redirect alla settimana,
// sul giorno toccato.
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

// dayIndexFromMealID risolve il day_of_week di un meal a partire dal suo id.
// Necessario per sapere quale card swappare dopo remove/add-item, dato che
// il client non passa sempre l'index esplicito.
func (a *App) dayIndexFromMealID(r *http.Request, mealID int64) (int, error) {
	var day int
	err := a.Store.DB.QueryRowContext(r.Context(),
		`SELECT day_of_week FROM meals WHERE id = ?`, mealID).Scan(&day)
	return day, err
}

// dayIndexFromItemID risolve il day_of_week del meal che contiene l'item.
func (a *App) dayIndexFromItemID(r *http.Request, itemID int64) (int, error) {
	var day int
	err := a.Store.DB.QueryRowContext(r.Context(),
		`SELECT m.day_of_week FROM meals m
		 JOIN meal_items mi ON mi.meal_id = m.id
		 WHERE mi.id = ?`, itemID).Scan(&day)
	return day, err
}

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
	// Idratazione: una sola query per la settimana, poi distribuita sui 7 giorni.
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
	// Freeze storico: registra il piano nutrizionista in uso al momento
	// dell'applicazione (best-effort, non blocca il flusso).
	if effective, err := a.Planner.EffectivePlanFull(r.Context(), user.ID); err == nil {
		_ = a.WeeklyPlans.SetSourcePlan(r.Context(), plan.ID, effective.ID)
	}
	a.Sessions.Put(r.Context(), "flash", "Template applicato alla settimana.")
	a.redirectToWeek(w, r, weekStart)
}

// handleCopyPrevious POST /week/copy-previous?start=YYYY-MM-DD
// Clona meals + meal_items dal piano della settimana precedente al piano
// corrente. Refuse se la settimana corrente non e' vuota o se la
// precedente non ha meals.
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
			a.flashAndBack(w, r, weekStart, "La settimana scorsa e' vuota: niente da copiare.")
			return
		case errors.Is(err, db.ErrPlanNotEmpty):
			a.flashAndBack(w, r, weekStart, "La settimana ha gia' dei pasti: cancellali prima di copiare.")
			return
		case errors.Is(err, db.ErrPlanNotFound):
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Piano copiato dalla settimana precedente.")
	a.redirectToWeek(w, r, weekStart)
}

// handleResetWeek POST /week/reset?start=YYYY-MM-DD — cancella tutti i
// meals della settimana corrente. Usato per annullare una copia o un
// apply-template fatti per sbaglio.
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
	a.Sessions.Put(r.Context(), "flash", "Settimana svuotata.")
	a.redirectToWeek(w, r, weekStart)
}

func (a *App) flashAndBack(w http.ResponseWriter, r *http.Request, weekStart time.Time, msg string) {
	a.Sessions.Put(r.Context(), "flash", msg)
	// Con HTMX un 303 verrebbe seguito dentro la XHR e la settimana intera
	// finirebbe nel frammento da sostituire: ricarichiamo la pagina.
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

// redirectToDay torna alla settimana con aperto il giorno dayIndex (0=lunedi').
func (a *App) redirectToDay(w http.ResponseWriter, r *http.Request, weekStart time.Time, dayIndex int) {
	if dayIndex < 0 || dayIndex > 6 {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	http.Redirect(w, r, weekDayURL(weekStart, weekStart.AddDate(0, 0, dayIndex)), http.StatusSeeOther)
}

// weekDayURL e' l'indirizzo della dashboard sulla settimana weekStart con
// aperto il giorno day.
func weekDayURL(weekStart, day time.Time) string {
	return "/?start=" + formatDate(weekStart) + "&day=" + formatDate(day)
}

// selectedDay sceglie il giorno aperto nella dashboard: quello in ?day= se
// cade nella settimana, altrimenti oggi se e' in settimana, altrimenti il
// lunedi'. Confronta le date come stringhe YYYY-MM-DD (vedi dayState).
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
	if !withinLen(name, MaxNameLen) {
		a.flashAndBack(w, r, weekStart, "Nome troppo lungo (max 200 caratteri).")
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
			http.Error(w, "pasto non trovato", http.StatusNotFound)
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
// Per ingredient supporta sia `ref_id` (locale gia' in DB) sia `barcode`
// (OFF, auto-import idempotente via resolveOFFBarcode).
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
		a.flashAndBack(w, r, weekStart, "Quantita' non valida.")
		return
	}

	var rPtr, iPtr *int64
	switch kind {
	case "recipe":
		idStr := r.PostFormValue("ref_id")
		refID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || refID <= 0 {
			a.flashAndBack(w, r, weekStart, "Seleziona una ricetta.")
			return
		}
		rPtr = &refID
	case "ingredient":
		barcode := strings.TrimSpace(r.PostFormValue("barcode"))
		if barcode != "" {
			ingID, err := a.resolveOFFBarcode(r.Context(), barcode)
			if err != nil {
				a.flashAndBack(w, r, weekStart, "Import OFF fallito: "+err.Error())
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
				a.flashAndBack(w, r, weekStart, "Seleziona un ingrediente.")
				return
			}
			iPtr = &refID
		}
	default:
		a.flashAndBack(w, r, weekStart, "Tipo non valido.")
		return
	}

	// qty_unit: 'g', 'ml', o 'pz'; ignorato per le ricette.
	qtyUnit := strings.TrimSpace(r.PostFormValue("qty_unit"))
	if qtyUnit != "g" && qtyUnit != "ml" && qtyUnit != "pz" {
		qtyUnit = "g"
	}

	if _, err := a.WeeklyPlans.AddItem(r.Context(), mealID, user.ID, rPtr, iPtr, qty, qtyUnit); err != nil {
		a.flashAndBack(w, r, weekStart, "Impossibile aggiungere la voce: "+err.Error())
		return
	}
	day, dayErr := a.dayIndexFromMealID(r, mealID)
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// splitSuggested tiene i suggeriti del piano nutrizionale il cui nome
// contiene q (tutti se q e' vuota) e li toglie dalle altre righe, cosi' nel
// picker ogni ingrediente compare una volta sola.
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
// Renderizza il partial picker (live HTMX) con le righe filtrate. Per
// kind=ingredient usa searchMixed (DB + OFF unificato). Per kind=recipe
// filtra le ricette visibili all'utente per nome.
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
	// dialog=1: il picker e' nella finestra sopra la settimana, quindi i form
	// aggiornano il giorno aperto via HTMX invece di ricaricare la pagina.
	inDialog := r.URL.Query().Get("dialog") == "1"
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	weekStart := parseWeekStart(r)
	weekStartStr := formatDate(weekStart)

	switch kind {
	case "ingredient":
		// Search OFF e' ora locale via FTS5: una sola chiamata sincrona
		// veloce (<50ms) restituisce locali + OFF unificati.
		rows, warn, err := a.searchMixed(r.Context(), q, includeOFFParam(r))
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		pkind := "meal_item"
		if asCheat {
			pkind = "meal_cheat"
		}

		// Suggeriti dal piano nutrizionista: attivi solo se meal_name e' fornito
		// e il picker non e' in modalita' cheat (sgarri non hanno suggerimenti).
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
		if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	case "recipe":
		data, err := a.recipePicker(r, q)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.MealID, data.WeekStart, data.Cheat, data.InDialog = mealID, weekStartStr, asCheat, inDialog
		if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "recipe_picker_results", data); err != nil {
			a.serverError(w, r, err)
		}
	default:
		http.Error(w, "kind non valido", http.StatusBadRequest)
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
			http.Error(w, "voce non trovata", http.StatusNotFound)
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
// Body: ids=12,9,15  (form-encoded). Persiste l'ordine dei meal_id per quel
// giorno e ritorna il partial day_card aggiornato.
func (a *App) handleReorderMeals(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	dayStr := r.URL.Query().Get("day_of_week")
	day, err := strconv.Atoi(dayStr)
	if err != nil || day < 0 || day > 6 {
		http.Error(w, "day_of_week mancante o invalido", http.StatusBadRequest)
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
			http.Error(w, "ordine non valido", http.StatusConflict)
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
			http.Error(w, "ordine non valido", http.StatusConflict)
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

// handleDayCard GET /days/{date} ritorna la card piena (+ scheda OOB) per
// il giorno indicato. Chiamato dalle schede dei giorni (hx-get) quando
// l'utente apre un altro giorno della settimana.
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

// handleAddItemForm GET /meals/{mealID}/items/new — shell del picker live
// HTMX. Le liste sono caricate via /meals/{id}/picker, niente precaricamento.
// Chiamato da HTMX (link della settimana) risponde con la sola finestra
// <dialog>, che modal.js apre sopra la settimana; senza JS e' una pagina.
type addItemFormData struct {
	authPageData
	MealID    int64
	MealName  string
	Date      time.Time // giorno del pasto, per il sottotitolo
	WeekStart string
	Cheat     bool // true = aggiungi come sgarro (POSTa a /meals/{id}/cheat/items)
	InDialog  bool // true = le aggiunte aggiornano il giorno via HTMX
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
		err = a.Render.RenderPartial(w, http.StatusOK, "meal_item_new", "meal_picker_dialog", data)
	} else {
		err = a.Render.Render(w, http.StatusOK, "meal_item_new", data)
	}
	if err != nil {
		a.serverError(w, r, err)
	}
}
