package app

import (
	"net/http"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/household"
)

// Household (#238) in the day: a bar per person, each row's quantity for each
// person, and who eats each meal. The scaling is in internal/household.

// hhPerson is one bar of the day: the person's kcal against their goal. Color 0 is
// the account owner.
type hhPerson struct {
	ID     int64
	Name   string
	Color  int
	Kcal   float64
	Target float64
	Owner  bool
}

// Pct is the bar's fill, 0 to 100.
func (p hhPerson) Pct() int {
	if p.Target <= 0 {
		return 0
	}
	if v := int(p.Kcal * 100 / p.Target); v < 100 {
		return v
	}
	return 100
}

// Over says whether the person is past their goal.
func (p hhPerson) Over() bool { return p.Target > 0 && p.Kcal > p.Target*1.05 }

// hhSkip says whether a person eats a meal.
type hhSkip struct {
	PersonID int64
	Name     string
	Color    int
	Skipped  bool
}

// hhShare is what a person eats of a row.
type hhShare struct {
	PersonID int64
	Name     string
	Color    int
	Qty      float64
	Auto     float64 // what the ratio gives, without a hand-set quantity
	Unit     string
	Manual   bool
	None     bool    // not eaten: the meal is skipped or the quantity is 0
	Hand     float64 // the quantity set by hand, kept when the person skips the whole meal
}

// attachHousehold fills the household's part of the days. It does nothing when the
// account has nobody else.
func (a *App) attachHousehold(r *http.Request, user *domain.User, full *domain.WeeklyPlanFull, days []dayView) error {
	if a.Household == nil || user == nil {
		return nil
	}
	people, err := a.Household.List(r.Context(), user.ID)
	if err != nil || len(people) == 0 {
		return err
	}
	week, err := a.Household.Week(r.Context(), user.ID, full.ID)
	if err != nil {
		return err
	}
	var base float64
	if user.TargetKcal != nil {
		base = *user.TargetKcal
	}
	servings, piece := a.t(r, "picker.servings_unit"), a.t(r, "picker.piece")
	persons := make([]household.Person, len(people))
	for i, p := range people {
		persons[i] = household.Person{ID: p.ID, Ratio: household.Ratio(p.TargetKcal, base),
			Quantities: week.Quantities[p.ID], Skipped: week.Skips[p.ID]}
	}
	for d := range days {
		dv := &days[d]
		dv.People = append(dv.People, hhPerson{Name: user.DisplayName, Kcal: dv.DayTotal.Kcal, Target: base, Owner: true})
		for i, p := range people {
			dv.People = append(dv.People, hhPerson{ID: p.ID, Name: p.Name, Color: i + 1,
				Kcal: persons[i].DayMacros(full.Days[d]).Kcal, Target: p.TargetKcal})
		}
		for m := range dv.Meals {
			mv := &dv.Meals[m]
			if mv.Meal.IsReminder {
				continue
			}
			mv.Owner = user.DisplayName
			for i, p := range people {
				mv.Skips = append(mv.Skips, hhSkip{PersonID: p.ID, Name: p.Name, Color: i + 1, Skipped: persons[i].Skipped[mv.Meal.ID]})
			}
			if mv.HasCheats { // what was eaten instead counts by the ratio; the rows are not theirs
				continue
			}
			for k := range mv.ItemViews {
				it := mv.ItemViews[k].Item
				unit := it.QuantityUnit
				switch {
				case it.Recipe != nil:
					unit = servings
				case unit == "pz":
					unit = piece
				}
				for i, p := range people {
					q := persons[i].Quantity(it)
					mv.ItemViews[k].People = append(mv.ItemViews[k].People, hhShare{PersonID: p.ID, Name: p.Name, Color: i + 1,
						Qty: q, Auto: persons[i].Auto(it), Unit: unit, Manual: persons[i].Manual(it), None: q <= 0, Hand: persons[i].Quantities[it.ID]})
				}
			}
		}
	}
	return nil
}
