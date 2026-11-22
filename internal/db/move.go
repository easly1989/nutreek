package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// noCheats keeps the meals that have no cheats (what was eaten instead): their
// rows are shown struck and count for nothing, so a row moved there would vanish
// from the day's totals.
const noCheats = `NOT EXISTS (SELECT 1 FROM meal_cheat_items c WHERE c.meal_id = meals.id)`

// MoveContext is what the «Move to…» menu of a row needs (#228): where the row
// is, and the other meals of its day.
type MoveContext struct {
	MealID    int64
	DayOfWeek int
	Meals     []MoveTarget // the other meals of the same day, in order
}

// MoveTarget is a meal a row can move to.
type MoveTarget struct {
	ID   int64
	Name string
}

// MoveContextFor returns the context of the row itemID (an ingredient or a
// recipe) in a plan of userID; ErrMealItemNotFound otherwise.
func (r *WeeklyPlans) MoveContextFor(ctx context.Context, itemID, userID int64) (*MoveContext, error) {
	var mc MoveContext
	var planID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT m.id, m.day_of_week, m.weekly_plan_id
		FROM meal_items mi
		JOIN meals m ON m.id = mi.meal_id
		JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE mi.id = ? AND wp.user_id = ?`, itemID, userID,
	).Scan(&mc.MealID, &mc.DayOfWeek, &planID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMealItemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("move context: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name FROM meals
		WHERE weekly_plan_id = ? AND day_of_week = ? AND id <> ? AND `+noCheats+`
		ORDER BY sort_order, id`, planID, mc.DayOfWeek, mc.MealID)
	if err != nil {
		return nil, fmt.Errorf("move targets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t MoveTarget
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan move target: %w", err)
		}
		mc.Meals = append(mc.Meals, t)
	}
	return &mc, rows.Err()
}

// MoveResult says where a row ended up.
type MoveResult struct {
	FromDay int // the day it left
	ToDay   int // the day it is in now
}

// MoveItem moves the row itemID (an ingredient or a recipe, with its
// customizations) to the meal toMealID of the same week. order is the ids of that
// meal's rows after the drop, the moved one included: the row takes its place
// there; with no order it goes last. ErrMealItemNotFound when the row is not in a
// plan of userID, ErrMealNotFound when the meal is not in the same plan,
// ErrReorderMismatch when order is not the meal's rows plus the moved one.
func (r *WeeklyPlans) MoveItem(ctx context.Context, itemID, userID, toMealID int64, order []int64) (MoveResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MoveResult{}, fmt.Errorf("begin move: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := moveItemTx(ctx, tx, itemID, userID, toMealID, order)
	if err != nil {
		return MoveResult{}, err
	}
	return res, tx.Commit()
}

// MoveItemToDay moves the row to another day of the same week: into the meal of
// that day with the same name as the one it leaves (any case), else into the last
// meal of the day, else into a new meal with the name it leaves, so the day is
// never refused. It goes last in the meal.
func (r *WeeklyPlans) MoveItemToDay(ctx context.Context, itemID, userID int64, day int) (MoveResult, error) {
	if day < 0 || day > 6 {
		return MoveResult{}, fmt.Errorf("day_of_week fuori range")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MoveResult{}, fmt.Errorf("begin move: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var planID int64
	var fromName string
	err = tx.QueryRowContext(ctx, `
		SELECT m.weekly_plan_id, m.name
		FROM meal_items mi
		JOIN meals m ON m.id = mi.meal_id
		JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE mi.id = ? AND wp.user_id = ?`, itemID, userID,
	).Scan(&planID, &fromName)
	if errors.Is(err, sql.ErrNoRows) {
		return MoveResult{}, ErrMealItemNotFound
	}
	if err != nil {
		return MoveResult{}, fmt.Errorf("load item meal: %w", err)
	}

	// The meals of that day that can take a row (see noCheats), in order: the
	// first with the name it leaves, else the last.
	fromName = strings.TrimSpace(fromName)
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name FROM meals
		WHERE weekly_plan_id = ? AND day_of_week = ? AND `+noCheats+`
		ORDER BY sort_order, id`, planID, day)
	if err != nil {
		return MoveResult{}, fmt.Errorf("find meal of the day: %w", err)
	}
	var toMeal, lastMeal int64
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return MoveResult{}, fmt.Errorf("scan meal: %w", err)
		}
		lastMeal = id
		if toMeal == 0 && strings.EqualFold(strings.TrimSpace(name), fromName) {
			toMeal = id
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return MoveResult{}, err
	}
	rows.Close()
	if toMeal == 0 {
		toMeal = lastMeal
	}
	if toMeal == 0 {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO meals (weekly_plan_id, day_of_week, name, sort_order) VALUES (?, ?, ?, 0)`,
			planID, day, fromName)
		if err != nil {
			return MoveResult{}, fmt.Errorf("insert meal: %w", err)
		}
		if toMeal, err = res.LastInsertId(); err != nil {
			return MoveResult{}, err
		}
	}
	res, err := moveItemTx(ctx, tx, itemID, userID, toMeal, nil)
	if err != nil {
		return MoveResult{}, err
	}
	return res, tx.Commit()
}

func moveItemTx(ctx context.Context, tx *sql.Tx, itemID, userID, toMealID int64, order []int64) (MoveResult, error) {
	var res MoveResult
	var fromMeal, fromPlan int64
	err := tx.QueryRowContext(ctx, `
		SELECT mi.meal_id, m.weekly_plan_id, m.day_of_week
		FROM meal_items mi
		JOIN meals m ON m.id = mi.meal_id
		JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE mi.id = ? AND wp.user_id = ?`, itemID, userID,
	).Scan(&fromMeal, &fromPlan, &res.FromDay)
	if errors.Is(err, sql.ErrNoRows) {
		return res, ErrMealItemNotFound
	}
	if err != nil {
		return res, fmt.Errorf("load item: %w", err)
	}
	var toPlan int64
	var cheated bool
	err = tx.QueryRowContext(ctx, `SELECT weekly_plan_id, day_of_week, NOT (`+noCheats+`) FROM meals WHERE id = ?`, toMealID).Scan(&toPlan, &res.ToDay, &cheated)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (toPlan != fromPlan || cheated)) {
		return res, ErrMealNotFound // another plan, even the user's own, is not this week; a meal with cheats takes no rows
	}
	if err != nil {
		return res, fmt.Errorf("load meal: %w", err)
	}

	current, err := txInt64s(ctx, tx, `SELECT id FROM meal_items WHERE meal_id = ? AND id <> ?`, toMealID, itemID)
	if err != nil {
		return res, fmt.Errorf("load meal items: %w", err)
	}
	if order == nil {
		var next int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order)+1, 0) FROM meal_items WHERE meal_id = ?`, toMealID).Scan(&next); err != nil {
			return res, fmt.Errorf("compute sort_order: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE meal_items SET meal_id = ?, sort_order = ? WHERE id = ?`, toMealID, next, itemID); err != nil {
			return res, fmt.Errorf("move item: %w", err)
		}
		return res, nil
	}
	if !sameInt64Set(append(current, itemID), order) {
		return res, ErrReorderMismatch
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meal_items SET meal_id = ? WHERE id = ?`, toMealID, itemID); err != nil {
		return res, fmt.Errorf("move item: %w", err)
	}
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, `UPDATE meal_items SET sort_order = ? WHERE id = ? AND meal_id = ?`, i, id, toMealID); err != nil {
			return res, fmt.Errorf("order item: %w", err)
		}
	}
	return res, nil
}

// txInt64s is scanInt64Set inside a transaction.
func txInt64s(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
