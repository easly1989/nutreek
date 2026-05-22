package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

// ---- view models -----------------------------------------------------------

// ingredientRow rappresenta una riga della lista risultati: puo' essere un
// ingrediente gia' nel DB oppure un prodotto OFF candidato all'import.
type ingredientRow struct {
	Local *domain.Ingredient // nil se la riga e' un OFF non ancora importato
	OFF   *off.Product       // nil se la riga e' gia' nel DB
}

// IsOFF e' usato dal template per decidere se mostrare il pulsante Importa.
func (r ingredientRow) IsOFF() bool { return r.Local == nil && r.OFF != nil }

// Name normalizza il nome per il template.
func (r ingredientRow) Name() string {
	if r.Local != nil {
		return r.Local.Name
	}
	if r.OFF != nil {
		return r.OFF.Name
	}
	return ""
}

// Brand normalizza il brand per il template.
func (r ingredientRow) Brand() string {
	if r.Local != nil && r.Local.Brand != nil {
		return *r.Local.Brand
	}
	if r.OFF != nil {
		return r.OFF.Brand
	}
	return ""
}

// SourceLabel ritorna la label per il badge.
func (r ingredientRow) SourceLabel() string {
	if r.Local != nil {
		if r.Local.Source == domain.IngredientSourceOFF {
			return "OpenFoodFacts"
		}
		return "Locale"
	}
	return "OpenFoodFacts (importabile)"
}

// Kcal/Protein/Carbs/Fat/Unit espongono i macro per il template.
// I template Go non possono unpackare multi-return, quindi un getter per campo.
func (r ingredientRow) Kcal() float64 {
	if r.Local != nil {
		return r.Local.KcalPer100
	}
	if r.OFF != nil {
		return r.OFF.Kcal
	}
	return 0
}
func (r ingredientRow) Protein() float64 {
	if r.Local != nil {
		return r.Local.ProteinPer100
	}
	if r.OFF != nil {
		return r.OFF.Protein
	}
	return 0
}
func (r ingredientRow) Carbs() float64 {
	if r.Local != nil {
		return r.Local.CarbsPer100
	}
	if r.OFF != nil {
		return r.OFF.Carbs
	}
	return 0
}
func (r ingredientRow) Fat() float64 {
	if r.Local != nil {
		return r.Local.FatPer100
	}
	if r.OFF != nil {
		return r.OFF.Fat
	}
	return 0
}
func (r ingredientRow) Unit() string {
	if r.Local != nil {
		return string(r.Local.Unit)
	}
	return "g"
}

// Barcode espone l'identificatore OFF per la form di import.
func (r ingredientRow) Barcode() string {
	if r.OFF != nil {
		return r.OFF.Code
	}
	return ""
}

// Importable indica se la riga OFF puo' essere importata (nutrienti completi).
func (r ingredientRow) Importable() bool {
	return r.OFF != nil && r.OFF.HasFullNutrients
}

type ingredientsListData struct {
	authPageData
	Rows       []ingredientRow
	Query      string
	OFFWarning string // riempito se la search OFF e' fallita

	// Campi di context del picker. Quando il partial e' renderizzato in
	// modalita' "picker" (es. dentro la pagina recipe-edit o nella card
	// di un meal), questi campi guidano il form action e i parametri.
	// PickerKind = "" -> modalita' display (pagina /ingredients), nessun
	// bottone "Aggiungi" (solo "Importa" sui soli OFF).
	PickerKind string // "recipe_item" | "meal_item" | ""
	RecipeID   int64  // se PickerKind == "recipe_item"
	MealID     int64  // se PickerKind == "meal_item"
	WeekStart  string // YYYY-MM-DD, per redirect post-add da meal_item
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

// ---- handlers --------------------------------------------------------------

func (a *App) handleIngredientsList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	// Render iniziale SSR: includi OFF solo se l'utente arriva gia' con una
	// query in URL (link condiviso, refresh). Senza q, lista solo locali.
	rows, warn, err := a.searchMixed(r.Context(), q, q != "")
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := ingredientsListData{
		authPageData: a.baseData(r),
		Rows:         rows,
		Query:        q,
		OFFWarning:   warn,
	}
	if err := a.Render.Render(w, http.StatusOK, "ingredients_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleIngredientsSearchPartial risponde a HTMX con il solo partial
// dei risultati. Usato dall'input live-search.
// Param `off=1` per includere OpenFoodFacts; senza, solo DB locale (veloce).
func (a *App) handleIngredientsSearchPartial(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, warn, err := a.searchMixed(r.Context(), q, includeOFFParam(r))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// CSRF token serve nel partial per il form Importa.
	data := ingredientsListData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Rows:         rows,
		Query:        q,
		OFFWarning:   warn,
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "ingredient_results", data); err != nil {
		a.serverError(w, r, err)
	}
}

// includeOFFParam ritorna true se il client ha richiesto esplicitamente
// l'integrazione con OpenFoodFacts (param off=1). Default false: il DB
// locale risponde a ogni keystroke, OFF viene chiamato solo a digitazione
// ferma per evitare query parziali.
func includeOFFParam(r *http.Request) bool {
	v := r.URL.Query().Get("off")
	return v == "1" || v == "true"
}

// offLiveTimeout limita l'attesa di OFF durante la ricerca live. Tre
// secondi erano troppi: il server teneva aperta la connessione HTTP per
// tutto il timeout del client OFF. Con 1.5s si degrada rapidamente
// mostrando solo i locali, senza bloccare la UI.
const offLiveTimeout = 1500 * time.Millisecond

// searchMixed esegue search locale e, se includeOFF==true e q != "",
// integra i risultati OpenFoodFacts deduplicati per barcode.
// warn != "" se OFF e' fallito (degradiamo).
func (a *App) searchMixed(ctx context.Context, q string, includeOFF bool) ([]ingredientRow, string, error) {
	var (
		locals []*domain.Ingredient
		err    error
	)
	if q == "" {
		locals, err = a.Ingredients.ListActive(ctx, 0, 50)
	} else {
		locals, err = a.Ingredients.SearchActive(ctx, q, 20)
	}
	if err != nil {
		return nil, "", err
	}

	rows := make([]ingredientRow, 0, len(locals))
	knownBarcodes := make(map[string]bool, len(locals))
	for _, l := range locals {
		rows = append(rows, ingredientRow{Local: l})
		if l.OFFBarcode != nil {
			knownBarcodes[*l.OFFBarcode] = true
		}
	}

	// OFF e' opzionale: il client live-search lo richiede solo a digitazione
	// ferma. Senza query non interroghiamo OFF (lista vuota = no-op).
	if !includeOFF || q == "" {
		return rows, "", nil
	}

	offCtx, offCancel := context.WithTimeout(ctx, offLiveTimeout)
	defer offCancel()
	products, offErr := a.OFF.SearchByName(offCtx, q)
	if offErr != nil {
		a.Logger.Warn("OFF search failed", "err", offErr, "q", q)
		return rows, "OpenFoodFacts non raggiungibile, mostro solo gli ingredienti locali.", nil
	}
	// Filtro strict AND lato UI: ogni termine deve comparire in nome/brand
	// del prodotto. Compensa il default OR di Search-a-licious che
	// altrimenti restituisce troppi falsi positivi (es. "avena conad"
	// matcherebbe ogni prodotto col brand "Avena" o "Conad" separati).
	products = off.FilterByAllTerms(products, q)
	for i := range products {
		p := products[i]
		if knownBarcodes[p.Code] {
			continue // gia' importato
		}
		rows = append(rows, ingredientRow{OFF: &p})
	}
	return rows, "", nil
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

// handleIngredientOFFImport importa per barcode dal pulsante "Importa".
// Idempotente: se gia' presente in DB, flash "gia' presente" senza errore.
func (a *App) handleIngredientOFFImport(w http.ResponseWriter, r *http.Request) {
	barcode := strings.TrimSpace(r.PostFormValue("barcode"))
	if barcode == "" {
		http.Error(w, "barcode mancante", http.StatusBadRequest)
		return
	}

	// Pre-check: gia' importato?
	existing, err := a.Ingredients.GetByBarcode(r.Context(), barcode)
	if err != nil && !errors.Is(err, db.ErrIngredientNotFound) {
		a.serverError(w, r, err)
		return
	}
	if existing != nil {
		a.Sessions.Put(r.Context(), "flash", "Ingrediente gia' presente nel DB.")
		http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
		return
	}

	product, err := a.OFF.GetByBarcode(r.Context(), barcode)
	if err != nil {
		if errors.Is(err, off.ErrNotFound) {
			http.Error(w, "Prodotto non trovato su OpenFoodFacts", http.StatusNotFound)
			return
		}
		a.Logger.Error("OFF GetByBarcode failed", "err", err, "barcode", barcode)
		http.Error(w, "OpenFoodFacts non raggiungibile, riprova fra poco.", http.StatusBadGateway)
		return
	}
	if !product.HasFullNutrients {
		http.Error(w, "Il prodotto OFF non ha tutti i macro: crea un ingrediente locale.", http.StatusUnprocessableEntity)
		return
	}

	var brandPtr *string
	if b := strings.TrimSpace(product.Brand); b != "" {
		brandPtr = &b
	}
	if _, err := a.Ingredients.CreateOFF(
		r.Context(),
		product.Code, product.Name, brandPtr,
		domain.UnitGram, // OFF e' sempre per 100g (per liquidi spesso aggiungono 100ml in ratio fra loro, lasciamo g per MVP)
		product.Kcal, product.Protein, product.Carbs, product.Fat,
	); err != nil && !errors.Is(err, db.ErrBarcodeAlreadyUsed) {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ingrediente importato da OpenFoodFacts.")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}

// resolveOFFBarcode importa l'OFF identificato dal barcode nel DB locale e
// ritorna l'id dell'ingrediente locale corrispondente. Idempotente: se il
// barcode e' gia' nel DB, ritorna l'esistente senza richiamare OFF.
// Se OFF non ha tutti i nutrienti, ritorna errore.
//
// Usato sia da handleRecipeAddItem (auto-import al click) sia (in tappa
// 16) dal flow meal_item, per evitare duplicare la logica.
func (a *App) resolveOFFBarcode(ctx context.Context, barcode string) (int64, error) {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return 0, errEmpty
	}
	// Pre-check DB: evita una chiamata HTTP se gia' importato.
	existing, err := a.Ingredients.GetByBarcode(ctx, barcode)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, db.ErrIngredientNotFound) {
		return 0, err
	}
	// Non in DB: chiama OFF.
	product, err := a.OFF.GetByBarcode(ctx, barcode)
	if err != nil {
		return 0, err
	}
	if !product.HasFullNutrients {
		return 0, errors.New("prodotto OFF senza macro completi")
	}
	var brandPtr *string
	if b := strings.TrimSpace(product.Brand); b != "" {
		brandPtr = &b
	}
	id, err := a.Ingredients.CreateOFF(
		ctx,
		product.Code, product.Name, brandPtr,
		domain.UnitGram,
		product.Kcal, product.Protein, product.Carbs, product.Fat,
	)
	if err == nil {
		return id, nil
	}
	if errors.Is(err, db.ErrBarcodeAlreadyUsed) {
		// Race / barcode appena creato da un altro path: recupera.
		got, gerr := a.Ingredients.GetByBarcode(ctx, product.Code)
		if gerr != nil {
			return 0, gerr
		}
		return got.ID, nil
	}
	return 0, err
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
