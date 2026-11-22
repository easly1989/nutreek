package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/household"
)

// Household (#238): the settings page where the people are managed, the dialog that
// sets what each of them eats of a row, and the toggle for a meal they skip. The
// day shows them (household_view.go); the shopping list sums them (db.ShoppingLists).

// householdRow is a person on the settings page.
type householdRow struct {
	db.HouseholdPerson
	Color int
	Ratio float64 // their goal over the owner's, 0 when the owner has none
}

// householdPageData is the view model of /settings/household.
type householdPageData struct {
	authPageData
	OwnerName   string
	OwnerTarget float64 // 0 until the owner sets a goal in the profile
	People      []householdRow
	CanAdd      bool
}

func (a *App) handleHouseholdPage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	people, err := a.Household.List(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := householdPageData{authPageData: a.baseData(r), OwnerName: user.DisplayName, CanAdd: len(people) < db.MaxPeople}
	data.ActiveNav = "/settings"
	if user.TargetKcal != nil {
		data.OwnerTarget = *user.TargetKcal
	}
	for i, p := range people {
		ratio := 0.0
		if data.OwnerTarget > 0 {
			ratio = p.TargetKcal / data.OwnerTarget
		}
		data.People = append(data.People, householdRow{HouseholdPerson: p, Color: i + 1, Ratio: ratio})
	}
	if err := a.Render.Render(w, r, http.StatusOK, "household", data); err != nil {
		a.serverError(w, r, err)
	}
}

// parseKcal reads a calorie goal: a whole number, "," or "." as the decimal mark.
func parseKcal(s string) float64 {
	v, err := parseNonNegFloat(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return v
}

// handleHouseholdSave POST /settings/household (add) and /settings/household/{id}
// (change): fields name and target_kcal.
func (a *App) handleHouseholdSave(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
		return
	}
	name, kcal := r.PostFormValue("name"), parseKcal(r.PostFormValue("target_kcal"))
	var err error
	if chi.URLParam(r, "id") == "" {
		_, err = a.Household.Add(r.Context(), user.ID, name, kcal)
	} else {
		var id int64
		if id, err = parseID(r, "id"); err != nil {
			a.notFound(w, r)
			return
		}
		err = a.Household.Update(r.Context(), user.ID, id, name, kcal)
	}
	switch {
	case errors.Is(err, db.ErrInvalidPerson):
		a.Sessions.Put(r.Context(), "flash", a.t(r, "household.error.invalid"))
	case errors.Is(err, db.ErrPersonNotFound):
		a.notFound(w, r)
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/household", http.StatusSeeOther)
}

// handleHouseholdDelete POST /settings/household/{id}/delete.
func (a *App) handleHouseholdDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.Household.Delete(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, db.ErrPersonNotFound) {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/household", http.StatusSeeOther)
}

// ---- a row's quantities ----------------------------------------------------

// householdItemData is the view model of the dialog.
type householdItemData struct {
	authPageData
	Name     string
	OwnerQty float64
	Unit     string
	People   []hhShare
	Action   string
}

// householdRow finds the row in the user's week: its view (with the people filled in)
// and the day it is on. ok is false when the row is not in the user's plan.
func (a *App) householdItem(r *http.Request, itemID int64) (v mealItemView, day int, ok bool, err error) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, weekStart)
	if err != nil {
		return v, 0, false, err
	}
	full, err := a.WeeklyPlans.LoadFull(r.Context(), plan)
	if err != nil {
		return v, 0, false, err
	}
	days := buildDayViews(full, weekStart)
	if err := a.attachHousehold(r, user, full, days); err != nil {
		return v, 0, false, err
	}
	for _, dv := range days {
		for _, mv := range dv.Meals {
			if mv.HasCheats {
				continue
			}
			for _, iv := range mv.ItemViews {
				if iv.Item.ID == itemID {
					return iv, dv.Index, true, nil
				}
			}
		}
	}
	return v, 0, false, nil
}

// handleHouseholdItemForm GET /meal-items/{itemID}/people?start=: the dialog (HTMX into
// #modal-host).
func (a *App) handleHouseholdItemForm(w http.ResponseWriter, r *http.Request) {
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToWeek(w, r, weekStart)
		return
	}
	iv, _, ok, err := a.householdItem(r, itemID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !ok || len(iv.People) == 0 {
		http.Error(w, a.t(r, "week.error.item_not_found"), http.StatusNotFound)
		return
	}
	data := householdItemData{authPageData: authPageData{CSRFToken: nosurf.Token(r)}, People: iv.People,
		OwnerQty: iv.Item.Quantity, Action: "/meal-items/" + strconv.FormatInt(itemID, 10) + "/people?start=" + formatDate(weekStart)}
	switch {
	case iv.Item.Recipe != nil:
		data.Name, data.Unit = iv.Item.Recipe.Name, a.t(r, "picker.servings_unit")
	case iv.Item.Ingredient != nil:
		data.Name, data.Unit = iv.Item.Ingredient.Name, iv.Item.QuantityUnit
		if data.Unit == "pz" {
			data.Unit = a.t(r, "picker.piece")
		}
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "household_item_dialog", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleHouseholdItemSave POST /meal-items/{itemID}/people?start=, one field q_<person id>
// for each person: empty is the automatic quantity, a number is theirs (0 = not eaten).
func (a *App) handleHouseholdItemSave(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
		return
	}
	iv, day, ok, err := a.householdItem(r, itemID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if !ok {
		http.Error(w, a.t(r, "week.error.item_not_found"), http.StatusNotFound)
		return
	}
	for _, p := range iv.People {
		raw, sent := r.PostForm["q_"+strconv.FormatInt(p.PersonID, 10)]
		if !sent {
			continue
		}
		raw[0] = strings.TrimSpace(raw[0])
		var err error
		if raw[0] == "" {
			err = a.Household.ClearQuantity(r.Context(), user.ID, p.PersonID, itemID)
		} else {
			q, perr := parseNonNegFloat(raw[0])
			if perr != nil || q > MaxItemQuantity {
				a.flashAndBack(w, r, parseWeekStart(r), a.t(r, "household.error.quantity"))
				return
			}
			if q == household.RoundTo(p.Auto, 0.01) { // the same as the automatic one: nothing to keep
				err = a.Household.ClearQuantity(r.Context(), user.ID, p.PersonID, itemID)
			} else {
				err = a.Household.SetQuantity(r.Context(), user.ID, p.PersonID, itemID, q)
			}
		}
		if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	a.respondMutationOrRedirect(w, r, parseWeekStart(r), day)
}

// handleHouseholdMealSkip POST /meals/{mealID}/people/{personID}/skip?start=, field skip
// (1 = does not eat the meal, 0 = eats it).
func (a *App) handleHouseholdMealSkip(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	mealID, err := parseID(r, "mealID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	personID, err := parseID(r, "personID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, a.t(r, "week.error.invalid_form"), http.StatusBadRequest)
		return
	}
	if err := a.Household.SetSkip(r.Context(), user.ID, personID, mealID, r.PostFormValue("skip") == "1"); err != nil {
		if errors.Is(err, db.ErrPersonNotFound) {
			a.notFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	day, err := a.dayIndexFromMealID(r, mealID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.respondMutationOrRedirect(w, r, parseWeekStart(r), day)
}
