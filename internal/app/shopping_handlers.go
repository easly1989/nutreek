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
// AutoNames raccoglie i nomi (lowercased) degli ingredienti auto per
// rilevare duplicati con voci utente: il template mostra un badge
// "anche in lista auto" sulla riga manuale corrispondente. (Tappa 6)
type shoppingListData struct {
	authPageData
	List       *domain.ShoppingListFull
	WeekStart  string // YYYY-MM-DD
	WeekCount  int
	NextWeek   string // bottone "lista per la prossima settimana"
	PrevWeek   string
}

// shoppingRowData e' usato per il partial shopping_row.html (swap HTMX
// dopo toggle di una singola riga, sia auto che user).
type shoppingRowData struct {
	authPageData
	Item       *domain.ShoppingListItem
	UserItem   *domain.ShoppingUserItem
	WeekStart  string
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
	data := shoppingListData{
		authPageData: a.baseData(r),
		List:         full,
		WeekStart:    weekStart.Format("2006-01-02"),
		WeekCount:    weeks,
		PrevWeek:     weekStart.AddDate(0, 0, -7).Format("2006-01-02"),
		NextWeek:     weekStart.AddDate(0, 0, 7).Format("2006-01-02"),
	}
	if err := a.Render.Render(w, http.StatusOK, "shopping_list", data); err != nil {
		a.serverError(w, r, err)
	}
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
