package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

const (
	suggestMinRunes = 2 // shorter than this suggests nothing
	suggestMax      = 6
	copyListsMax    = 4 // past lists the copy sheet offers
)

// suggestView is a suggestion under the field of the manual items. Head is the
// part of the text the user typed (shown bold), Tail the rest.
type suggestView struct {
	Description string
	Head, Tail  string
	Qty         float64
	QtyAttr     string // Qty as a plain number for the field
	Unit        string
	Uses        int
}

// copyItemView is an item of a past list in the copy sheet.
type copyItemView struct {
	ID          int64
	Description string
	Qty         float64
	Unit        string
	Text        string // a quantity written as free text
	Checked     bool   // ticked to be copied
}

// copyListView is a past list in the copy sheet: one tab.
type copyListView struct {
	WeekStart time.Time
	WeekCount int
	Items     []copyItemView
	Active    bool
}

// suggestionViews splits each text into what the user typed and the rest.
func suggestionViews(sugs []db.ManualSuggestion, typed string) []suggestView {
	typed = strings.TrimSpace(typed)
	n := utf8.RuneCountInString(typed)
	out := make([]suggestView, 0, len(sugs))
	for _, s := range sugs {
		v := suggestView{Description: s.Description, Qty: s.Quantity, Unit: s.Unit, Uses: s.Uses, Tail: s.Description}
		if s.Quantity > 0 {
			v.QtyAttr = strconv.FormatFloat(s.Quantity, 'f', -1, 64)
		}
		if r := []rune(s.Description); len(r) >= n && strings.EqualFold(string(r[:n]), typed) {
			v.Head, v.Tail = string(r[:n]), string(r[n:])
		}
		out = append(out, v)
	}
	return out
}

// pastCopyLists is what the copy sheet offers: the items added by hand to the
// latest past lists, minus those already added by hand to this one. Items that
// were set aside (ignored) start unticked.
func pastCopyLists(past []db.PastManualList, current []domain.ShoppingUserItem) []copyListView {
	have := make(map[string]bool, len(current))
	for _, it := range current {
		have[strings.ToLower(strings.TrimSpace(it.Description))] = true
	}
	var out []copyListView
	seen := map[string]bool{} // an item of an earlier tab is ticked once, in the latest list
	for _, p := range past {
		v := copyListView{WeekStart: p.WeekStart, WeekCount: p.WeekCount}
		for _, it := range p.Items {
			key := strings.ToLower(strings.TrimSpace(it.Description))
			if have[key] {
				continue
			}
			v.Items = append(v.Items, copyItemView{ID: it.ID, Description: it.Description, Qty: it.Quantity, Unit: it.Unit, Text: it.QuantityText, Checked: !it.Ignored && !seen[key]})
			seen[key] = true
		}
		if len(v.Items) > 0 {
			out = append(out, v)
		}
	}
	if len(out) > 0 {
		out[0].Active = true
	}
	return out
}

// handleShoppingSuggest GET /shopping-list/suggest?description=
// The items added by hand before that match what is being typed, as a fragment
// for the list under the field.
func (a *App) handleShoppingSuggest(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	typed := strings.TrimSpace(r.URL.Query().Get("description"))
	var views []suggestView
	if utf8.RuneCountInString(typed) >= suggestMinRunes && utf8.RuneCountInString(typed) <= MaxNameLen {
		sugs, err := a.ShoppingLists.SuggestManualItems(r.Context(), user.ID, typed, suggestMax)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		views = suggestionViews(sugs, typed)
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "shopping_list", "shop_suggest", views); err != nil {
		a.serverError(w, r, err)
	}
}

// handleShoppingCopy POST /shopping-list/{id}/copy
// Body: item (repeated), ids of items added by hand to other lists. They are
// added to this one with their quantity.
func (a *App) handleShoppingCopy(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	listID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	back := shoppingListURL(r)
	if err := r.ParseForm(); err != nil {
		a.shoppingFlashBack(w, r, back, "shopping.flash.copy_none")
		return
	}
	var ids []int64
	for _, raw := range r.PostForm["item"] {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		a.shoppingFlashBack(w, r, back, "shopping.flash.copy_none")
		return
	}
	if _, err := a.ShoppingLists.CopyManualItems(r.Context(), listID, user.ID, ids); err != nil {
		if errors.Is(err, db.ErrShoppingListNotFound) {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
