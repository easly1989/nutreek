package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/justinas/nosurf"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/profile"
)

// Move an ingredient or a recipe to another meal (#228): by dragging it (dnd.js,
// one Sortable group for the meals of a day) or from the «Move to…» menu of its
// row, the way that works from the keyboard and on a phone. The menu lists the
// other meals of the day and, under «Another day…», the other days of the week.

// moveMenuData is the view model of the menu.
type moveMenuData struct {
	authPageData
	ItemID    int64
	WeekStart string
	Meals     []db.MoveTarget
	Days      []moveDay
}

// moveDay is another day of the week a row can go to.
type moveDay struct {
	Index int
	Date  time.Time
}

// handleMealItemMoveMenu GET /meal-items/{itemID}/move?start=YYYY-MM-DD: the
// menu panel of a row, loaded when it opens.
func (a *App) handleMealItemMoveMenu(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	weekStart := parseWeekStart(r)
	mc, err := a.WeeklyPlans.MoveContextFor(r.Context(), itemID, user.ID)
	if err != nil {
		a.moveError(w, r, err)
		return
	}
	data := moveMenuData{
		authPageData: authPageData{CSRFToken: nosurf.Token(r)},
		ItemID:       itemID,
		WeekStart:    formatDate(weekStart),
		Meals:        mc.Meals,
	}
	for d := 0; d < 7; d++ {
		if d != mc.DayOfWeek {
			data.Days = append(data.Days, moveDay{Index: d, Date: weekStart.AddDate(0, 0, d)})
		}
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "move_menu", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) moveError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrMealItemNotFound):
		http.Error(w, a.t(r, "week.error.item_not_found"), http.StatusNotFound)
	case errors.Is(err, db.ErrMealNotFound), errors.Is(err, db.ErrPlanNotFound):
		http.Error(w, a.t(r, "week.error.meal_not_found"), http.StatusNotFound)
	case errors.Is(err, db.ErrReorderMismatch):
		http.Error(w, a.t(r, "week.error.invalid_order"), http.StatusConflict)
	default:
		a.serverError(w, r, err)
	}
}

// handleMealItemMove POST /meal-items/{itemID}/move?start=YYYY-MM-DD. The form
// names where the row goes: meal_id (a meal of the same week), with ids, the
// meal's rows in their new order, when it was dropped between them; or to_day
// (0 = Monday), which picks the meal. It answers with the day the row left, and
// the tab of the day it went to when that is another.
func (a *App) handleMealItemMove(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	weekStart := parseWeekStart(r)
	itemID, err := parseID(r, "itemID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	var res db.MoveResult
	if day := strings.TrimSpace(r.PostFormValue("to_day")); day != "" {
		d, convErr := strconv.Atoi(day)
		if convErr != nil || d < 0 || d > 6 {
			http.Error(w, a.t(r, "week.error.missing_day"), http.StatusBadRequest)
			return
		}
		res, err = a.WeeklyPlans.MoveItemToDay(r.Context(), itemID, user.ID, d)
	} else {
		mealID, convErr := strconv.ParseInt(r.PostFormValue("meal_id"), 10, 64)
		if convErr != nil || mealID <= 0 {
			http.Error(w, a.t(r, "week.error.meal_not_found"), http.StatusBadRequest)
			return
		}
		var order []int64
		if raw := strings.TrimSpace(r.PostFormValue("ids")); raw != "" {
			if order, err = parseIDList(raw); err != nil {
				http.Error(w, a.t(r, "week.error.invalid_ids"), http.StatusBadRequest)
				return
			}
		}
		res, err = a.WeeklyPlans.MoveItem(r.Context(), itemID, user.ID, mealID, order)
	}
	if err != nil {
		a.moveError(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		a.redirectToDay(w, r, weekStart, res.FromDay)
		return
	}
	dv, week, err := a.loadDayCard(r, user.ID, weekStart, res.FromDay)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	resp := a.buildDayMutation(r, *dv, week, weekStart, dayState(dv.Date, todayDate()))
	if res.ToDay != res.FromDay {
		// The tab of the day it went to shows its kcal: refresh it too.
		to, _, err := a.loadDayCard(r, user.ID, weekStart, res.ToDay)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		progress := profile.BuildMacroProgress(to.DayTotal, user.TargetKcal, user.TargetProtein, user.TargetCarbs, user.TargetFat)
		tab := makeDayCard(*to, dayState(to.Date, todayDate()), false, formatDate(weekStart), nosurf.Token(r), progress, 0, nil, nil)
		tab.OOB = true
		resp.Extra = &tab
	}
	if err := a.Render.RenderPartial(w, r, http.StatusOK, "dashboard", "day_card_with_pill_oob", resp); err != nil {
		a.serverError(w, r, err)
	}
}
