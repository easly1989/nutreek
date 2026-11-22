package app

import (
	"errors"
	"net/http"

	"github.com/amon2126/nutreek/internal/db"
)

const precheckRowsMax = 500

// handleShoppingPrecheck POST /shopping-list/{id}/precheck
// Body: row (repeated, the row keys) and, for each, st-<key> (have, some or
// need) plus have-<key> and unit-<key> for «some». Applies the check before
// shopping: rows are set aside, lowered or brought back, never deleted.
func (a *App) handleShoppingPrecheck(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	listID, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	back := shoppingListURL(r)
	if err := r.ParseForm(); err != nil {
		a.shoppingFlashBack(w, r, back, "shopping.flash.precheck_invalid")
		return
	}
	keys := r.PostForm["row"]
	if len(keys) > precheckRowsMax {
		keys = keys[:precheckRowsMax]
	}
	choices := make([]db.PrecheckChoice, 0, len(keys))
	for _, k := range keys {
		c := db.PrecheckChoice{Key: k, State: r.PostFormValue("st-" + k)}
		if c.State == db.PrecheckSome {
			qty, ok := parseShoppingQuantity(r.PostFormValue("have-" + k))
			if !ok {
				qty = 0
			}
			c.Have, c.Unit = qty, r.PostFormValue("unit-"+k)
		}
		choices = append(choices, c)
	}
	switch err := a.ShoppingLists.ApplyPrecheck(r.Context(), listID, user.ID, choices); {
	case err == nil:
		http.Redirect(w, r, back, http.StatusSeeOther)
	case errors.Is(err, db.ErrShoppingListNotFound):
		a.notFound(w, r)
	case errors.Is(err, db.ErrShoppingPrecheckInvalid):
		a.shoppingFlashBack(w, r, back+"&check=1", "shopping.flash.precheck_invalid")
	default:
		a.serverError(w, r, err)
	}
}
