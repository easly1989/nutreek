package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// shoppingListData e' il view-model passato a shopping_list.html.
type shoppingListData struct {
	authPageData
	List          *domain.ShoppingListFull
	View          shoppingView
	UserItemViews []shoppingUserItemView // per badge duplicati
	WeekStart     string                 // YYYY-MM-DD
	WeekCount     int
	NextWeek      string
	PrevWeek      string
}

// shoppingUserItemView arricchisce ShoppingUserItem con il flag
// "duplicato": true se la description corrisponde (case-insensitive)
// al nome di un ingrediente auto presente in lista.
type shoppingUserItemView struct {
	Item      domain.ShoppingUserItem
	Duplicate bool
}

// parseShoppingWeek normalizza il param `week` (default = lunedi'
// prossimo) e `weeks` (1..4, default 1).
func parseShoppingWeek(r *http.Request) (time.Time, int) {
	weekStart := nextMonday(time.Now())
	if v := strings.TrimSpace(r.URL.Query().Get("week")); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	if v := strings.TrimSpace(r.PostFormValue("week")); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			weekStart = db.MondayOf(t)
		}
	}
	weeks := 1
	for _, v := range []string{r.URL.Query().Get("weeks"), r.PostFormValue("weeks")} {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 && n <= 4 {
			weeks = n
		}
	}
	return weekStart, weeks
}

// nextMonday ritorna il lunedi' della settimana corrente o successiva.
// Se oggi e' lunedi' resta oggi.
func nextMonday(t time.Time) time.Time {
	m := db.MondayOf(t)
	// Se oggi e' la stessa settimana del lunedi' calcolato, va bene.
	return m
}

// handleShoppingList GET /shopping-list?week=YYYY-MM-DD&weeks=N
func (a *App) handleShoppingList(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart, weeks := parseShoppingWeek(r)
	list, err := a.ShoppingLists.GetOrCreateList(r.Context(), user.ID, weekStart, weeks)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	// Se la lista non ha ancora items auto, generala on-demand al primo
	// load. Anche se l'utente cambia weeks dropdown.
	full, err := a.ShoppingLists.LoadFull(r.Context(), list.ID, user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(full.Items) == 0 || full.WeekCount != weeks {
		if err := a.ShoppingLists.Generate(r.Context(), list.ID, user.ID); err != nil {
			a.serverError(w, r, err)
			return
		}
		full, err = a.ShoppingLists.LoadFull(r.Context(), list.ID, user.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	// Calcola flag duplicati: voce utente con description che matcha
	// (case-insensitive) il nome di un auto-item presente.
	autoNames := make(map[string]bool, len(full.Items))
	for _, it := range full.Items {
		if it.Ingredient != nil {
			autoNames[strings.ToLower(strings.TrimSpace(it.Ingredient.Name))] = true
		}
	}
	userViews := make([]shoppingUserItemView, 0, len(full.UserItems))
	for _, ui := range full.UserItems {
		userViews = append(userViews, shoppingUserItemView{
			Item:      ui,
			Duplicate: autoNames[strings.ToLower(strings.TrimSpace(ui.Description))],
		})
	}

	ingIDs := make([]int64, 0, len(full.Items))
	for _, it := range full.Items {
		ingIDs = append(ingIDs, it.IngredientID)
	}
	cats, err := a.Categories.CategoriesForIngredients(r.Context(), ingIDs)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	data := shoppingListData{
		authPageData:  a.baseData(r),
		List:          full,
		View:          buildShoppingView(full.Items, cats),
		UserItemViews: userViews,
		WeekStart:     weekStart.Format("2006-01-02"),
		WeekCount:     weeks,
		PrevWeek:      weekStart.AddDate(0, 0, -7).Format("2006-01-02"),
		NextWeek:      weekStart.AddDate(0, 0, 7).Format("2006-01-02"),
	}
	if err := a.Render.Render(w, http.StatusOK, "shopping_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleAddShoppingUserItem POST /shopping-list/{id}/user-items
// Body: description, quantity_text. Aggiunge una voce manuale.
// HX-Request: torna la pagina intera (semplifica) tramite redirect.
func (a *App) handleAddShoppingUserItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	listID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	desc := strings.TrimSpace(r.PostFormValue("description"))
	qty := strings.TrimSpace(r.PostFormValue("quantity_text"))
	back := shoppingListURL(r)
	if desc == "" {
		a.Sessions.Put(r.Context(), "flash", "Descrizione obbligatoria.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if !withinLen(desc, MaxNameLen) {
		a.Sessions.Put(r.Context(), "flash", "Descrizione troppo lunga.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if _, err := a.ShoppingLists.AddUserItem(r.Context(), listID, user.ID, desc, qty); err != nil {
		if errors.Is(err, db.ErrShoppingListNotFound) {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// handleToggleShoppingUserItem POST /shopping-list/user-items/{id}/toggle
func (a *App) handleToggleShoppingUserItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.ShoppingLists.ToggleUserItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrShoppingItemNotFound) {
			http.Error(w, "voce non trovata", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// handleRemoveShoppingUserItem POST /shopping-list/user-items/{id}/delete
func (a *App) handleRemoveShoppingUserItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.ShoppingLists.RemoveUserItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrShoppingItemNotFound) {
			http.Error(w, "voce non trovata", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// handleShoppingGenerate POST /shopping-list/generate?week=YYYY-MM-DD&weeks=N
func (a *App) handleShoppingGenerate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart, weeks := parseShoppingWeek(r)
	list, err := a.ShoppingLists.GetOrCreateList(r.Context(), user.ID, weekStart, weeks)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if err := a.ShoppingLists.Generate(r.Context(), list.ID, user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Lista rigenerata.")
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// handleShoppingToggle POST /shopping-list/items/{id}/toggle
// Torna sempre alla lista: la voce spuntata cambia gruppo e l'avanzamento
// cambia, quindi HTMX segue il redirect e ne prende la lista (hx-select).
func (a *App) handleShoppingToggle(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.ShoppingLists.ToggleItem(r.Context(), itemID, user.ID); err != nil {
		if errors.Is(err, db.ErrShoppingItemNotFound) {
			http.Error(w, "voce non trovata", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, shoppingListURL(r), http.StatusSeeOther)
}

// shoppingListURL e' l'indirizzo della lista da cui arriva la richiesta,
// ricostruito da week/weeks validati (mai copiati dall'input).
func shoppingListURL(r *http.Request) string {
	weekStart, weeks := parseShoppingWeek(r)
	return "/shopping-list?week=" + weekStart.Format("2006-01-02") + "&weeks=" + strconv.Itoa(weeks)
}
