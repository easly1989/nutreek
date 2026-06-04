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
	Plan         *domain.WeeklyPlanFull
	Days         []dayView     // legacy: usato nei test eventuali
	AllCards     []dayCardData // 7 giorni, in ordine, ognuno renderizzato come pill in timeline
	FocusCards   []dayCardData // sotto-insieme di AllCards: card piene in #focus-grid
	WeekStart    time.Time
	WeekStartStr string // YYYY-MM-DD per i form
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

// dayCardData e' il view-model passato ai partial day_pill.html (timeline)
// e day_card.html (focus-grid). Self-contained per essere renderizzabile
// sia inline (dashboard) sia da solo (HTMX swap dopo mutation).
//
// `State` descrive il giorno calendariale (past/today/future). `InFocus` dice
// se il giorno e' attualmente nella griglia focus. Default iniziale:
// today + future = InFocus=true; past = InFocus=false.
type dayCardData struct {
	Day         dayView
	DateStr     string // YYYY-MM-DD per data-day attribute
	State       string // "past" | "today" | "future"
	InFocus     bool   // true = card piena visibile in #focus-grid
	WeekStart   string // YYYY-MM-DD per nascondere/passare nei form
	CSRFToken   string
	Progress    profile.MacroProgress
	WorkoutKcal float64              // kcal bruciate con allenamenti in quel giorno
	Workouts    []domain.WorkoutLog  // allenamenti del giorno
	Activities  []domain.WorkoutActivity // per il select nel form di aggiunta
	Water       waterWidgetData      // widget idratazione del giorno
}

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
	if it.QuantityUnit == "pz" && it.Ingredient != nil && it.Ingredient.UnitWeightG != nil {
		return it.Quantity * (*it.Ingredient.UnitWeightG)
	}
	return it.Quantity
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

// makeDayCard costruisce dayCardData a partire da una dayView e dal contesto.
// `state` e' il giorno calendariale; `inFocus` decide se andra' anche nella
// griglia focus. Default: today/future = true, past = false.
func makeDayCard(dv dayView, state string, inFocus bool, weekStartStr, csrfToken string, progress profile.MacroProgress, workoutKcal float64, workouts []domain.WorkoutLog, activities []domain.WorkoutActivity) dayCardData {
	return dayCardData{
		Day:         dv,
		DateStr:     formatDate(dv.Date),
		State:       state,
		InFocus:     inFocus,
		WeekStart:   weekStartStr,
		CSRFToken:   csrfToken,
		Progress:    progress,
		WorkoutKcal: workoutKcal,
		Workouts:    workouts,
		Activities:  activities,
	}
}

// reorderFocusCards porta la card "today" in prima posizione; le restanti
// mantengono l'ordine Lun-Dom originale (skipping today). Le pillole nella
// timeline rimangono sempre in ordine calendariale fisso.
func reorderFocusCards(cards []dayCardData) []dayCardData {
	idx := -1
	for i, c := range cards {
		if c.State == "today" {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return cards // today già prima o assente
	}
	out := make([]dayCardData, 0, len(cards))
	out = append(out, cards[idx])
	out = append(out, cards[:idx]...)
	out = append(out, cards[idx+1:]...)
	return out
}

// buildDayCards costruisce le 7 dayCardData con InFocus default-driven dallo
// stato calendariale: today + future = true, past = false. La timeline
// renderizza tutte e 7 le pillole; #focus-grid solo quelle InFocus.
func buildDayCards(days []dayView, today time.Time, weekStartStr, csrfToken string, user *domain.User, workoutKcals [7]float64, workoutLogs [7][]domain.WorkoutLog, activities []domain.WorkoutActivity, waterMLs [7]int) []dayCardData {
	out := make([]dayCardData, 0, 7)
	allPast := len(days) > 0 && dayState(days[len(days)-1].Date, today) == "past"
	target := waterTargetFor(user)
	for i, dv := range days {
		state := dayState(dv.Date, today)
		inFocus := allPast || state != "past"
		var progress profile.MacroProgress
		if user != nil {
			progress = profile.BuildMacroProgress(dv.DayTotal, user.TargetKcal, user.TargetProtein, user.TargetCarbs, user.TargetFat)
		}
		card := makeDayCard(dv, state, inFocus, weekStartStr, csrfToken, progress, workoutKcals[i], workoutLogs[i], activities)
		card.Water = waterWidgetData{DateStr: card.DateStr, WaterML: waterMLs[i], WaterTargetML: target, CSRFToken: csrfToken}
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

// dayMutationResponse e' il view-model per day_card_with_pill_oob: card +
// pillola OOB + totali settimanali OOB. Serve per aggiornare in unica risposta
// tutti gli elementi che dipendono dai macro dopo una mutazione (add, remove,
// reorder), in modo che il client non debba ricaricare la pagina.
type dayMutationResponse struct {
	Card      dayCardData
	WeekTotal macros.Macros
}

// renderDayCard scrive il partial day_card piu' la pillola corrispondente
// con HX-OOB swap, in un unico response. Cosi' una mutation aggiorna sia
// la card in #focus-grid sia la pillola in #timeline-pills (kcal totali,
// accent state).
func (a *App) renderDayCard(w http.ResponseWriter, r *http.Request, dv dayView, weekTotal macros.Macros, weekStart time.Time, state string) {
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
	if a.WaterLogs != nil && user != nil {
		ml, _ := a.WaterLogs.TotalForDate(r.Context(), user.ID, dv.Date)
		card.Water = waterWidgetData{DateStr: card.DateStr, WaterML: ml, WaterTargetML: waterTargetFor(user), CSRFToken: card.CSRFToken}
	}
	resp := dayMutationResponse{Card: card, WeekTotal: weekTotal}
	if err := a.Render.RenderPartial(w, http.StatusOK, "dashboard", "day_card_with_pill_oob", resp); err != nil {
		a.serverError(w, r, err)
	}
}

// loadDayCard carica il piano dell'utente per la settimana data, costruisce
// la dayView del giorno richiesto e ritorna anche il totale settimanale per
// permettere l'aggiornamento OOB del riquadro macro in alto.
func (a *App) loadDayCard(r *http.Request, userID int64, weekStart time.Time, dayIndex int) (*dayView, macros.Macros, error) {
	if dayIndex < 0 || dayIndex > 6 {
		return nil, macros.Macros{}, fmt.Errorf("day_index fuori range")
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), userID, weekStart)
	if err != nil {
		return nil, macros.Macros{}, err
	}
	full, err := a.WeeklyPlans.LoadFull(r.Context(), plan)
	if err != nil {
		return nil, macros.Macros{}, err
	}
	days := buildDayViews(full, weekStart)
	dv := days[dayIndex]
	weekTotal := macros.ForWeek(full.Days)
	return &dv, weekTotal, nil
}

// nosurfTokenOrEmpty e' un thin wrapper per leggere il CSRF token; usato dai
// renderer di partial dove non c'e' un baseData() pre-costruito.
func nosurfTokenOrEmpty(r *http.Request) string {
	return nosurf.Token(r)
}

// respondMutationOrRedirect gestisce in modo uniforme il dual-mode (HTMX
// vs full navigation) dopo una mutazione su un meal/item. Per HX-Request
// ritorna la card piena + la pillola OOB; altrimenti redirect alla settimana.
func (a *App) respondMutationOrRedirect(w http.ResponseWriter, r *http.Request, weekStart time.Time, dayIndex int) {
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	user := userFromContext(r.Context())
	dv, weekTotal, err := a.loadDayCard(r, user.ID, weekStart, dayIndex)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	state := dayState(dv.Date, todayDate())
	a.renderDayCard(w, r, *dv, weekTotal, weekStart, state)
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
	allCards := buildDayCards(days, today, weekStartStr, base.CSRFToken, user, workoutKcals, workoutLogs, activities, waterMLs)
	focusCards := make([]dayCardData, 0, len(allCards))
	for _, c := range allCards {
		if c.InFocus {
			focusCards = append(focusCards, c)
		}
	}
	focusCards = reorderFocusCards(focusCards)
	weekTotal := macros.ForWeek(full.Days)

	data := dashboardData{
		authPageData: base,
		Plan:         full,
		Days:         days,
		AllCards:     allCards,
		FocusCards:   focusCards,
		WeekStart:    weekStart,
		WeekStartStr: weekStartStr,
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
			for _, ing := range suggested {
				nutrRows = append(nutrRows, ingredientRow{Local: ing})
			}
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
		// Filtra per nome (case-insensitive).
		filtered := recipes
		if q != "" {
			ql := strings.ToLower(q)
			filtered = filtered[:0]
			for _, rc := range recipes {
				if strings.Contains(strings.ToLower(rc.Name), ql) {
					filtered = append(filtered, rc)
				}
			}
		}
		data := struct {
			authPageData
			Recipes   []*domain.Recipe
			Query     string
			MealID    int64
			WeekStart string
			Cheat     bool
		}{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			Recipes:      filtered,
			Query:        q,
			MealID:       mealID,
			WeekStart:    weekStartStr,
			Cheat:        asCheat,
		}
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

// parseIDList interpreta il valore form `ids` come lista comma-separated e
// ritorna gli int64 corrispondenti. Errore se vuoto o malformato.
func parseIDList(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("ids vuoto")
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("id non valido: %q", p)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ids vuoto")
	}
	return out, nil
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

// handleDayCard GET /days/{date} ritorna la card piena (+ pillola OOB) per
// il giorno indicato. Chiamato dal client quando l'utente porta un giorno
// in focus (click su una pillola not-in-focus). Il "togliere dal focus"
// e' UI-only, lato client (rimuove la card dal DOM).
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
	dv, weekTotal, err := a.loadDayCard(r, user.ID, weekStart, dayIndex)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	state := dayState(dv.Date, todayDate())
	a.renderDayCard(w, r, *dv, weekTotal, weekStart, state)
}

// handleAddItemForm GET /meals/{mealID}/items/new — shell del picker live
// HTMX. Le liste sono caricate via /meals/{id}/picker, niente precaricamento.
type addItemFormData struct {
	authPageData
	MealID    int64
	MealName  string
	WeekStart string
	Cheat     bool // true = aggiungi come sgarro (POSTa a /meals/{id}/cheat/items)
	Error     string
}

func (a *App) handleAddItemForm(w http.ResponseWriter, r *http.Request) {
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	mealName := strings.TrimSpace(r.URL.Query().Get("meal_name"))
	data := addItemFormData{
		authPageData: a.baseData(r),
		MealID:       mealID,
		MealName:     mealName,
		WeekStart:    formatDate(weekStart),
		Cheat:        r.URL.Query().Get("cheat") == "1",
	}
	if err := a.Render.Render(w, http.StatusOK, "meal_item_new", data); err != nil {
		a.serverError(w, r, err)
	}
}
