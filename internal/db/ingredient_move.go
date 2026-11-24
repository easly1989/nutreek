package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrMoveUses is returned when two ingredients cannot swap uses: the same one,
// or units that do not match.
var ErrMoveUses = errors.New("cannot move the uses of this ingredient")

// MoveUsesResult counts what MoveUses changed.
type MoveUsesResult struct {
	Moved   int // rows now pointing at the target
	Skipped int // rows in pieces left alone: the two ingredients weigh a piece differently
}

// The rows of one user's data that use an ingredient, per table. Each condition
// takes the ingredient (?1) and the user (?2); the tables with a unit also let
// the caller keep back the rows counted in pieces.
const (
	ownMeals   = `meal_id IN (SELECT m.id FROM meals m JOIN weekly_plans p ON p.id = m.weekly_plan_id WHERE p.user_id = ?2)`
	ownRecipes = `recipe_id IN (SELECT id FROM recipes WHERE user_id = ?2)`
	ownPlan    = `variation_id IN (SELECT v.id FROM nutritionist_meal_variations v
		JOIN nutritionist_meal_types t ON t.id = v.meal_type_id
		JOIN nutritionist_plans pl ON pl.id = t.plan_id WHERE pl.user_id = ?2)`
	ownMealItems = `meal_item_id IN (SELECT i.id FROM meal_items i JOIN meals m ON m.id = i.meal_id
		JOIN weekly_plans p ON p.id = m.weekly_plan_id WHERE p.user_id = ?2)`
)

// moveTables lists where an ingredient is used. unit: the table has a quantity_unit.
var moveTables = []struct {
	table, owned string
	unit         bool
}{
	{"meal_items", ownMeals, true},
	{"meal_cheat_items", ownMeals, false},
	{"recipe_ingredients", ownRecipes, true},
	{"nutritionist_meal_items", ownPlan, true},
	{"meal_item_customizations", ownMealItems, false},
}

// MoveUses points every use of ingredient from in userID's own data at to: meals
// and cheat meals of their weeks, their recipes, their nutritionist plan and the
// customizations of their meal items. Nothing of another user's is touched, and
// the ingredient itself stays. Rows counted in pieces move only when both
// ingredients weigh a piece the same, because the quantity would otherwise mean
// another weight. Both ingredients must share a unit.
func (r *Ingredients) MoveUses(ctx context.Context, userID int64, from, to *domain.Ingredient) (MoveUsesResult, error) {
	if from == nil || to == nil || from.ID == to.ID || from.Unit != to.Unit {
		return MoveUsesResult{}, ErrMoveUses
	}
	samePiece := from.PieceWeight() == to.PieceWeight()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MoveUsesResult{}, fmt.Errorf("move uses: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var res MoveUsesResult
	for _, t := range moveTables {
		keep := ""
		if t.unit && !samePiece {
			keep = ` AND quantity_unit <> 'pz'`
			var n int
			q := `SELECT count(*) FROM ` + t.table + ` WHERE ingredient_id = ?1 AND quantity_unit = 'pz' AND ` + t.owned
			if err := tx.QueryRowContext(ctx, q, from.ID, userID).Scan(&n); err != nil {
				return MoveUsesResult{}, fmt.Errorf("move uses: count %s: %w", t.table, err)
			}
			res.Skipped += n
		}
		// OR IGNORE: a customization already on the target stays as it is.
		q := `UPDATE OR IGNORE ` + t.table + ` SET ingredient_id = ?3 WHERE ingredient_id = ?1 AND ` + t.owned + keep
		out, err := tx.ExecContext(ctx, q, from.ID, userID, to.ID)
		if err != nil {
			return MoveUsesResult{}, fmt.Errorf("move uses: %s: %w", t.table, err)
		}
		n, _ := out.RowsAffected()
		res.Moved += int(n)
	}
	if err := tx.Commit(); err != nil {
		return MoveUsesResult{}, fmt.Errorf("move uses: commit: %w", err)
	}
	return res, nil
}
