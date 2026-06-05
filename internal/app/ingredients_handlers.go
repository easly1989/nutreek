package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
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

	// Allergens e' popolato (batch) solo per le righe locali taggate: i
	// template lo usano per il badge ⚠ nel picker e nella lista.
	Allergens []domain.AllergenTag
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

// LocalID ritorna l'ID DB dell'ingrediente locale (0 se la riga e' solo OFF).
func (r ingredientRow) LocalID() int64 {
	if r.Local != nil {
		return r.Local.ID
	}
	return 0
}

// Barcode espone l'identificatore OFF per la form di import.
func (r ingredientRow) Barcode() string {
	if r.OFF != nil {
		return r.OFF.Code
	}
	return ""
}

// Importable indica se la riga OFF puo' essere proposta per l'import. Sempre
// true per gli OFF risultanti dalla FTS locale: il dump esportato da OFF ha
// nutrienti incompleti per quasi tutti i prodotti italiani (volontari che
// inseriscono solo nome/barcode), ma l'import vero passa per GetByBarcode
// sulla live API, che invece ha i dati aggiornati. La validazione finale
// "ha macro completi?" avviene quindi nell'handler di import, non qui.
func (r ingredientRow) Importable() bool {
	return r.OFF != nil
}

// HasFullNutrients indica al template se mostrare i macro o un placeholder.
// I locali hanno sempre macro completi (validati al create). Gli OFF derivati
// dal dump spesso no (per i prodotti italiani il dump e' quasi sempre vuoto);
// in quel caso il template mostra un hint invece dei valori 0/0/0/0 fuorvianti.
func (r ingredientRow) HasFullNutrients() bool {
	if r.Local != nil {
		return true
	}
	return r.OFF != nil && r.OFF.HasFullNutrients
}

// ImageURL espone la URL del thumbnail OFF. Se il dump non ha image_url
// (prodotti importati prima della migrazione 0012), costruisce una URL CDN
// best-effort dal barcode — gestire con onerror nel template.
func (r ingredientRow) ImageURL() string {
	if r.OFF != nil {
		if r.OFF.ImageURL != "" {
			return r.OFF.ImageURL
		}
		return off.OFFCDNImageURL(r.OFF.Code)
	}
	return ""
}

// DefaultQuantity ritorna la quantita' predefinita per il picker (0 = non impostata).
func (r ingredientRow) DefaultQuantity() float64 {
	if r.Local != nil && r.Local.DefaultQuantity != nil {
		return *r.Local.DefaultQuantity
	}
	return 0
}

// UnitWeightG espone il peso medio per unita' se disponibile (parsing OFF o
// override manuale). Restituisce 0 se non noto: il template usa questo per
// decidere se mostrare il selettore g/pz nel picker.
func (r ingredientRow) UnitWeightG() float64 {
	if r.Local != nil && r.Local.UnitWeightG != nil {
		return *r.Local.UnitWeightG
	}
	if r.OFF != nil && r.OFF.UnitWeightG != nil {
		return *r.OFF.UnitWeightG
	}
	return 0
}

// HasMicronutrients ritorna true se il locale ha almeno un micronutriente.
func (r ingredientRow) HasMicronutrients() bool {
	if r.Local == nil {
		return false
	}
	return r.Local.FiberPer100 != nil || r.Local.SugarsPer100 != nil ||
		r.Local.SaturatedFatPer100 != nil || r.Local.SodiumPer100 != nil
}

// FiberPer100Val ritorna le fibre per 100g/ml (0 se non disponibili).
func (r ingredientRow) FiberPer100Val() float64 {
	if r.Local != nil && r.Local.FiberPer100 != nil {
		return *r.Local.FiberPer100
	}
	return 0
}

// SugarsPer100Val ritorna gli zuccheri per 100g/ml (0 se non disponibili).
func (r ingredientRow) SugarsPer100Val() float64 {
	if r.Local != nil && r.Local.SugarsPer100 != nil {
		return *r.Local.SugarsPer100
	}
	return 0
}

// SaturatedFatPer100Val ritorna i grassi saturi per 100g/ml (0 se non disponibili).
func (r ingredientRow) SaturatedFatPer100Val() float64 {
	if r.Local != nil && r.Local.SaturatedFatPer100 != nil {
		return *r.Local.SaturatedFatPer100
	}
	return 0
}

// SodiumPer100Val ritorna il sodio per 100g/ml (0 se non disponibili).
func (r ingredientRow) SodiumPer100Val() float64 {
	if r.Local != nil && r.Local.SodiumPer100 != nil {
		return *r.Local.SodiumPer100
	}
	return 0
}

type ingredientsListData struct {
	authPageData
	Rows       []ingredientRow
	Query      string
	OFFWarning string // riempito se la search OFF locale e' fallita (raro)

	// Filtro allergeni della pagina /ingredients (non usato dai picker).
	AllAllergens       []domain.AllergenTag
	SelectedAllergenID int64

	// Campi di context del picker. Quando il partial e' renderizzato in
	// modalita' "picker" (es. dentro la pagina recipe-edit o nella card
	// di un meal), questi campi guidano il form action e i parametri.
	// PickerKind = "" -> modalita' display (pagina /ingredients), nessun
	// bottone "Aggiungi" (solo "Importa" sui soli OFF).
	PickerKind        string         // "recipe_item" | "meal_item" | "nutritionist_item" | ""
	RecipeID          int64          // se PickerKind == "recipe_item"
	MealID            int64          // se PickerKind == "meal_item"
	VariationID       int64          // se PickerKind == "nutritionist_item"
	WeekStart         string         // YYYY-MM-DD, per redirect post-add da meal_item
	NutritionistItems []ingredientRow // suggeriti dal piano nutrizionista
}

type ingredientsNewData struct {
	authPageData
	Form              ingredientForm
	Error             string
	Categories        []domain.IngredientCategory
	Allergens         []domain.AllergenTag
	SelectedAllergens map[int64]bool
}

// ingredientForm cattura i campi del form "nuovo/modifica ingrediente" per poter
// rifare il render in caso di errore senza perdere i valori inseriti.
type ingredientForm struct {
	Name            string
	Brand           string
	Unit            string
	Kcal            string
	Protein         string
	Carbs           string
	Fat             string
	UnitWeightG     string  // opzionale: peso per unita' in g (es. "150")
	DefaultQuantity string  // opzionale: quantita' predefinita nel picker
	CategoryID      string  // opzionale: ID categoria
	AllergenIDs     []int64 // opzionale: ID allergeni selezionati
	// Micronutrienti opzionali (per 100 g/ml)
	Fiber        string
	Sugars       string
	SaturatedFat string
	Sodium       string
}

// ---- handlers --------------------------------------------------------------

func (a *App) handleIngredientsList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	allergenID := parseAllergenFilter(r)
	// Render iniziale SSR: includi OFF solo se l'utente arriva gia' con una
	// query in URL (link condiviso, refresh). Senza q, lista solo locali.
	// Col filtro allergene attivo niente OFF: i prodotti non importati non
	// hanno tag allergeni, quindi sarebbero comunque esclusi.
	rows, warn, err := a.searchMixed(r.Context(), q, q != "" && allergenID == 0)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows = filterRowsByAllergen(rows, allergenID)
	allergens, _ := a.Categories.ListAllergens(r.Context())
	data := ingredientsListData{
		authPageData:       a.baseData(r),
		Rows:               rows,
		Query:              q,
		OFFWarning:         warn,
		AllAllergens:       allergens,
		SelectedAllergenID: allergenID,
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
	allergenID := parseAllergenFilter(r)
	rows, warn, err := a.searchMixed(r.Context(), q, includeOFFParam(r) && allergenID == 0)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	rows = filterRowsByAllergen(rows, allergenID)
	// CSRF token serve nel partial per il form Importa.
	data := ingredientsListData{
		authPageData:       authPageData{CSRFToken: nosurf.Token(r)},
		Rows:               rows,
		Query:              q,
		OFFWarning:         warn,
		SelectedAllergenID: allergenID,
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "ingredient_results", data); err != nil {
		a.serverError(w, r, err)
	}
}

// parseAllergenFilter legge il parametro ?allergen=ID dalla query (0 = nessun filtro).
func parseAllergenFilter(r *http.Request) int64 {
	if id, err := strconv.ParseInt(r.URL.Query().Get("allergen"), 10, 64); err == nil && id > 0 {
		return id
	}
	return 0
}

// filterRowsByAllergen tiene solo le righe che contengono l'allergene indicato.
// Con allergenID == 0 ritorna le righe invariate.
func filterRowsByAllergen(rows []ingredientRow, allergenID int64) []ingredientRow {
	if allergenID == 0 {
		return rows
	}
	out := rows[:0]
	for _, row := range rows {
		for _, tag := range row.Allergens {
			if tag.ID == allergenID {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// includeOFFParam ritorna true se il client ha richiesto esplicitamente
// l'integrazione con OpenFoodFacts (param off=1). Default false: il DB
// locale risponde a ogni keystroke, OFF viene chiamato solo a digitazione
// ferma per evitare query parziali.
func includeOFFParam(r *http.Request) bool {
	v := r.URL.Query().Get("off")
	return v == "1" || v == "true"
}

// searchMixed esegue search locale e, se includeOFF==true e q != "",
// integra i risultati OpenFoodFacts deduplicati per barcode.
// Ora la search OFF e' interamente locale via SQLite FTS5 (popolata dal
// Syncer dal dump pubblico), quindi <50ms, niente timeout, niente 503.
// `warn` resta nell'interfaccia per gestire eventuali errori SQL ma in
// pratica e' sempre vuoto.
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

	a.attachAllergens(ctx, rows)

	if !includeOFF || q == "" {
		return rows, "", nil
	}

	products, offErr := a.OFF.SearchByName(ctx, q)
	if offErr != nil {
		a.Logger.Warn("OFF search failed", "err", offErr, "q", q)
		return rows, "Ricerca OpenFoodFacts non disponibile, mostro solo gli ingredienti locali.", nil
	}
	// FTS5 fa AND nativo fra token + prefix match: nessun filtro aggiuntivo necessario.
	for i := range products {
		p := products[i]
		if knownBarcodes[p.Code] {
			continue // gia' importato
		}
		rows = append(rows, ingredientRow{OFF: &p})
	}
	return rows, "", nil
}

// attachAllergens carica in batch gli allergeni delle righe locali e li
// assegna in-place, cosi' il picker e la lista possono mostrare il badge ⚠
// senza query N+1. Le righe OFF non importate non hanno allergeni.
func (a *App) attachAllergens(ctx context.Context, rows []ingredientRow) {
	if a.Categories == nil {
		return
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Local != nil {
			ids = append(ids, row.Local.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	byIng, err := a.Categories.AllergensForIngredients(ctx, ids)
	if err != nil {
		a.Logger.Warn("attach allergens failed", "err", err)
		return
	}
	for i := range rows {
		if rows[i].Local != nil {
			rows[i].Allergens = byIng[rows[i].Local.ID]
		}
	}
}

// allergenIDSet converte una slice di ID in un set per il rendering dei
// checkbox "selezionati" nel form ingrediente.
func allergenIDSet(ids []int64) map[int64]bool {
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func (a *App) handleIngredientNewForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	cats, _ := a.Categories.ListCategories(r.Context(), user.ID)
	allergens, _ := a.Categories.ListAllergens(r.Context())
	data := ingredientsNewData{
		authPageData:      a.baseData(r),
		Form:              ingredientForm{Unit: "g"},
		Categories:        cats,
		Allergens:         allergens,
		SelectedAllergens: map[int64]bool{},
	}
	if err := a.Render.Render(w, http.StatusOK, "ingredient_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleIngredientCreate(w http.ResponseWriter, r *http.Request) {
	allergenStrs := r.PostForm["allergen_ids"]
	var parsedAllergenIDs []int64
	for _, s := range allergenStrs {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			parsedAllergenIDs = append(parsedAllergenIDs, id)
		}
	}
	form := ingredientForm{
		Name:         strings.TrimSpace(r.PostFormValue("name")),
		Brand:        strings.TrimSpace(r.PostFormValue("brand")),
		Unit:         strings.TrimSpace(r.PostFormValue("unit")),
		Kcal:         strings.TrimSpace(r.PostFormValue("kcal")),
		Protein:      strings.TrimSpace(r.PostFormValue("protein")),
		Carbs:        strings.TrimSpace(r.PostFormValue("carbs")),
		Fat:          strings.TrimSpace(r.PostFormValue("fat")),
		UnitWeightG:  strings.TrimSpace(r.PostFormValue("unit_weight_g")),
		CategoryID:   strings.TrimSpace(r.PostFormValue("category_id")),
		AllergenIDs:  parsedAllergenIDs,
		Fiber:        strings.TrimSpace(r.PostFormValue("fiber")),
		Sugars:       strings.TrimSpace(r.PostFormValue("sugars")),
		SaturatedFat: strings.TrimSpace(r.PostFormValue("saturated_fat")),
		Sodium:       strings.TrimSpace(r.PostFormValue("sodium")),
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

	// unit_weight_g opzionale: peso per unità (es. 150 per una banana da 150g).
	var unitWeightG *float64
	if s := strings.TrimSpace(r.PostFormValue("unit_weight_g")); s != "" {
		if v, err2 := parseNonNegFloat(s); err2 == nil && v > 0 {
			unitWeightG = &v
		}
	}

	// default_quantity opzionale: quantità predefinita nel picker.
	var defaultQuantity *float64
	if s := strings.TrimSpace(r.PostFormValue("default_quantity")); s != "" {
		if v, err2 := parseNonNegFloat(s); err2 == nil && v > 0 {
			defaultQuantity = &v
		}
	}

	ingID, err := a.Ingredients.CreateLocal(r.Context(), form.Name, brandPtr, unit, kcal, protein, carbs, fat, creatorID, unitWeightG, defaultQuantity,
		parseOptionalFloat(form.Fiber), parseOptionalFloat(form.Sugars),
		parseOptionalFloat(form.SaturatedFat), parseOptionalFloat(form.Sodium))
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	// Salva categoria (opzionale).
	if form.CategoryID != "" {
		if catID, err2 := strconv.ParseInt(form.CategoryID, 10, 64); err2 == nil && catID > 0 {
			_ = a.Categories.SetIngredientCategory(r.Context(), ingID, &catID)
		}
	}
	// Salva allergeni (opzionale).
	if len(form.AllergenIDs) > 0 {
		_ = a.Categories.SetIngredientAllergens(r.Context(), ingID, form.AllergenIDs)
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
		// Anche la live API non ha i 4 macro per questo barcode (dato di OFF
		// genuinamente incompleto). Redirige con flash invece di errore raw:
		// l'utente puo' creare un ingrediente locale manualmente.
		a.Sessions.Put(r.Context(), "flash",
			"OpenFoodFacts non ha i macro completi per questo prodotto. Creane uno locale.")
		http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
		return
	}

	var brandPtr *string
	if b := strings.TrimSpace(product.Brand); b != "" {
		brandPtr = &b
	}
	if _, err := a.Ingredients.CreateOFF(
		r.Context(),
		product.Code, product.Name, brandPtr,
		domain.UnitGram,
		product.Kcal, product.Protein, product.Carbs, product.Fat,
		product.UnitWeightG,
		product.Fiber, product.Sugars, product.SaturatedFat, product.Sodium,
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
		product.UnitWeightG,
		product.Fiber, product.Sugars, product.SaturatedFat, product.Sodium,
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

// handleOFFMacroCheck verifica i macro di un singolo prodotto OFF tramite
// barcode lookup. Prima controlla off_products locale (nessuna chiamata HTTP),
// poi cade su GetByBarcode live se il record locale manca o e' incompleto.
// Ritorna un fragment HTML con lo stato macro: green dot + valori, oppure
// dot rosso + "macro non disponibili". Usato da ingredient_results.html
// via HTMX lazy load (hx-trigger="load delay:500ms").
func (a *App) handleOFFMacroCheck(w http.ResponseWriter, r *http.Request) {
	barcode := chi.URLParam(r, "barcode")
	if barcode == "" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	// 1. Cerca nel DB locale (off_products) per evitare chiamate HTTP inutili.
	var name, brand string
	var kcal, protein, carbs, fat float64
	var hasFull int
	err := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT product_name, brands, kcal_per_100, protein_per_100, carbs_per_100, fat_per_100, has_full_nutrients
		FROM off_products WHERE barcode = ?`, barcode).Scan(
		&name, &brand, &kcal, &protein, &carbs, &fat, &hasFull)
	if err == nil && hasFull == 1 {
		writeMacroFragment(w, true, kcal, protein, carbs, fat)
		return
	}

	// 2. Live API check.
	product, err := a.OFF.GetByBarcode(r.Context(), barcode)
	if err != nil || !product.HasFullNutrients {
		writeMacroFragment(w, false, 0, 0, 0, 0)
		return
	}
	writeMacroFragment(w, true, product.Kcal, product.Protein, product.Carbs, product.Fat)
}

func writeMacroFragment(w http.ResponseWriter, full bool, kcal, protein, carbs, fat float64) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if full {
		fmt.Fprintf(w,
			`<span class="inline-block w-2 h-2 rounded-full mr-0.5 align-middle bg-accent"></span>`+
				`<span class="text-accent">OpenFoodFacts</span>`+
				` · %.0f kcal · P %.1fg · C %.1fg · F %.1fg (per 100g)`,
			kcal, protein, carbs, fat)
	} else {
		fmt.Fprintf(w,
			`<span class="inline-block w-2 h-2 rounded-full mr-0.5 align-middle bg-danger/60"></span>`+
				`<span class="text-muted">OpenFoodFacts</span>`+
				`<span class="text-danger/80 italic"> · macro non disponibili su OFF</span>`)
	}
}

type ingredientEditData struct {
	authPageData
	Form              ingredientForm
	Error             string
	IngredientID      int64
	IsOFF             bool
	Categories        []domain.IngredientCategory
	Allergens         []domain.AllergenTag
	SelectedAllergens map[int64]bool
}

func (a *App) handleIngredientEditForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	ing, err := a.Ingredients.GetByID(r.Context(), id)
	if err != nil {
		a.notFound(w, r)
		return
	}
	user := userFromContext(r.Context())
	cats, _ := a.Categories.ListCategories(r.Context(), user.ID)
	allergens, _ := a.Categories.ListAllergens(r.Context())
	selected, _ := a.Categories.GetIngredientAllergenIDs(r.Context(), id)

	form := ingredientForm{
		Name:    ing.Name,
		Unit:    string(ing.Unit),
		Kcal:    strconv.FormatFloat(ing.KcalPer100, 'f', -1, 64),
		Protein: strconv.FormatFloat(ing.ProteinPer100, 'f', -1, 64),
		Carbs:   strconv.FormatFloat(ing.CarbsPer100, 'f', -1, 64),
		Fat:     strconv.FormatFloat(ing.FatPer100, 'f', -1, 64),
	}
	if ing.Brand != nil {
		form.Brand = *ing.Brand
	}
	if ing.UnitWeightG != nil {
		form.UnitWeightG = strconv.FormatFloat(*ing.UnitWeightG, 'f', -1, 64)
	}
	if ing.DefaultQuantity != nil {
		form.DefaultQuantity = strconv.FormatFloat(*ing.DefaultQuantity, 'f', -1, 64)
	}
	if ing.FiberPer100 != nil {
		form.Fiber = strconv.FormatFloat(*ing.FiberPer100, 'f', -1, 64)
	}
	if ing.SugarsPer100 != nil {
		form.Sugars = strconv.FormatFloat(*ing.SugarsPer100, 'f', -1, 64)
	}
	if ing.SaturatedFatPer100 != nil {
		form.SaturatedFat = strconv.FormatFloat(*ing.SaturatedFatPer100, 'f', -1, 64)
	}
	if ing.SodiumPer100 != nil {
		form.Sodium = strconv.FormatFloat(*ing.SodiumPer100, 'f', -1, 64)
	}

	data := ingredientEditData{
		authPageData:      a.baseData(r),
		Form:              form,
		IngredientID:      id,
		IsOFF:             ing.Source == domain.IngredientSourceOFF,
		Categories:        cats,
		Allergens:         allergens,
		SelectedAllergens: selected,
	}
	if err := a.Render.Render(w, http.StatusOK, "ingredient_edit", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleIngredientUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	// Verifica che esista.
	ing, err := a.Ingredients.GetByID(r.Context(), id)
	if err != nil {
		a.notFound(w, r)
		return
	}

	var parsedAllergenIDs []int64
	for _, s := range r.PostForm["allergen_ids"] {
		if aid, err := strconv.ParseInt(s, 10, 64); err == nil {
			parsedAllergenIDs = append(parsedAllergenIDs, aid)
		}
	}
	form := ingredientForm{
		Name:            strings.TrimSpace(r.PostFormValue("name")),
		Brand:           strings.TrimSpace(r.PostFormValue("brand")),
		Unit:            strings.TrimSpace(r.PostFormValue("unit")),
		Kcal:            strings.TrimSpace(r.PostFormValue("kcal")),
		Protein:         strings.TrimSpace(r.PostFormValue("protein")),
		Carbs:           strings.TrimSpace(r.PostFormValue("carbs")),
		Fat:             strings.TrimSpace(r.PostFormValue("fat")),
		UnitWeightG:     strings.TrimSpace(r.PostFormValue("unit_weight_g")),
		DefaultQuantity: strings.TrimSpace(r.PostFormValue("default_quantity")),
		CategoryID:      strings.TrimSpace(r.PostFormValue("category_id")),
		AllergenIDs:     parsedAllergenIDs,
		Fiber:           strings.TrimSpace(r.PostFormValue("fiber")),
		Sugars:          strings.TrimSpace(r.PostFormValue("sugars")),
		SaturatedFat:    strings.TrimSpace(r.PostFormValue("saturated_fat")),
		Sodium:          strings.TrimSpace(r.PostFormValue("sodium")),
	}

	renderEditErr := func(msg string) {
		user := userFromContext(r.Context())
		cats, _ := a.Categories.ListCategories(r.Context(), user.ID)
		allergens, _ := a.Categories.ListAllergens(r.Context())
		data := ingredientEditData{
			authPageData:      a.baseData(r),
			Form:              form,
			Error:             msg,
			IngredientID:      id,
			IsOFF:             ing.Source == domain.IngredientSourceOFF,
			Categories:        cats,
			Allergens:         allergens,
			SelectedAllergens: allergenIDSet(form.AllergenIDs),
		}
		_ = a.Render.Render(w, http.StatusUnprocessableEntity, "ingredient_edit", data)
	}

	if form.Name == "" {
		renderEditErr("Il nome è obbligatorio.")
		return
	}
	if len(form.Name) > 200 {
		renderEditErr("Nome troppo lungo (max 200 caratteri).")
		return
	}
	unit := domain.IngredientUnit(form.Unit)
	if unit != domain.UnitGram && unit != domain.UnitMilliliter {
		renderEditErr("Unità non valida (usare g o ml).")
		return
	}

	kcal, err := parseNonNegFloat(form.Kcal)
	if err != nil {
		renderEditErr("Kcal/100 non valida: " + err.Error())
		return
	}
	protein, err := parseNonNegFloat(form.Protein)
	if err != nil {
		renderEditErr("Proteine/100 non valide: " + err.Error())
		return
	}
	carbs, err := parseNonNegFloat(form.Carbs)
	if err != nil {
		renderEditErr("Carbo/100 non validi: " + err.Error())
		return
	}
	fat, err := parseNonNegFloat(form.Fat)
	if err != nil {
		renderEditErr("Grassi/100 non validi: " + err.Error())
		return
	}

	var brandPtr *string
	if form.Brand != "" {
		brandPtr = &form.Brand
	}

	var unitWeightG *float64
	if s := form.UnitWeightG; s != "" {
		if v, err2 := parseNonNegFloat(s); err2 == nil && v > 0 {
			unitWeightG = &v
		}
	}
	var defaultQuantity *float64
	if s := form.DefaultQuantity; s != "" {
		if v, err2 := parseNonNegFloat(s); err2 == nil && v > 0 {
			defaultQuantity = &v
		}
	}

	if err := a.Ingredients.Update(r.Context(), id, form.Name, brandPtr, unit, kcal, protein, carbs, fat, unitWeightG, defaultQuantity,
		parseOptionalFloat(form.Fiber), parseOptionalFloat(form.Sugars),
		parseOptionalFloat(form.SaturatedFat), parseOptionalFloat(form.Sodium)); err != nil {
		a.serverError(w, r, err)
		return
	}

	// Aggiorna categoria.
	if form.CategoryID != "" {
		if catID, err2 := strconv.ParseInt(form.CategoryID, 10, 64); err2 == nil && catID > 0 {
			_ = a.Categories.SetIngredientCategory(r.Context(), id, &catID)
		}
	} else {
		_ = a.Categories.SetIngredientCategory(r.Context(), id, nil)
	}

	// Aggiorna allergeni (replace: lista vuota = rimuove tutti i tag).
	_ = a.Categories.SetIngredientAllergens(r.Context(), id, form.AllergenIDs)

	a.Sessions.Put(r.Context(), "flash", "Ingrediente aggiornato.")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}

func (a *App) renderIngredientError(w http.ResponseWriter, r *http.Request, form ingredientForm, msg string) {
	user := userFromContext(r.Context())
	cats, _ := a.Categories.ListCategories(r.Context(), user.ID)
	allergens, _ := a.Categories.ListAllergens(r.Context())
	data := ingredientsNewData{
		authPageData:      a.baseData(r),
		Form:              form,
		Error:             msg,
		Categories:        cats,
		Allergens:         allergens,
		SelectedAllergens: allergenIDSet(form.AllergenIDs),
	}
	if err := a.Render.Render(w, http.StatusUnprocessableEntity, "ingredient_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

// parseOptionalFloat analoga a parseNonNegFloat ma ritorna nil invece di
// errore se s e' vuota. Usata per micronutrienti opzionali.
func parseOptionalFloat(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := parseNonNegFloat(s)
	if err != nil {
		return nil
	}
	return &v
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

