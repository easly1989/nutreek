package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/amon2126/nutreek/internal/db"
)

// handleAddMealCheat POST /meals/{mealID}/cheat/items — recipe XOR ingredient.
// Identico ad handleAddMealItem ma scrive in meal_cheat_items via
// WeeklyPlans.AddCheat. Supporta `barcode` per auto-import OFF.
func (a *App) handleAddMealCheat(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	kind := r.PostFormValue("kind") // "recipe" | "ingredient"
	qtyStr := r.PostFormValue("quantity")
	qty, err := parseNonNegFloat(qtyStr)
	if err != nil || qty <= 0 {
		a.flashAndBack(w, r, weekStart, "Quantita' non valida.")
		return
	}

	var rPtr, iPtr *int64
	switch kind {
	case "recipe":
		idStr := r.PostFormValue("ref_id")
		refID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || refID <= 0 {
			a.flashAndBack(w, r, weekStart, "Seleziona una ricetta.")
			return
		}
		rPtr = &refID
	case "ingredient":
		barcode := strings.TrimSpace(r.PostFormValue("barcode"))
		if barcode != "" {
			ingID, err := a.resolveOFFBarcode(r.Context(), barcode)
			if err != nil {
				a.flashAndBack(w, r, weekStart, "Import OFF fallito: "+err.Error())
				return
			}
			iPtr = &ingID
		} else {
			idStr := r.PostFormValue("ref_id")
			if idStr == "" {
				idStr = r.PostFormValue("ingredient_id")
			}
			refID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || refID <= 0 {
				a.flashAndBack(w, r, weekStart, "Seleziona un ingrediente.")
				return
			}
			iPtr = &refID
		}
	default:
		a.flashAndBack(w, r, weekStart, "Tipo non valido.")
		return
	}

	// I cheat sono memorizzati sempre in grammi/ml: se l'utente sceglie "pz"
	// nel picker, converti subito moltiplicando per il peso medio dell'unita'.
	if iPtr != nil && strings.TrimSpace(r.PostFormValue("qty_unit")) == "pz" {
		ing, gerr := a.Ingredients.GetByID(r.Context(), *iPtr)
		if gerr == nil && ing.UnitWeightG != nil && *ing.UnitWeightG > 0 {
			qty = qty * (*ing.UnitWeightG)
		}
	}

	if _, err := a.WeeklyPlans.AddCheat(r.Context(), mealID, user.ID, rPtr, iPtr, qty); err != nil {
		a.flashAndBack(w, r, weekStart, "Impossibile aggiungere lo sgarro: "+err.Error())
		return
	}
	day, dayErr := a.dayIndexFromMealID(r, mealID)
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}

// handleRemoveMealCheat POST /meal-cheat-items/{cheatID}/delete?start=YYYY-MM-DD
func (a *App) handleRemoveMealCheat(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	cheatID, err := parseID(r, "cheatID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	// Risolve il day del meal a partire dal cheat (per lo swap card).
	var day int
	dayErr := a.Store.DB.QueryRowContext(r.Context(),
		`SELECT m.day_of_week FROM meals m
		 JOIN meal_cheat_items mci ON mci.meal_id = m.id
		 WHERE mci.id = ?`, cheatID).Scan(&day)
	if err := a.WeeklyPlans.RemoveCheat(r.Context(), cheatID, user.ID); err != nil {
		if errors.Is(err, db.ErrCheatNotFound) {
			http.Error(w, "sgarro non trovato", http.StatusNotFound)
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

// handleClearMealCheats POST /meals/{mealID}/cheat/clear?start=YYYY-MM-DD
// Rimuove tutti gli sgarri del pasto, riporta lo stato a "ufficiale".
func (a *App) handleClearMealCheats(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	day, dayErr := a.dayIndexFromMealID(r, mealID)
	if err := a.WeeklyPlans.ClearCheats(r.Context(), mealID, user.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if dayErr != nil {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	a.respondMutationOrRedirect(w, r, weekStart, day)
}
