package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Household (#238): the people who share an account's week. See migration 0047 and
// internal/household for how their quantities are worked out.

// MaxPeople is how many people a household holds besides the account owner.
const MaxPeople = 6

// ErrInvalidPerson is returned for a name or a goal that cannot be saved, or a seventh person.
var ErrInvalidPerson = errors.New("invalid person")

// ErrPersonNotFound is returned for a person, a meal or an item that is not the user's.
var ErrPersonNotFound = errors.New("person not found")

// HouseholdPerson is one person of a household.
type HouseholdPerson struct {
	ID         int64
	UserID     int64
	Name       string
	TargetKcal float64
}

// HouseholdWeek is what the people changed in one week's plan, by person id.
type HouseholdWeek struct {
	Quantities map[int64]map[int64]float64 // person -> meal item -> quantity set by hand
	Skips      map[int64]map[int64]bool    // person -> meal -> not eaten
}

// Household is the repository of the household tables.
type Household struct {
	db *sql.DB
}

// NewHousehold returns the repository.
func NewHousehold(db *sql.DB) *Household { return &Household{db: db} }

// List returns the people of a household in the order they were added.
func (h *Household) List(ctx context.Context, userID int64) ([]HouseholdPerson, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT id, user_id, name, target_kcal FROM household_people WHERE user_id = ? ORDER BY sort_order, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list household: %w", err)
	}
	defer rows.Close()
	var out []HouseholdPerson
	for rows.Next() {
		var p HouseholdPerson
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.TargetKcal); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func validPerson(name string, kcal float64) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= 60 && kcal >= 500 && kcal <= 6000
}

// Add adds a person to the household and returns their id.
func (h *Household) Add(ctx context.Context, userID int64, name string, kcal float64) (int64, error) {
	name, ok := validPerson(name, kcal)
	if !ok {
		return 0, ErrInvalidPerson
	}
	var have int
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM household_people WHERE user_id = ?`, userID).Scan(&have); err != nil {
		return 0, err
	}
	if have >= MaxPeople {
		return 0, ErrInvalidPerson
	}
	res, err := h.db.ExecContext(ctx,
		`INSERT INTO household_people (user_id, name, target_kcal, sort_order) VALUES (?, ?, ?, ?)`, userID, name, kcal, have)
	if err != nil {
		return 0, fmt.Errorf("add person: %w", err)
	}
	return res.LastInsertId()
}

// Update changes a person's name and goal.
func (h *Household) Update(ctx context.Context, userID, id int64, name string, kcal float64) error {
	name, ok := validPerson(name, kcal)
	if !ok {
		return ErrInvalidPerson
	}
	res, err := h.db.ExecContext(ctx,
		`UPDATE household_people SET name = ?, target_kcal = ? WHERE id = ? AND user_id = ?`, name, kcal, id, userID)
	return affected(res, err)
}

// Delete removes a person and everything they changed.
func (h *Household) Delete(ctx context.Context, userID, id int64) error {
	res, err := h.db.ExecContext(ctx, `DELETE FROM household_people WHERE id = ? AND user_id = ?`, id, userID)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPersonNotFound
	}
	return nil
}

// Week returns what the user's people changed in the plan planID.
func (h *Household) Week(ctx context.Context, userID, planID int64) (HouseholdWeek, error) {
	w := HouseholdWeek{Quantities: map[int64]map[int64]float64{}, Skips: map[int64]map[int64]bool{}}
	rows, err := h.db.QueryContext(ctx, `
		SELECT q.person_id, q.meal_item_id, q.quantity
		FROM household_item_quantities q
		JOIN household_people p ON p.id = q.person_id
		JOIN meal_items i ON i.id = q.meal_item_id
		JOIN meals m ON m.id = i.meal_id
		WHERE p.user_id = ? AND m.weekly_plan_id = ?`, userID, planID)
	if err != nil {
		return w, fmt.Errorf("household quantities: %w", err)
	}
	for rows.Next() {
		var person, item int64
		var q float64
		if err := rows.Scan(&person, &item, &q); err != nil {
			rows.Close()
			return w, err
		}
		if w.Quantities[person] == nil {
			w.Quantities[person] = map[int64]float64{}
		}
		w.Quantities[person][item] = q
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return w, err
	}
	rows, err = h.db.QueryContext(ctx, `
		SELECT s.person_id, s.meal_id
		FROM household_meal_skips s
		JOIN household_people p ON p.id = s.person_id
		JOIN meals m ON m.id = s.meal_id
		WHERE p.user_id = ? AND m.weekly_plan_id = ?`, userID, planID)
	if err != nil {
		return w, fmt.Errorf("household skips: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var person, meal int64
		if err := rows.Scan(&person, &meal); err != nil {
			return w, err
		}
		if w.Skips[person] == nil {
			w.Skips[person] = map[int64]bool{}
		}
		w.Skips[person][meal] = true
	}
	return w, rows.Err()
}

// owns checks that the person and the meal item both belong to the user.
func (h *Household) ownsItem(ctx context.Context, userID, personID, itemID int64) error {
	var n int
	err := h.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM household_people p, meal_items i
		JOIN meals m ON m.id = i.meal_id
		JOIN weekly_plans w ON w.id = m.weekly_plan_id
		WHERE p.id = ? AND p.user_id = ? AND i.id = ? AND w.user_id = ?`, personID, userID, itemID, userID).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrPersonNotFound
	}
	return nil
}

func (h *Household) ownsMeal(ctx context.Context, userID, personID, mealID int64) error {
	var n int
	err := h.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM household_people p, meals m
		JOIN weekly_plans w ON w.id = m.weekly_plan_id
		WHERE p.id = ? AND p.user_id = ? AND m.id = ? AND w.user_id = ?`, personID, userID, mealID, userID).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrPersonNotFound
	}
	return nil
}

// SetQuantity sets by hand what a person eats of a meal item (0 = not eaten).
func (h *Household) SetQuantity(ctx context.Context, userID, personID, itemID int64, qty float64) error {
	if qty < 0 || qty > 100000 {
		return ErrInvalidPerson
	}
	if err := h.ownsItem(ctx, userID, personID, itemID); err != nil {
		return err
	}
	_, err := h.db.ExecContext(ctx, `
		INSERT INTO household_item_quantities (person_id, meal_item_id, quantity) VALUES (?, ?, ?)
		ON CONFLICT (person_id, meal_item_id) DO UPDATE SET quantity = excluded.quantity`, personID, itemID, qty)
	return err
}

// ClearQuantity gives the item back to the automatic quantity.
func (h *Household) ClearQuantity(ctx context.Context, userID, personID, itemID int64) error {
	if err := h.ownsItem(ctx, userID, personID, itemID); err != nil {
		return err
	}
	_, err := h.db.ExecContext(ctx, `DELETE FROM household_item_quantities WHERE person_id = ? AND meal_item_id = ?`, personID, itemID)
	return err
}

// SetSkip marks a whole meal as eaten or not eaten by a person.
func (h *Household) SetSkip(ctx context.Context, userID, personID, mealID int64, skip bool) error {
	if err := h.ownsMeal(ctx, userID, personID, mealID); err != nil {
		return err
	}
	if skip {
		_, err := h.db.ExecContext(ctx, `INSERT OR IGNORE INTO household_meal_skips (person_id, meal_id) VALUES (?, ?)`, personID, mealID)
		return err
	}
	_, err := h.db.ExecContext(ctx, `DELETE FROM household_meal_skips WHERE person_id = ? AND meal_id = ?`, personID, mealID)
	return err
}
