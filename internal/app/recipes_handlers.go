package app

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// ---- view models -----------------------------------------------------------

type recipesListData struct {
	authPageData
	Mine   []recipeListRow
	Others []*domain.Recipe // pubbliche di altri utenti
}

// recipeListRow è una riga della lista "Le mie" con il conteggio degli usi
// nei piani settimanali. Serve a mostrare un warning informato nella conferma
// di eliminazione (es. "Usata in 3 pasti — saranno aggiornati").
type recipeListRow struct {
	Recipe *domain.Recipe
	Usages int
}

type recipeNewData struct {
	authPageData
	Form  recipeForm
	Error string
}

type recipeForm struct {
	Name        string
	Description string
	Servings    string
	IsPublic    bool
}

// recipeDuplicateData è il view-model del modal di duplicazione.
type recipeDuplicateData struct {
	authPageData
	SrcID    int64
	SrcName  string
	NewName  string // valore pre-compilato (default "Copia di X")
	Error    string
}

type recipeEditData struct {
	authPageData
	Recipe       *domain.RecipeWithItems
	Form         recipeForm
	Error        string
	ItemError    string
	CanEdit      bool                  // true se l'utente loggato e' l'autore
	Total        macros.Macros         // totale ricetta
	PerServing   macros.Macros         // per-serving
	Ingredients  []*domain.Ingredient  // pool per il select del nuovo item
	NewItemQty   string                // valore nel form item dopo errore
	NewItemIng   string                // ingredient_id selezionato
}

// ---- list ------------------------------------------------------------------

func (a *App) handleRecipesList(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	all, err := a.Recipes.ListVisible(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	usages, err := a.Recipes.UsagesByRecipe(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := recipesListData{authPageData: a.baseData(r)}
	for _, rec := range all {
		if rec.UserID == user.ID {
			data.Mine = append(data.Mine, recipeListRow{Recipe: rec, Usages: usages[rec.ID]})
		} else {
			data.Others = append(data.Others, rec)
		}
	}
	if err := a.Render.Render(w, http.StatusOK, "recipes_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- new -------------------------------------------------------------------

func (a *App) handleRecipeNewForm(w http.ResponseWriter, r *http.Request) {
	data := recipeNewData{
		authPageData: a.baseData(r),
		Form:         recipeForm{Servings: "1"},
	}
	if err := a.Render.Render(w, http.StatusOK, "recipe_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleRecipeCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	form := recipeForm{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Servings:    strings.TrimSpace(r.PostFormValue("servings")),
		IsPublic:    r.PostFormValue("is_public") == "on",
	}
	if form.Name == "" {
		a.renderRecipeNewError(w, r, form, "Il nome e' obbligatorio.")
		return
	}
	if !withinLen(form.Name, MaxNameLen) {
		a.renderRecipeNewError(w, r, form, "Nome troppo lungo (max 200 caratteri).")
		return
	}
	if !withinLen(form.Description, MaxDescriptionLen) {
		a.renderRecipeNewError(w, r, form, "Descrizione troppo lunga (max 2000 caratteri).")
		return
	}
	servings, err := strconv.Atoi(form.Servings)
	if err != nil || servings <= 0 {
		a.renderRecipeNewError(w, r, form, "Porzioni: serve un intero > 0.")
		return
	}
	var descPtr *string
	if form.Description != "" {
		descPtr = &form.Description
	}
	id, err := a.Recipes.Create(r.Context(), user.ID, form.Name, descPtr, servings, form.IsPublic)
	if err != nil {
		if errors.Is(err, db.ErrRecipeNameTaken) {
			a.renderRecipeNewError(w, r, form, "Esiste già una tua ricetta con questo nome.")
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta creata.")
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) renderRecipeNewError(w http.ResponseWriter, r *http.Request, form recipeForm, msg string) {
	data := recipeNewData{authPageData: a.baseData(r), Form: form, Error: msg}
	if err := a.Render.Render(w, http.StatusUnprocessableEntity, "recipe_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

// ---- edit / detail ---------------------------------------------------------

func (a *App) handleRecipeEdit(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeEdit(w, r, rec, "", "", "", "")
}

func (a *App) renderRecipeEdit(
	w http.ResponseWriter, r *http.Request,
	rec *domain.RecipeWithItems,
	formError, itemError, newItemQty, newItemIng string,
) {
	user := userFromContext(r.Context())
	canEdit := user != nil && user.ID == rec.UserID

	// Calcolo macro: somma item per item, poi per-serving.
	items := make([]macros.IngredientItem, 0, len(rec.Items))
	for _, it := range rec.Items {
		items = append(items, macros.IngredientItem{Ingredient: it.Ingredient, Quantity: it.Quantity})
	}
	total := macros.ForRecipe(items)

	desc := ""
	if rec.Description != nil {
		desc = *rec.Description
	}
	data := recipeEditData{
		authPageData: a.baseData(r),
		Recipe:       rec,
		Form: recipeForm{
			Name:        rec.Name,
			Description: desc,
			Servings:    strconv.Itoa(rec.Servings),
			IsPublic:    rec.IsPublic,
		},
		Error:      formError,
		ItemError:  itemError,
		CanEdit:    canEdit,
		Total:      total,
		PerServing: macros.PerServing(total, rec.Servings),
		NewItemQty: newItemQty,
		NewItemIng: newItemIng,
	}
	if canEdit {
		// Carichiamo gli ingredienti per il select solo se l'utente puo' editare.
		ings, err := a.Ingredients.ListActive(r.Context(), 0, 200)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Ingredients = ings
	}
	status := http.StatusOK
	if formError != "" || itemError != "" {
		status = http.StatusUnprocessableEntity
	}
	if err := a.Render.Render(w, status, "recipe_edit", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleRecipeUpdate gestisce POST /recipes/{id} (modifica meta).
func (a *App) handleRecipeUpdate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	form := recipeForm{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Servings:    strings.TrimSpace(r.PostFormValue("servings")),
		IsPublic:    r.PostFormValue("is_public") == "on",
	}
	if form.Name == "" {
		a.reRenderEdit(w, r, id, "Il nome e' obbligatorio.", "")
		return
	}
	if !withinLen(form.Name, MaxNameLen) {
		a.reRenderEdit(w, r, id, "Nome troppo lungo (max 200 caratteri).", "")
		return
	}
	if !withinLen(form.Description, MaxDescriptionLen) {
		a.reRenderEdit(w, r, id, "Descrizione troppo lunga (max 2000 caratteri).", "")
		return
	}
	servings, err := strconv.Atoi(form.Servings)
	if err != nil || servings <= 0 {
		a.reRenderEdit(w, r, id, "Porzioni: serve un intero > 0.", "")
		return
	}
	var descPtr *string
	if form.Description != "" {
		descPtr = &form.Description
	}
	if err := a.Recipes.Update(r.Context(), id, user.ID, form.Name, descPtr, servings, form.IsPublic); err != nil {
		if errors.Is(err, db.ErrRecipeNameTaken) {
			a.reRenderEdit(w, r, id, "Esiste già una tua ricetta con questo nome.", "")
			return
		}
		a.handleRecipeError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta aggiornata.")
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipeAddItem POST /recipes/{id}/items. Accetta uno fra:
//   - ingredient_id: id di un ingrediente gia' nel DB locale.
//   - barcode: codice OFF di un prodotto NON ancora importato; il server
//     fa auto-import (idempotente: se gia' presente recupera l'esistente).
// La distinzione viene fatta dal partial picker_results in base alla
// natura della riga cliccata.
func (a *App) handleRecipeAddItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	qtyStr := strings.TrimSpace(r.PostFormValue("quantity"))
	qty, err := parseNonNegFloat(qtyStr)
	if err != nil || qty <= 0 {
		a.reRenderEditItem(w, r, id, "Quantita' non valida.", qtyStr, r.PostFormValue("ingredient_id"))
		return
	}

	// Risolvi l'ingrediente target. Due strade:
	barcode := strings.TrimSpace(r.PostFormValue("barcode"))
	var ingID int64
	if barcode != "" {
		// Auto-import OFF idempotente.
		resolved, err := a.resolveOFFBarcode(r.Context(), barcode)
		if err != nil {
			a.reRenderEditItem(w, r, id, "Import OFF fallito: "+err.Error(), qtyStr, "")
			return
		}
		ingID = resolved
	} else {
		ingIDStr := r.PostFormValue("ingredient_id")
		parsed, err := strconv.ParseInt(ingIDStr, 10, 64)
		if err != nil || parsed <= 0 {
			a.reRenderEditItem(w, r, id, "Seleziona un ingrediente.", qtyStr, ingIDStr)
			return
		}
		ingID = parsed
	}

	if _, err := a.Recipes.AddItem(r.Context(), id, user.ID, ingID, qty); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipePicker GET /recipes/{id}/picker?q=...
// Renderizza il partial picker_results in modalita' recipe_item.
// Usa lo stesso searchMixed della pagina /ingredients (locale + OFF).
func (a *App) handleRecipePicker(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	// Verifica che l'utente sia autore (solo l'autore puo' editare).
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	if rec.UserID != user.ID {
		http.Error(w, "non sei l'autore", http.StatusForbidden)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
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
		PickerKind:   "recipe_item",
		RecipeID:     id,
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "ingredients_list", "picker_results", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleRecipeRemoveItem POST /recipes/{id}/items/{itemID}/delete (form-driven).
// HTMX swap della pagina: redirect 303 alla edit page.
func (a *App) handleRecipeRemoveItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.Recipes.RemoveItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrRecipeItemNotFound) {
			http.Error(w, "voce non trovata", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/recipes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleRecipeDelete POST /recipes/{id}/delete. Conta le occorrenze nei
// piani settimanali prima della cancellazione, perché la FK ON DELETE CASCADE
// (migrazione 0008) le rimuove silenziosamente: il flash informa l'utente.
func (a *App) handleRecipeDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	usages, _ := a.Recipes.CountMealUsages(r.Context(), id)
	if err := a.Recipes.Delete(r.Context(), id, user.ID); err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	msg := "Ricetta eliminata."
	if usages > 0 {
		msg = fmt.Sprintf("Ricetta eliminata. Rimossa anche da %d %s nei piani settimanali.",
			usages, pluralize(usages, "voce", "voci"))
	}
	a.Sessions.Put(r.Context(), "flash", msg)
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

// handleRecipesBulkDelete POST /recipes/bulk-delete. Cancella più ricette
// in una sola chiamata. Le ricette non di proprietà dell'utente vengono
// silenziosamente ignorate (BulkDelete filtra per user_id).
func (a *App) handleRecipesBulkDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form invalido", http.StatusBadRequest)
		return
	}
	raw := r.PostForm["recipe_id"]
	ids := make([]int64, 0, len(raw))
	for _, s := range raw {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		a.Sessions.Put(r.Context(), "flash", "Nessuna ricetta selezionata.")
		http.Redirect(w, r, "/recipes", http.StatusSeeOther)
		return
	}
	usages, _ := a.Recipes.CountMealUsagesMulti(r.Context(), ids)
	n, err := a.Recipes.BulkDelete(r.Context(), ids, user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	var msg string
	switch {
	case n == 0:
		msg = "Nessuna ricetta eliminata."
	case n == 1:
		msg = "1 ricetta eliminata."
	default:
		msg = fmt.Sprintf("%d ricette eliminate.", n)
	}
	if usages > 0 && n > 0 {
		msg += fmt.Sprintf(" Rimosse %d %s nei piani settimanali.",
			usages, pluralize(usages, "voce", "voci"))
	}
	a.Sessions.Put(r.Context(), "flash", msg)
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}

// handleRecipeDuplicateForm GET /recipes/{id}/duplicate
// Renderizza il modal con il campo "nuovo nome" pre-compilato a "Copia di X".
func (a *App) handleRecipeDuplicateForm(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	src, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeDuplicateModal(w, r, recipeDuplicateData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		SrcID:        src.ID,
		SrcName:      src.Name,
		NewName:      proposeDuplicateName(src.Name),
	})
}

// handleRecipeDuplicate POST /recipes/{id}/duplicate
// Valida il nuovo nome (obbligatorio, diverso dall'originale, unico) e crea
// una copia profonda. Risponde con HX-Redirect alla edit della nuova ricetta.
func (a *App) handleRecipeDuplicate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	srcID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	newName := strings.TrimSpace(r.PostFormValue("new_name"))

	renderErr := func(msg string) {
		a.renderRecipeDuplicateFormOnly(w, r, recipeDuplicateData{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			SrcID:        srcID,
			NewName:      newName,
			Error:        msg,
		})
	}

	if newName == "" {
		renderErr("Inserisci un nome per la copia.")
		return
	}
	if !withinLen(newName, MaxNameLen) {
		renderErr("Nome troppo lungo (max 200 caratteri).")
		return
	}
	// Verifica esplicita: nome uguale all'originale è bloccato a prescindere
	// dal vincolo UNIQUE — messaggio dedicato.
	src, err := a.Recipes.GetWithItems(r.Context(), srcID, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	if strings.EqualFold(newName, src.Name) {
		renderErr("Il nome della copia deve essere diverso dall'originale.")
		return
	}

	newID, err := a.Recipes.Duplicate(r.Context(), srcID, user.ID, newName)
	if err != nil {
		if errors.Is(err, db.ErrRecipeNameTaken) {
			renderErr("Esiste già una tua ricetta con questo nome.")
			return
		}
		a.handleRecipeError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Ricetta duplicata. Modifica gli ingredienti per personalizzarla.")
	// HTMX rispetta HX-Redirect e fa un full reload alla URL specificata.
	w.Header().Set("HX-Redirect", "/recipes/"+strconv.FormatInt(newID, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) renderRecipeDuplicateModal(w http.ResponseWriter, r *http.Request, data recipeDuplicateData) {
	mdata := modalDialogData{
		Title:     "Duplica ricetta",
		Inner:     "recipe_duplicate_form",
		InnerData: data,
	}
	if err := a.Render.RenderPartial(w, http.StatusOK, "recipes_list", "modal_dialog", mdata); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) renderRecipeDuplicateFormOnly(w http.ResponseWriter, r *http.Request, data recipeDuplicateData) {
	if err := a.Render.RenderPartial(w, http.StatusUnprocessableEntity, "recipes_list", "recipe_duplicate_form", data); err != nil {
		a.serverError(w, r, err)
	}
}

// proposeDuplicateName ritorna un nome plausibile per la copia. Se l'utente
// poi conferma e quel nome è già preso, il vincolo UNIQUE lo riporta indietro
// con errore esplicito.
func proposeDuplicateName(name string) string {
	return "Copia di " + name
}

// pluralize sceglie singolare/plurale in base al conteggio.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// ---- helpers ---------------------------------------------------------------

func (a *App) reRenderEdit(w http.ResponseWriter, r *http.Request, id int64, formErr, itemErr string) {
	user := userFromContext(r.Context())
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeEdit(w, r, rec, formErr, itemErr, "", "")
}

func (a *App) reRenderEditItem(w http.ResponseWriter, r *http.Request, id int64, itemErr, qty, ingID string) {
	user := userFromContext(r.Context())
	rec, err := a.Recipes.GetWithItems(r.Context(), id, user.ID)
	if err != nil {
		a.handleRecipeError(w, r, err)
		return
	}
	a.renderRecipeEdit(w, r, rec, "", itemErr, qty, ingID)
}

func (a *App) handleRecipeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrRecipeNotFound):
		a.notFound(w, r)
	case errors.Is(err, db.ErrRecipeForbidden):
		http.Error(w, "operazione non consentita", http.StatusForbidden)
	default:
		a.serverError(w, r, err)
	}
}

func parseID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, key), 10, 64)
}
