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
)

// ---- view models -----------------------------------------------------------

type dashboardData struct {
	authPageData
	Plan         *domain.WeeklyPlanFull
	Days         []dayView // legacy: usato nei test eventuali
	PastDays     []dayCardData
	FocusDays    []dayCardData
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

// dayCardData e' il view-model passato al partial day_card.html. Self-contained
// per essere renderizzabile sia inline (dashboard) sia da solo (HTMX swap dopo
// add/remove meal o item).
type dayCardData struct {
	Day       dayView
	DateStr   string // YYYY-MM-DD per data-day attribute
	State     string // "past" | "today" | "future" | "focus"
	WeekStart string // YYYY-MM-DD per nascondere/passare nei form
	CSRFToken string
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
			mv := mealView{Meal: m}
			for _, it := range m.Items {
				macroIt := macros.ForMealItem(it)
				mv.ItemViews = append(mv.ItemViews, mealItemView{Item: it, Macros: macroIt})
				mv.MealTotal = mv.MealTotal.Add(macroIt)
			}
			dv.Meals = append(dv.Meals, mv)
			dv.DayTotal = dv.DayTotal.Add(mv.MealTotal)
		}
		out[d] = dv
	}
	return out
}

// makeDayCard costruisce dayCardData a partire da una dayView e dal contesto.
func makeDayCard(dv dayView, state, weekStartStr, csrfToken string) dayCardData {
	return dayCardData{
		Day:       dv,
		DateStr:   formatDate(dv.Date),
		State:     state,
		WeekStart: weekStartStr,
		CSRFToken: csrfToken,
	}
}

// splitDayCards divide i 7 giorni in due liste in base al giorno odierno:
// passati (collassati nella timeline) e focus (oggi + futuri, in griglia
// piena). Il giorno odierno e' sempre primo nei focus.
func splitDayCards(days []dayView, today time.Time, weekStartStr, csrfToken string) (past, focus []dayCardData) {
	past = make([]dayCardData, 0, 6)
	focus = make([]dayCardData, 0, 7)
	for _, dv := range days {
		state := dayState(dv.Date, today)
		card := makeDayCard(dv, state, weekStartStr, csrfToken)
		if state == "past" {
			past = append(past, card)
		} else {
			focus = append(focus, card)
		}
	}
	return past, focus
}

// renderDayCard scrive il partial day_card per una specifica dayView. Usato
// per gli swap HTMX dopo le mutazioni e per l'endpoint /days/{date}.
func (a *App) renderDayCard(w http.ResponseWriter, r *http.Request, dv dayView, weekStart time.Time, state string) {
	card := makeDayCard(dv, state, formatDate(weekStart), nosurfTokenOrEmpty(r))
	if err := a.Render.RenderPartial(w, http.StatusOK, "dashboard", "day_card", card); err != nil {
		a.serverError(w, r, err)
	}
}

// loadDayCard carica il piano dell'utente per la settimana data, costruisce
// la dayView del giorno richiesto e ritorna nil se il giorno e' fuori range.
func (a *App) loadDayCard(r *http.Request, userID int64, weekStart time.Time, dayIndex int) (*dayView, error) {
	if dayIndex < 0 || dayIndex > 6 {
		return nil, fmt.Errorf("day_index fuori range")
	}
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), userID, weekStart)
	if err != nil {
		return nil, err
	}
	full, err := a.WeeklyPlans.LoadFull(r.Context(), plan)
	if err != nil {
		return nil, err
	}
	days := buildDayViews(full, weekStart)
	dv := days[dayIndex]
	return &dv, nil
}

// nosurfTokenOrEmpty e' un thin wrapper per leggere il CSRF token; usato dai
// renderer di partial dove non c'e' un baseData() pre-costruito.
func nosurfTokenOrEmpty(r *http.Request) string {
	return nosurf.Token(r)
}

// respondMutationOrRedirect gestisce in modo uniforme il dual-mode (HTMX
// vs full navigation) dopo una mutazione su un meal/item. Per HX-Request
// ritorna il partial day_card; altrimenti redirect alla settimana.
func (a *App) respondMutationOrRedirect(w http.ResponseWriter, r *http.Request, weekStart time.Time, dayIndex int) {
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	user := userFromContext(r.Context())
	dv, err := a.loadDayCard(r, user.ID, weekStart, dayIndex)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	state := dayState(dv.Date, todayDate())
	// Se la mutazione e' avvenuta su un giorno passato, l'utente l'ha
	// "messo a focus" nella timeline: rimaniamo in focus per non far
	// sparire la card mentre la sta usando.
	if state == "past" {
		state = "focus"
	}
	a.renderDayCard(w, r, *dv, weekStart, state)
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
	pastDays, focusDays := splitDayCards(days, today, weekStartStr, base.CSRFToken)
	weekTotal := macros.ForWeek(full.Days)

	data := dashboardData{
		authPageData: base,
		Plan:         full,
		Days:         days,
		PastDays:     pastDays,
		FocusDays:    focusDays,
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
func (a *App) handleAddMealItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
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
	day, dayErr := a.dayIndexFromMealID(r, mealID)
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
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

// handleDayCard GET /days/{date}?focus=1 ritorna il partial day_card per il
// giorno indicato. Usato dalla timeline per "promuovere" un giorno passato
// nella griglia focus senza ricaricare la pagina.
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
	dv, err := a.loadDayCard(r, user.ID, weekStart, dayIndex)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	state := dayState(dv.Date, todayDate())
	if r.URL.Query().Get("focus") == "1" {
		state = "focus"
	}
	a.renderDayCard(w, r, *dv, weekStart, state)
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
		a.notFound(w, r)
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
