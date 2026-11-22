package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

// ingredientExists is ErrIngredientNotFound for an id that is not an active
// ingredient: a deleted one, or one the reconciler replaced while a dialog was open.
func ingredientExists(ctx context.Context, d *sql.DB, id int64) error {
	var one int
	err := d.QueryRowContext(ctx, `SELECT 1 FROM ingredients WHERE id = ? AND superseded_by IS NULL`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrIngredientNotFound
	}
	if err != nil {
		return fmt.Errorf("ingredient exists: %w", err)
	}
	return nil
}

// MealIngredientItem is a plain-ingredient row of a meal with the name of the
// meal, for the dialog that replaces its ingredient (#227).
type MealIngredientItem struct {
	domain.MealItem
	MealName  string
	DayOfWeek int
}

// IngredientItem returns the row itemID when it holds a plain ingredient (not a
// recipe) in a plan of userID; ErrMealItemNotFound otherwise.
func (r *WeeklyPlans) IngredientItem(ctx context.Context, itemID, userID int64) (*MealIngredientItem, error) {
	var it MealIngredientItem
	err := r.db.QueryRowContext(ctx, `
		SELECT mi.id, mi.meal_id, mi.ingredient_id, mi.quantity, mi.quantity_unit, mi.sort_order, m.name, m.day_of_week
		FROM meal_items mi
		JOIN meals m ON m.id = mi.meal_id
		JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE mi.id = ? AND wp.user_id = ? AND mi.ingredient_id IS NOT NULL`, itemID, userID,
	).Scan(&it.ID, &it.MealID, &it.IngredientID, &it.Quantity, &it.QuantityUnit, &it.SortOrder, &it.MealName, &it.DayOfWeek)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMealItemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load meal ingredient item: %w", err)
	}
	return &it, nil
}

// ReplaceItemIngredient puts another ingredient in a plain-ingredient row of a
// meal, with the quantity and unit given (stored as storedUnit says for the new
// ingredient). ErrMealItemNotFound when the row is not an ingredient row in a
// plan of userID; ErrIngredientNotFound when the new one is outside the
// catalog. The row keeps its place in the meal.
func (r *WeeklyPlans) ReplaceItemIngredient(ctx context.Context, itemID, userID, ingredientID int64, quantity float64, unit string) error {
	if quantity <= 0 {
		return fmt.Errorf("quantita' deve essere > 0")
	}
	if _, err := r.IngredientItem(ctx, itemID, userID); err != nil {
		return err
	}
	if err := ingredientExists(ctx, r.db, ingredientID); err != nil {
		return err
	}
	if err := ingredientVisibleTo(ctx, r.db, ingredientID, userID); err != nil {
		return err
	}
	stored, err := storedUnit(ctx, r.db, unit, ingredientID)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE meal_items SET ingredient_id = ?, quantity = ?, quantity_unit = ? WHERE id = ? AND ingredient_id IS NOT NULL`,
		ingredientID, quantity, stored, itemID)
	if err != nil {
		return fmt.Errorf("replace meal_item ingredient: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMealItemNotFound
	}
	// What a person of the household set by hand (#238) was for the old ingredient.
	if _, err := r.db.ExecContext(ctx, `DELETE FROM household_item_quantities WHERE meal_item_id = ?`, itemID); err != nil {
		return fmt.Errorf("clear household quantities: %w", err)
	}
	return nil
}

// ReplaceItem puts another ingredient in a row of recipeID, with the quantity
// and unit given (see storedUnit). ErrRecipeItemNotFound when the row is not in
// a recipe of userID; ErrIngredientNotFound when the new ingredient is outside
// the catalog. The row keeps its place in the recipe.
func (r *Recipes) ReplaceItem(ctx context.Context, recipeID, itemID, userID, ingredientID int64, quantity float64, unit string) error {
	if quantity <= 0 {
		return fmt.Errorf("quantita' deve essere > 0")
	}
	if err := r.assertOwner(ctx, recipeID, userID); err != nil {
		return err
	}
	if err := ingredientExists(ctx, r.db, ingredientID); err != nil {
		return err
	}
	if err := ingredientVisibleTo(ctx, r.db, ingredientID, userID); err != nil {
		return err
	}
	stored, err := storedUnit(ctx, r.db, unit, ingredientID)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE recipe_ingredients SET ingredient_id = ?, quantity = ?, quantity_unit = ? WHERE id = ? AND recipe_id = ?`,
		ingredientID, quantity, stored, itemID, recipeID)
	if err != nil {
		return fmt.Errorf("replace recipe item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRecipeItemNotFound
	}
	return r.touch(ctx, recipeID)
}

// SameCategoryIDs returns the ids of the ingredients userID put in the same
// category as ingredientID (the ingredient itself included), or an empty set
// when it has none for that user.
func (c *Categories) SameCategoryIDs(ctx context.Context, userID, ingredientID int64) (map[int64]bool, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT ingredient_id FROM ingredient_user_categories
		WHERE user_id = ? AND category_id = (
			SELECT category_id FROM ingredient_user_categories WHERE user_id = ? AND ingredient_id = ?)`,
		userID, userID, ingredientID)
	if err != nil {
		return nil, fmt.Errorf("same category: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan same category: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}
