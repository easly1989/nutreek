package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// shoppingListData e' il view-model passato a shopping_list.html.
type shoppingListData struct {
	authPageData
	List          *domain.ShoppingListFull
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

// shoppingRowData e' usato per il partial shopping_row.html (swap HTMX
// dopo toggle di una singola riga, sia auto che user).
type shoppingRowData struct {
	authPageData
	Item      *domain.ShoppingListItem
	UserItem  *domain.ShoppingUserItem
	WeekStart string
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
	if v := strings.TrimSpace(r.URL.Query().Get("weeks")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 4 {
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

	data := shoppingListData{
		authPageData:  a.baseData(r),
		List:          full,
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
	weekStart := strings.TrimSpace(r.PostFormValue("week"))
	weeks := strings.TrimSpace(r.PostFormValue("weeks"))
	if desc == "" {
		a.Sessions.Put(r.Context(), "flash", "Descrizione obbligatoria.")
		http.Redirect(w, r, "/shopping-list?week="+weekStart+"&weeks="+weeks, http.StatusSeeOther)
		return
	}
	if !withinLen(desc, MaxNameLen) {
		a.Sessions.Put(r.Context(), "flash", "Descrizione troppo lunga.")
		http.Redirect(w, r, "/shopping-list?week="+weekStart+"&weeks="+weeks, http.StatusSeeOther)
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
	http.Redirect(w, r, "/shopping-list?week="+weekStart+"&weeks="+weeks, http.StatusSeeOther)
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
	weekStart := strings.TrimSpace(r.URL.Query().Get("week"))
	if weekStart == "" {
		weekStart = r.PostFormValue("week")
	}
	if r.Header.Get("HX-Request") == "true" {
		// Ricarica la lista per recuperare lo stato aggiornato.
		listID, ws, item, err := a.findShoppingUserItem(r, user.ID, itemID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		_ = listID
		// Per la riga manuale lo swap si limita all'item.
		data := struct {
			authPageData
			Item      domain.ShoppingUserItem
			Duplicate bool
			WeekStart string
		}{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			Item:         *item,
			WeekStart:    ws,
		}
		if err := a.Render.RenderPartial(w, http.StatusOK, "shopping_list", "shopping_row_user", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	http.Redirect(w, r, "/shopping-list?week="+weekStart, http.StatusSeeOther)
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
	if r.Header.Get("HX-Request") == "true" {
		w.WriteHeader(http.StatusOK) // body vuoto, riga rimossa via outerHTML="" semantica
		return
	}
	weekStart := strings.TrimSpace(r.URL.Query().Get("week"))
	if weekStart == "" {
		weekStart = r.PostFormValue("week")
	}
	http.Redirect(w, r, "/shopping-list?week="+weekStart, http.StatusSeeOther)
}

// findShoppingUserItem trova un singolo user_item per render del partial.
func (a *App) findShoppingUserItem(r *http.Request, userID, itemID int64) (int64, string, *domain.ShoppingUserItem, error) {
	var listID int64
	var weekStartStr string
	err := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT sl.id, sl.week_start
		FROM shopping_user_items sui
		JOIN shopping_lists sl ON sl.id = sui.shopping_list_id
		WHERE sui.id = ? AND sl.user_id = ?
	`, itemID, userID).Scan(&listID, &weekStartStr)
	if err != nil {
		return 0, "", nil, err
	}
	full, err := a.ShoppingLists.LoadFull(r.Context(), listID, userID)
	if err != nil {
		return 0, "", nil, err
	}
	for _, it := range full.UserItems {
		if it.ID == itemID {
			return listID, strings.Split(weekStartStr, "T")[0], &it, nil
		}
	}
	return 0, "", nil, db.ErrShoppingItemNotFound
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
	http.Redirect(w, r,
		"/shopping-list?week="+weekStart.Format("2006-01-02")+"&weeks="+strconv.Itoa(weeks),
		http.StatusSeeOther)
}

// handleShoppingToggle POST /shopping-list/items/{id}/toggle
// Per HX-Request ritorna la riga partial aggiornata (swap inline);
// altrimenti redirect alla lista.
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
	if r.Header.Get("HX-Request") == "true" {
		// Recupera lo stato aggiornato per renderizzare la riga.
		listID, weekStart, item, err := a.findShoppingItem(r, user.ID, itemID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		_ = listID
		data := shoppingRowData{
			authPageData: authPageData{CSRFToken: nosurf.Token(r)},
			Item:         item,
			WeekStart:    weekStart,
		}
		if err := a.Render.RenderPartial(w, http.StatusOK, "shopping_list", "shopping_row_auto", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	weekStart := r.URL.Query().Get("week")
	if weekStart == "" {
		weekStart = r.PostFormValue("week")
	}
	http.Redirect(w, r, "/shopping-list?week="+weekStart, http.StatusSeeOther)
}

// findShoppingItem ricarica un singolo shopping_list_item per render
// del partial post-toggle.
func (a *App) findShoppingItem(r *http.Request, userID, itemID int64) (int64, string, *domain.ShoppingListItem, error) {
	var listID int64
	var weekStartStr string
	err := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT sl.id, sl.week_start
		FROM shopping_list_items sli
		JOIN shopping_lists sl ON sl.id = sli.shopping_list_id
		WHERE sli.id = ? AND sl.user_id = ?
	`, itemID, userID).Scan(&listID, &weekStartStr)
	if err != nil {
		return 0, "", nil, err
	}
	full, err := a.ShoppingLists.LoadFull(r.Context(), listID, userID)
	if err != nil {
		return 0, "", nil, err
	}
	for _, it := range full.Items {
		if it.ID == itemID {
			return listID, strings.Split(weekStartStr, "T")[0], &it, nil
		}
	}
	return 0, "", nil, db.ErrShoppingItemNotFound
}
