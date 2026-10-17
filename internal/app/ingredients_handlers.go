package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/off"
	"github.com/amon2126/nutreek/internal/web"
)

// ---- view models -----------------------------------------------------------

// ingredientRow is a row of the result list: either an ingredient already in
// the DB or an OFF product that can be imported.
type ingredientRow struct {
	Local *domain.Ingredient // nil when the row is an OFF product not imported yet
	OFF   *off.Product       // nil when the row is already in the DB

	// Allergens and Category are filled (in batch) only for local rows:
	// the templates show them as chips in the picker and the list.
	Allergens []domain.AllergenTag
	Category  *domain.IngredientCategory

	// CanEdit says whether the viewer may edit the ingredient's data;
	// Author is the name of whoever created it, shown when it is someone else (#143).
	CanEdit bool
	Author  string
}

// IsOFF is used by the template to decide whether to show the Import button.
func (r ingredientRow) IsOFF() bool { return r.Local == nil && r.OFF != nil }

// Name normalizes the name for the template.
func (r ingredientRow) Name() string {
	if r.Local != nil {
		return r.Local.Name
	}
	if r.OFF != nil {
		return r.OFF.Name
	}
	return ""
}

// Brand normalizes the brand for the template.
func (r ingredientRow) Brand() string {
	if r.Local != nil && r.Local.Brand != nil {
		return *r.Local.Brand
	}
	if r.OFF != nil {
		return r.OFF.Brand
	}
	return ""
}

// Kcal/Protein/Carbs/Fat/Unit expose the macros to the template.
// Go templates cannot unpack multiple returns, hence one getter per field.
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

// LocalID returns the DB ID of the local ingredient (0 when the row is OFF only).
func (r ingredientRow) LocalID() int64 {
	if r.Local != nil {
		return r.Local.ID
	}
	return 0
}

// Barcode exposes the OFF identifier for the import form.
func (r ingredientRow) Barcode() string {
	if r.OFF != nil {
		return r.OFF.Code
	}
	return ""
}

// Importable says whether the OFF row can be offered for import. Always true
// for OFF rows from the local FTS: OFF's exported dump has incomplete
// nutrients for nearly every Italian product (volunteers entering only name
// and barcode), but the actual import goes through GetByBarcode on the live
// API, which has up-to-date data. The final "are the macros complete?" check
// therefore happens in the import handler, not here.
func (r ingredientRow) Importable() bool {
	return r.OFF != nil
}

// HasFullNutrients tells the template whether to show the macros or a
// placeholder. Local rows always have complete macros (validated on create).
// OFF rows from the dump often do not (for Italian products the dump is
// nearly always empty); then the template shows a hint instead of a
// misleading 0/0/0/0.
func (r ingredientRow) HasFullNutrients() bool {
	if r.Local != nil {
		return true
	}
	return r.OFF != nil && r.OFF.HasFullNutrients
}

// ImageURL exposes the OFF thumbnail URL, only on the host the CSP allows
// (off.ThumbURL). When the dump has no image_url (products imported before
// migration 0012) or one that cannot be shown, it builds a best-effort CDN URL
// from the barcode: when that does not exist, the template shows the icon.
func (r ingredientRow) ImageURL() string {
	if r.OFF == nil {
		return ""
	}
	if u := off.ThumbURL(r.OFF.ImageURL); u != "" {
		return u
	}
	return off.ThumbURL(off.OFFCDNImageURL(r.OFF.Code))
}

// DefaultQuantity returns the default quantity for the picker (0 = not set).
func (r ingredientRow) DefaultQuantity() float64 {
	if r.Local != nil && r.Local.DefaultQuantity != nil {
		return *r.Local.DefaultQuantity
	}
	return 0
}

// UnitWeightG exposes the average weight per unit when known (OFF parsing or
// a manual override). It returns 0 when unknown: the template uses it to
// decide whether to show the g/pz selector in the picker.
func (r ingredientRow) UnitWeightG() float64 {
	if r.Local != nil && r.Local.UnitWeightG != nil {
		return *r.Local.UnitWeightG
	}
	if r.OFF != nil && r.OFF.UnitWeightG != nil {
		return *r.OFF.UnitWeightG
	}
	return 0
}

// fromOFF says whether the data comes from OpenFoodFacts: a row not imported
// or an imported ingredient. For these the weight of a "piece" is the
// package's (or the piece's, in multipacks like "6 × 25 g"), and the default
// quantity is the OFF serving in g/ml.
func (r ingredientRow) fromOFF() bool {
	if r.Local != nil {
		return r.Local.Source == domain.IngredientSourceOFF
	}
	return r.OFF != nil
}

// ServingG is the OFF serving in g/ml (0 when unknown): for an import it is
// the default quantity stored by CreateOFF.
func (r ingredientRow) ServingG() float64 {
	if !r.fromOFF() {
		return 0
	}
	if r.Local == nil {
		if r.OFF.ServingG != nil {
			return *r.OFF.ServingG
		}
		return 0
	}
	return r.DefaultQuantity()
}

// PickQuantity and PickUnit are the quantity and unit proposed in the picker
// (#136). The quantity comes only from real data:
//   - OFF product: the serving in g/ml, or an empty field (0) when unknown;
//     never "1 pz", which for OFF means the whole package;
//   - ingredient created by hand: the default quantity (up to 10 read as
//     pieces when a piece weight exists, above that as g/ml), otherwise 1 pz
//     when the piece is known, otherwise an empty field.
func (r ingredientRow) PickQuantity() float64 {
	if r.fromOFF() {
		return r.ServingG()
	}
	if q := r.DefaultQuantity(); q > 0 {
		return q
	}
	if r.UnitWeightG() > 0 {
		return 1
	}
	return 0
}

// PickUnit: see PickQuantity. "pz" only for ingredients created by hand.
func (r ingredientRow) PickUnit() string {
	if !r.fromOFF() && r.UnitWeightG() > 0 && r.PickQuantity() <= 10 {
		return "pz"
	}
	return r.Unit()
}

// HasMicronutrients returns true when the local row has at least one micronutrient.
func (r ingredientRow) HasMicronutrients() bool {
	if r.Local == nil {
		return false
	}
	return r.Local.FiberPer100 != nil || r.Local.SugarsPer100 != nil ||
		r.Local.SaturatedFatPer100 != nil || r.Local.SodiumPer100 != nil
}

// FiberPer100Val returns fiber per 100g/ml (0 when not available).
func (r ingredientRow) FiberPer100Val() float64 {
	if r.Local != nil && r.Local.FiberPer100 != nil {
		return *r.Local.FiberPer100
	}
	return 0
}

// SugarsPer100Val returns sugars per 100g/ml (0 when not available).
func (r ingredientRow) SugarsPer100Val() float64 {
	if r.Local != nil && r.Local.SugarsPer100 != nil {
		return *r.Local.SugarsPer100
	}
	return 0
}

// SaturatedFatPer100Val returns saturated fat per 100g/ml (0 when not available).
func (r ingredientRow) SaturatedFatPer100Val() float64 {
	if r.Local != nil && r.Local.SaturatedFatPer100 != nil {
		return *r.Local.SaturatedFatPer100
	}
	return 0
}

// SodiumPer100Val returns sodium per 100g/ml (0 when not available).
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
	OFFWarning string // filled when the local OFF search failed (rare)

	// Filters of the /ingredients page (not used by the pickers).
	Filter        ingredientFilter
	AllAllergens  []domain.AllergenTag
	AllCategories []domain.IngredientCategory

	// Picker context fields. When the partial is rendered in "picker" mode
	// (e.g. inside the recipe-edit page or in a meal's card), these fields
	// drive the form action and its parameters.
	// PickerKind = "" -> display mode (the /ingredients page), no "Add"
	// button (only "Import" on OFF rows).
	PickerKind        string          // "recipe_item" | "meal_item" | "nutritionist_item" | ""
	RecipeID          int64           // when PickerKind == "recipe_item"
	MealID            int64           // when PickerKind == "meal_item"
	VariationID       int64           // when PickerKind == "nutritionist_item"
	WeekStart         string          // YYYY-MM-DD, for the redirect after adding a meal_item
	InDialog          bool            // picker in the dialog over the week (HTMX form)
	SearchedOFF       bool            // the search also reached the OFF network (off=1)
	NutritionistItems []ingredientRow // suggestions from the nutrition plan
}

type ingredientsNewData struct {
	authPageData
	Form              ingredientForm
	Error             string
	Categories        []domain.IngredientCategory
	Allergens         []domain.AllergenTag
	SelectedAllergens map[int64]bool
	ReadOnly          bool // always false: the ingredient_fields partial reads it
}

// ingredientForm captures the fields of the "new/edit ingredient" form so it
// can be rendered again after an error without losing what was entered.
type ingredientForm struct {
	Name            string
	Brand           string
	Unit            string
	Kcal            string
	Protein         string
	Carbs           string
	Fat             string
	UnitWeightG     string  // optional: weight per unit in g (e.g. "150")
	DefaultQuantity string  // optional: default quantity in the picker
	CategoryID      string  // optional: category ID
	Barcode         string  // optional: code read by the scanner (quick "New ingredient" only)
	AllergenIDs     []int64 // optional: IDs of the selected allergens
	// Optional micronutrients (per 100 g/ml)
	Fiber        string
	Sugars       string
	SaturatedFat string
	Sodium       string
}

// ---- handlers --------------------------------------------------------------

func (a *App) handleIngredientsList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	f := parseIngredientFilter(r)
	// Initial SSR render: include OFF only when the user arrives with a query
	// already in the URL (shared link, refresh). Without q, local rows only.
	rows, warn, err := a.searchMixed(r.Context(), q, q != "" && f.WantsOFF())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	user := userFromContext(r.Context())
	allergens, _ := a.Categories.ListAllergens(r.Context())
	categories, _ := a.Categories.ListCategories(r.Context(), user.ID)
	data := ingredientsListData{
		authPageData:  a.baseData(r),
		Rows:          f.apply(rows),
		Query:         q,
		OFFWarning:    warn,
		Filter:        f,
		AllAllergens:  allergens,
		AllCategories: categories,
	}
	if err := a.Render.Render(w, r, http.StatusOK, "ingredients_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleIngredientsSearchPartial answers HTMX with just the results partial.
// Used by the live-search input.
// Param `off=1` includes OpenFoodFacts; without it, the local DB only (fast).
func (a *App) handleIngredientsSearchPartial(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	f := parseIngredientFilter(r)
	rows, warn, err := a.searchMixed(r.Context(), q, includeOFFParam(r) && f.WantsOFF())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// The partial needs the CSRF token for the Import form.
	data := ingredientsListData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		Rows:         f.apply(rows),
		Query:        q,
		OFFWarning:   warn,
		Filter:       f,
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "ingredients_list", "ingredient_results", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ingredientFilter holds the filters of the /ingredients page: source
// (Src: "" all, "mine" in the database, "off" only OpenFoodFacts products not
// imported), category and allergen (0 = no filter).
type ingredientFilter struct {
	Src        string
	CategoryID int64
	AllergenID int64
}

func parseIngredientFilter(r *http.Request) ingredientFilter {
	q := r.URL.Query()
	f := ingredientFilter{Src: q.Get("src")}
	if f.Src != "mine" && f.Src != "off" {
		f.Src = ""
	}
	if id, err := strconv.ParseInt(q.Get("category"), 10, 64); err == nil && id > 0 {
		f.CategoryID = id
	}
	if id, err := strconv.ParseInt(q.Get("allergen"), 10, 64); err == nil && id > 0 {
		f.AllergenID = id
	}
	return f
}

// WantsOFF says whether to search OpenFoodFacts too: products not imported
// have no category or allergens, so those filters exclude them anyway.
func (f ingredientFilter) WantsOFF() bool {
	return f.Src != "mine" && f.CategoryID == 0 && f.AllergenID == 0
}

func (f ingredientFilter) values() url.Values {
	v := url.Values{}
	if f.Src != "" {
		v.Set("src", f.Src)
	}
	if f.CategoryID > 0 {
		v.Set("category", strconv.FormatInt(f.CategoryID, 10))
	}
	if f.AllergenID > 0 {
		v.Set("allergen", strconv.FormatInt(f.AllergenID, 10))
	}
	return v
}

// Query is the filter parameters (without q) for the live search.
func (f ingredientFilter) Query() string { return f.values().Encode() }

// Href is the link to the list with the search q and these filters. It is
// built here because html/template would encode the & and = of an interpolated query.
func (f ingredientFilter) Href(q string) template.URL {
	v := f.values()
	if q != "" {
		v.Set("q", q)
	}
	if len(v) == 0 {
		return "/ingredients"
	}
	return template.URL("/ingredients?" + v.Encode())
}

// WithSrc and WithoutTags derive the filters of the links (source, reset).
func (f ingredientFilter) WithSrc(src string) ingredientFilter { f.Src = src; return f }
func (f ingredientFilter) WithoutTags() ingredientFilter {
	f.CategoryID, f.AllergenID = 0, 0
	return f
}

// apply keeps the rows that pass every filter.
func (f ingredientFilter) apply(rows []ingredientRow) []ingredientRow {
	out := make([]ingredientRow, 0, len(rows))
	for _, row := range rows {
		switch {
		case f.Src == "mine" && row.IsOFF(), f.Src == "off" && !row.IsOFF():
			continue
		case f.CategoryID > 0 && (row.Category == nil || row.Category.ID != f.CategoryID):
			continue
		case f.AllergenID > 0 && !hasAllergen(row.Allergens, f.AllergenID):
			continue
		}
		out = append(out, row)
	}
	return out
}

func hasAllergen(tags []domain.AllergenTag, id int64) bool {
	for _, t := range tags {
		if t.ID == id {
			return true
		}
	}
	return false
}

// includeOFFParam returns true when the client explicitly asked for the
// OpenFoodFacts integration (param off=1). Default false: the local DB
// answers every keystroke, OFF is called only once typing stops, to avoid
// partial queries.
func includeOFFParam(r *http.Request) bool {
	v := r.URL.Query().Get("off")
	return v == "1" || v == "true"
}

// searchMixed runs the local search and, when includeOFF==true and q != "",
// merges the OpenFoodFacts results deduplicated by barcode.
// The OFF search is now fully local via SQLite FTS5 (filled by the Syncer
// from the public dump), so <50ms, no timeouts, no 503s.
// `warn` stays in the interface to handle any SQL errors but in practice it
// is always empty.
func (a *App) searchMixed(ctx context.Context, q string, includeOFF bool) ([]ingredientRow, string, error) {
	if code, ok := off.NormalizeBarcode(q); ok {
		return a.searchBarcode(ctx, code, includeOFF)
	}
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

	a.attachTags(ctx, rows)

	if !includeOFF || q == "" {
		return rows, "", nil
	}

	products, offErr := a.OFF.SearchByName(ctx, q)
	if offErr != nil {
		a.Logger.Warn("OFF search failed", "err", offErr, "q", q)
		return rows, i18n.T(i18n.FromContext(ctx), "ingredients.off.search_unavailable"), nil
	}
	// FTS5 does a native AND across tokens + prefix match: no extra filter needed.
	for i := range products {
		p := products[i]
		if knownBarcodes[p.Code] {
			continue // already imported
		}
		rows = append(rows, ingredientRow{OFF: &p})
	}
	return rows, "", nil
}

// searchBarcode is the search by barcode (the picker's scanner or digits
// typed by hand): at most one row. First the user's own ingredients (OFF
// imports or local ones linked to the code), then the local copy of OFF; the
// network only with live=true, that is once typing stops. No row = not found.
func (a *App) searchBarcode(ctx context.Context, code string, live bool) ([]ingredientRow, string, error) {
	ing, err := a.Ingredients.GetByBarcode(ctx, code)
	if err == nil {
		rows := []ingredientRow{{Local: ing}}
		a.attachTags(ctx, rows)
		return rows, "", nil
	}
	if !errors.Is(err, db.ErrIngredientNotFound) {
		return nil, "", err
	}
	if a.OFF == nil {
		return nil, "", nil
	}
	p, err := a.OFF.FindByBarcode(ctx, code, live)
	switch {
	case errors.Is(err, off.ErrNotFound):
		return nil, "", nil
	case errors.Is(err, off.ErrUnavailable):
		a.Logger.Warn("OFF barcode lookup failed", "err", err, "barcode", code)
		return nil, i18n.T(i18n.FromContext(ctx), "ingredients.off.barcode_unavailable"), nil
	case err != nil:
		return nil, "", err
	}
	return []ingredientRow{{OFF: p}}, "", nil
}

// attachTags loads in batch the allergens and the category (the viewer's) of
// the local rows and assigns them in place, so the picker and the list can
// show them without N+1 queries; it also marks who may edit them and whose
// they are. OFF rows not imported have neither allergens nor a category.
func (a *App) attachTags(ctx context.Context, rows []ingredientRow) {
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
	viewer := userFromContext(ctx)
	byIng, err := a.Categories.AllergensForIngredients(ctx, ids)
	if err != nil {
		a.Logger.Warn("attach allergens failed", "err", err)
	}
	var cats map[int64]domain.IngredientCategory
	if viewer != nil {
		cats, err = a.Categories.CategoriesForIngredients(ctx, viewer.ID, ids)
		if err != nil {
			a.Logger.Warn("attach categories failed", "err", err)
		}
	}
	authors := map[int64]string{}
	for i := range rows {
		if rows[i].Local == nil {
			continue
		}
		rows[i].Allergens = byIng[rows[i].Local.ID]
		if c, ok := cats[rows[i].Local.ID]; ok {
			rows[i].Category = &c
		}
		rows[i].CanEdit = a.canEditIngredient(viewer, rows[i].Local)
		if !rows[i].CanEdit {
			rows[i].Author = a.ingredientAuthor(ctx, rows[i].Local, authors)
		}
	}
}

// canEditIngredient says whether u may edit the ingredient's data (#143):
// whoever created it edits it, or the owner. An ingredient with no author
// (created before #143, or by the reconciler) is edited by the owner; when
// the instance has no owner, by anyone, as before.
func (a *App) canEditIngredient(u *domain.User, ing *domain.Ingredient) bool {
	if u == nil || ing == nil {
		return false
	}
	if a.isOwner(u) {
		return true
	}
	if ing.CreatedBy != nil {
		return *ing.CreatedBy == u.ID
	}
	return a.Cfg.OwnerEmail == ""
}

// ingredientAuthor returns the name of whoever created the ingredient, "" when
// it has no author. cache avoids reading the same user again in a list.
func (a *App) ingredientAuthor(ctx context.Context, ing *domain.Ingredient, cache map[int64]string) string {
	if ing.CreatedBy == nil {
		return ""
	}
	id := *ing.CreatedBy
	if name, ok := cache[id]; ok {
		return name
	}
	name := ""
	if u, err := a.Users.GetByID(ctx, id); err == nil {
		name = u.DisplayName
	}
	if cache != nil {
		cache[id] = name
	}
	return name
}

// ownCategory parses a form's category_id: nil when empty, an error when it
// is not one of userID's categories (an invented id or another user's too).
func (a *App) ownCategory(ctx context.Context, userID int64, raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, db.ErrCategoryNotFound
	}
	cats, err := a.Categories.ListCategories(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range cats {
		if c.ID == id {
			return &id, nil
		}
	}
	return nil, db.ErrCategoryNotFound
}

// allergenIDSet turns a slice of IDs into a set for rendering the "selected"
// checkboxes in the ingredient form.
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
	if err := a.Render.Render(w, r, http.StatusOK, "ingredient_new", data); err != nil {
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
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Brand:       strings.TrimSpace(r.PostFormValue("brand")),
		Unit:        strings.TrimSpace(r.PostFormValue("unit")),
		Kcal:        strings.TrimSpace(r.PostFormValue("kcal")),
		Protein:     strings.TrimSpace(r.PostFormValue("protein")),
		Carbs:       strings.TrimSpace(r.PostFormValue("carbs")),
		Fat:         strings.TrimSpace(r.PostFormValue("fat")),
		UnitWeightG: strings.TrimSpace(r.PostFormValue("unit_weight_g")),
		// DefaultQuantity only serves to show the value again after an error.
		DefaultQuantity: strings.TrimSpace(r.PostFormValue("default_quantity")),
		CategoryID:      strings.TrimSpace(r.PostFormValue("category_id")),
		AllergenIDs:     parsedAllergenIDs,
		Fiber:           strings.TrimSpace(r.PostFormValue("fiber")),
		Sugars:          strings.TrimSpace(r.PostFormValue("sugars")),
		SaturatedFat:    strings.TrimSpace(r.PostFormValue("saturated_fat")),
		Sodium:          strings.TrimSpace(r.PostFormValue("sodium")),
	}

	vals, msg := a.validateIngredient(r, form)
	if msg != "" {
		a.renderIngredientError(w, r, form, msg)
		return
	}

	var brandPtr *string
	if form.Brand != "" {
		brandPtr = &form.Brand
	}
	user := userFromContext(r.Context())
	var creatorID *int64
	var categoryID *int64
	if user != nil {
		id := user.ID
		creatorID = &id
		var err error
		if categoryID, err = a.ownCategory(r.Context(), user.ID, form.CategoryID); err != nil {
			a.renderIngredientError(w, r, form, a.t(r, "ingredients.error.category"))
			return
		}
	}

	// optional unit_weight_g: weight per unit (e.g. 150 for a 150g banana).
	var unitWeightG *float64
	if s := strings.TrimSpace(r.PostFormValue("unit_weight_g")); s != "" {
		if v, err2 := parseNonNegFloat(s); err2 == nil && v > 0 {
			unitWeightG = &v
		}
	}

	// optional default_quantity: the default quantity in the picker.
	var defaultQuantity *float64
	if s := strings.TrimSpace(r.PostFormValue("default_quantity")); s != "" {
		if v, err2 := parseNonNegFloat(s); err2 == nil && v > 0 {
			defaultQuantity = &v
		}
	}

	ingID, err := a.Ingredients.CreateLocal(r.Context(), form.Name, brandPtr, vals.Unit, vals.Kcal, vals.Protein, vals.Carbs, vals.Fat, creatorID, unitWeightG, defaultQuantity,
		parseOptionalFloat(form.Fiber), parseOptionalFloat(form.Sugars),
		parseOptionalFloat(form.SaturatedFat), parseOptionalFloat(form.Sodium))
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	// Save the category (optional), the creator's.
	if categoryID != nil {
		_ = a.Categories.SetIngredientCategory(r.Context(), user.ID, ingID, categoryID)
	}
	// Save the allergens (optional).
	if len(form.AllergenIDs) > 0 {
		_ = a.Categories.SetIngredientAllergens(r.Context(), ingID, form.AllergenIDs)
	}

	a.Sessions.Put(r.Context(), "flash", "ingredients.flash.created")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}

// handleIngredientOFFImport imports by barcode from the "Importa" button.
// Idempotent: when already in the DB, an "already there" flash with no error.
func (a *App) handleIngredientOFFImport(w http.ResponseWriter, r *http.Request) {
	barcode := strings.TrimSpace(r.PostFormValue("barcode"))
	if barcode == "" {
		http.Error(w, a.t(r, "ingredients.error.barcode_missing"), http.StatusBadRequest)
		return
	}

	// Pre-check: already imported?
	existing, err := a.Ingredients.GetByBarcode(r.Context(), barcode)
	if err != nil && !errors.Is(err, db.ErrIngredientNotFound) {
		a.serverError(w, r, err)
		return
	}
	if existing != nil {
		a.Sessions.Put(r.Context(), "flash", "ingredients.flash.already_imported")
		http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
		return
	}

	product, err := a.OFF.GetByBarcode(r.Context(), barcode)
	if err != nil {
		if errors.Is(err, off.ErrNotFound) {
			http.Error(w, a.t(r, "ingredients.off.not_found"), http.StatusNotFound)
			return
		}
		a.Logger.Error("OFF GetByBarcode failed", "err", err, "barcode", barcode)
		http.Error(w, a.t(r, "ingredients.off.unreachable"), http.StatusBadGateway)
		return
	}
	if !product.HasFullNutrients {
		// The live API lacks the 4 macros for this barcode too (OFF data
		// genuinely incomplete). Redirect with a flash instead of a raw error:
		// the user can create a local ingredient by hand.
		a.Sessions.Put(r.Context(), "flash", "ingredients.flash.off_incomplete")
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
		importerID(r.Context()),
		product.UnitWeightG, product.ServingG,
		product.Fiber, product.Sugars, product.SaturatedFat, product.Sodium,
	); err != nil && !errors.Is(err, db.ErrBarcodeAlreadyUsed) {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "ingredients.flash.imported")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}

// resolveOFFBarcode imports the OFF product identified by the barcode into
// the local DB and returns the id of the matching local ingredient.
// Idempotent: when the barcode is already in the DB, it returns the existing
// one without calling OFF again. When OFF lacks some nutrients, it returns an error.
//
// Used both by handleRecipeAddItem (auto-import on click) and (in stage 16)
// by the meal_item flow, to avoid duplicating the logic.
func (a *App) resolveOFFBarcode(ctx context.Context, barcode string) (int64, error) {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return 0, errEmpty
	}
	// DB pre-check: avoids an HTTP call when already imported.
	existing, err := a.Ingredients.GetByBarcode(ctx, barcode)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, db.ErrIngredientNotFound) {
		return 0, err
	}
	// Not among the ingredients: OFF's API, which brings the micronutrients too.
	// When OFF does not answer, the local copy is enough, if it has the macros.
	product, err := a.OFF.GetByBarcode(ctx, barcode)
	if err != nil && !errors.Is(err, off.ErrNotFound) {
		if local, lerr := a.OFF.FindByBarcode(ctx, barcode, false); lerr == nil && local.HasFullNutrients {
			product, err = local, nil
		}
	}
	if err != nil {
		return 0, err
	}
	if !product.HasFullNutrients {
		return 0, errOFFNoMacros
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
		importerID(ctx),
		product.UnitWeightG, product.ServingG,
		product.Fiber, product.Sugars, product.SaturatedFat, product.Sodium,
	)
	if err == nil {
		return id, nil
	}
	if errors.Is(err, db.ErrBarcodeAlreadyUsed) {
		// Race / barcode just created by another path: recover it.
		got, gerr := a.Ingredients.GetByBarcode(ctx, product.Code)
		if gerr != nil {
			return 0, gerr
		}
		return got.ID, nil
	}
	return 0, err
}

// importerID is the author of an imported OFF product: the request's user,
// nil outside an authenticated request.
func importerID(ctx context.Context) *int64 {
	if u := userFromContext(ctx); u != nil {
		id := u.ID
		return &id
	}
	return nil
}

// handleOFFMacroCheck checks the macros of a single OFF product through a
// barcode lookup (FindByBarcode: local off_products, then the network when
// the local record is missing or incomplete).
// Returns an HTML fragment with the macro state: green dot + values, or a
// red dot + "macro non disponibili". Used by ingredient_results.html via an
// HTMX lazy load (hx-trigger="load delay:500ms").
func (a *App) handleOFFMacroCheck(w http.ResponseWriter, r *http.Request) {
	barcode := chi.URLParam(r, "barcode")
	if barcode == "" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	product, err := a.OFF.FindByBarcode(r.Context(), barcode, true)
	if err != nil || !product.HasFullNutrients {
		a.writeMacroFragment(w, r, false, 0, 0, 0, 0)
		return
	}
	a.writeMacroFragment(w, r, true, product.Kcal, product.Protein, product.Carbs, product.Fat)
}

// writeMacroFragment writes the kcal and macros per 100 g of an OFF product
// in the format of the ingredient_results rows.
func (a *App) writeMacroFragment(w http.ResponseWriter, r *http.Request, full bool, kcal, protein, carbs, fat float64) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	esc := func(key string) string { return html.EscapeString(a.t(r, key)) }
	lang := i18n.FromContext(r.Context())
	if full {
		fmt.Fprintf(w,
			`<span class="flex items-center gap-2.5 text-[12.5px] text-muted tabular-nums">`+
				`<span class="text-[14px] text-text"><b>%s</b> kcal</span>`+
				`<span><b class="text-prot font-semibold">%s</b> %s g</span>`+
				`<span><b class="text-carb font-semibold">%s</b> %s g</span>`+
				`<span><b class="text-fat font-semibold">%s</b> %s g</span></span>`,
			web.FormatNum(lang, kcal),
			esc("common.protein_short"), web.FormatDec(lang, protein, 1),
			esc("common.carbs_short"), web.FormatDec(lang, carbs, 1),
			esc("common.fat_short"), web.FormatDec(lang, fat, 1))
	} else {
		fmt.Fprintf(w, `<span class="text-[12.5px] text-muted">%s</span>`, esc("ingredients.row.no_macros"))
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
	// ReadOnly: the viewer may not edit the data, only their own category
	// (#143). Author is the name of whoever created it ("" when there is none).
	ReadOnly bool
	Author   string
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

	form := ingredientFormFrom(ing)
	// Without this preselection, saving cleared the category.
	current, err := a.Categories.CategoriesForIngredients(r.Context(), user.ID, []int64{id})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if c, ok := current[id]; ok {
		form.CategoryID = strconv.FormatInt(c.ID, 10)
	}

	data := ingredientEditData{
		authPageData:      a.baseData(r),
		Form:              form,
		IngredientID:      id,
		IsOFF:             ing.Source == domain.IngredientSourceOFF,
		Categories:        cats,
		Allergens:         allergens,
		SelectedAllergens: selected,
		ReadOnly:          !a.canEditIngredient(user, ing),
	}
	if data.ReadOnly {
		data.Author = a.ingredientAuthor(r.Context(), ing, nil)
	}
	if err := a.Render.Render(w, r, http.StatusOK, "ingredient_edit", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ingredientFormFrom fills the edit form with the saved data.
func ingredientFormFrom(ing *domain.Ingredient) ingredientForm {
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
	return form
}

func (a *App) handleIngredientUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	// Check that it exists.
	ing, err := a.Ingredients.GetByID(r.Context(), id)
	if err != nil {
		a.notFound(w, r)
		return
	}
	user := userFromContext(r.Context())
	if !a.canEditIngredient(user, ing) {
		a.saveIngredientCategoryOnly(w, r, ing)
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
		_ = a.Render.Render(w, r, http.StatusUnprocessableEntity, "ingredient_edit", data)
	}

	vals, msg := a.validateIngredient(r, form)
	if msg != "" {
		renderEditErr(msg)
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
	categoryID, err := a.ownCategory(r.Context(), user.ID, form.CategoryID)
	if err != nil {
		renderEditErr(a.t(r, "ingredients.error.category"))
		return
	}

	if err := a.Ingredients.Update(r.Context(), id, form.Name, brandPtr, vals.Unit, vals.Kcal, vals.Protein, vals.Carbs, vals.Fat, unitWeightG, defaultQuantity,
		parseOptionalFloat(form.Fiber), parseOptionalFloat(form.Sugars),
		parseOptionalFloat(form.SaturatedFat), parseOptionalFloat(form.Sodium)); err != nil {
		a.serverError(w, r, err)
		return
	}

	// Update the category of whoever saves.
	_ = a.Categories.SetIngredientCategory(r.Context(), user.ID, id, categoryID)

	// Update the allergens (replace: an empty list removes every tag).
	_ = a.Categories.SetIngredientAllergens(r.Context(), id, form.AllergenIDs)

	a.Sessions.Put(r.Context(), "flash", "ingredients.flash.updated")
	http.Redirect(w, r, "/ingredients", http.StatusSeeOther)
}

// saveIngredientCategoryOnly is the save of whoever may not edit the
// ingredient (#143): only their category counts, the POST's other fields
// are ignored.
func (a *App) saveIngredientCategoryOnly(w http.ResponseWriter, r *http.Request, ing *domain.Ingredient) {
	user := userFromContext(r.Context())
	raw := strings.TrimSpace(r.PostFormValue("category_id"))
	categoryID, err := a.ownCategory(r.Context(), user.ID, raw)
	if err != nil {
		cats, _ := a.Categories.ListCategories(r.Context(), user.ID)
		allergens, _ := a.Categories.ListAllergens(r.Context())
		selected, _ := a.Categories.GetIngredientAllergenIDs(r.Context(), ing.ID)
		form := ingredientFormFrom(ing)
		form.CategoryID = raw
		data := ingredientEditData{
			authPageData:      a.baseData(r),
			Form:              form,
			Error:             a.t(r, "ingredients.error.category"),
			IngredientID:      ing.ID,
			IsOFF:             ing.Source == domain.IngredientSourceOFF,
			Categories:        cats,
			Allergens:         allergens,
			SelectedAllergens: selected,
			ReadOnly:          true,
			Author:            a.ingredientAuthor(r.Context(), ing, nil),
		}
		_ = a.Render.Render(w, r, http.StatusUnprocessableEntity, "ingredient_edit", data)
		return
	}
	if err := a.Categories.SetIngredientCategory(r.Context(), user.ID, ing.ID, categoryID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "ingredients.flash.category_saved")
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
	if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "ingredient_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ingredientValues are the fields every ingredient form has, parsed.
type ingredientValues struct {
	Unit                      domain.IngredientUnit
	Kcal, Protein, Carbs, Fat float64
}

// validateIngredient checks the name, the unit and the four macros shared by
// the new, edit and quick forms. It returns them parsed, or the message to
// show ("" when the form is valid).
func (a *App) validateIngredient(r *http.Request, form ingredientForm) (ingredientValues, string) {
	var v ingredientValues
	if form.Name == "" {
		return v, a.t(r, "ingredients.error.name_required")
	}
	if !withinLen(form.Name, MaxNameLen) {
		return v, a.t(r, "ingredients.error.name_too_long", "Max", MaxNameLen)
	}
	v.Unit = domain.IngredientUnit(form.Unit)
	if v.Unit != domain.UnitGram && v.Unit != domain.UnitMilliliter {
		return v, a.t(r, "ingredients.error.unit")
	}
	for _, f := range []struct {
		key, raw string
		dst      *float64
	}{
		{"ingredients.error.kcal", form.Kcal, &v.Kcal},
		{"ingredients.error.protein", form.Protein, &v.Protein},
		{"ingredients.error.carbs", form.Carbs, &v.Carbs},
		{"ingredients.error.fat", form.Fat, &v.Fat},
	} {
		x, err := parseNonNegFloat(f.raw)
		if err != nil {
			return v, a.numberError(r, f.key, err)
		}
		*f.dst = x
	}
	return v, ""
}

// parseOptionalFloat is like parseNonNegFloat but returns nil instead of an
// error when s is empty. Used for optional micronutrients.
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

// parseNonNegFloat accepts numbers with a comma or a dot as the decimal
// separator (Italian forms often use the comma).
func parseNonNegFloat(s string) (float64, error) {
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, errEmpty
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errNotNumber
	}
	if v < 0 {
		return 0, errNegative
	}
	return v, nil
}

// errors exposed as vars to give meaningful messages without pkg errors.
var (
	errEmpty     = simpleErr("valore mancante")
	errNegative  = simpleErr("valore negativo")
	errNotNumber = simpleErr("non è un numero")

	// errOFFNoMacros is shown to the user through errText (reasons).
	errOFFNoMacros = simpleErr("prodotto OFF senza macro completi")
)

// numberError is the validation message of a number field that
// parseNonNegFloat rejected: key names the field, err gives the reason.
func (a *App) numberError(r *http.Request, key string, err error) string {
	reason := "ingredients.error.not_a_number"
	switch {
	case errors.Is(err, errEmpty):
		reason = "ingredients.error.missing"
	case errors.Is(err, errNegative):
		reason = "ingredients.error.negative"
	}
	return a.t(r, key, "Reason", a.t(r, reason))
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
