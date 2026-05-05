package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrCheatNotFound = sgarro inesistente o non di proprieta' dell'utente.
var ErrCheatNotFound = errors.New("sgarro non trovato")

// AddCheat aggiunge una riga di sgarro a un meal. Recipe XOR ingredient.
// Quantity > 0. SortOrder calcolato come MAX(sort_order)+1 nel meal.
func (r *WeeklyPlans) AddCheat(
	ctx context.Context,
	mealID, userID int64,
	recipeID, ingredientID *int64,
	quantity float64,
) (int64, error) {
	// Verifica meal owner via JOIN su weekly_plans.user_id.
	var planUser int64
	err := r.db.QueryRowContext(ctx, `
		SELECT wp.user_id
		FROM meals m JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE m.id = ?`, mealID,
	).Scan(&planUser)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrMealNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("meal owner: %w", err)
	}
	if planUser != userID {
		return 0, ErrPlanNotFound
	}
	if (recipeID == nil) == (ingredientID == nil) {
		return 0, fmt.Errorf("specifica esattamente una fra recipe_id e ingredient_id")
	}
	if quantity <= 0 {
		return 0, fmt.Errorf("quantita' deve essere > 0")
	}
	var nextOrder int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order)+1, 0) FROM meal_cheat_items WHERE meal_id = ?`,
		mealID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute cheat sort_order: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO meal_cheat_items (meal_id, recipe_id, ingredient_id, quantity, sort_order)
		 VALUES (?, ?, ?, ?, ?)`,
		mealID, recipeID, ingredientID, quantity, nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert cheat: %w", err)
	}
	return res.LastInsertId()
}

// RemoveCheat cancella una singola riga di sgarro (ownership via JOIN).
func (r *WeeklyPlans) RemoveCheat(ctx context.Context, cheatID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM meal_cheat_items
		WHERE id = ?
		  AND meal_id IN (
		    SELECT m.id FROM meals m
		    JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		    WHERE wp.user_id = ?
		  )`, cheatID, userID)
	if err != nil {
		return fmt.Errorf("delete cheat: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCheatNotFound
	}
	return nil
}

// ClearCheats rimuove tutte le righe di sgarro per il meal indicato.
// Riporta il pasto allo stato "ufficiale" (le sue macro tornano dagli items).
func (r *WeeklyPlans) ClearCheats(ctx context.Context, mealID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM meal_cheat_items
		WHERE meal_id = ?
		  AND meal_id IN (
		    SELECT m.id FROM meals m
		    JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		    WHERE wp.user_id = ? AND m.id = ?
		  )`, mealID, userID, mealID)
	if err != nil {
		return fmt.Errorf("clear cheats: %w", err)
	}
	_, _ = res.RowsAffected()
	return nil
}

// ReorderCheats persiste l'ordine degli sgarri di un meal. Stesso pattern
// di ReorderMealItems: ids deve coincidere con l'insieme corrente.
func (r *WeeklyPlans) ReorderCheats(ctx context.Context, mealID, userID int64, ids []int64) error {
	var planUser int64
	err := r.db.QueryRowContext(ctx, `
		SELECT wp.user_id
		FROM meals m JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE m.id = ?`, mealID,
	).Scan(&planUser)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMealNotFound
	}
	if err != nil {
		return fmt.Errorf("meal owner: %w", err)
	}
	if planUser != userID {
		return ErrPlanNotFound
	}
	currentIDs, err := scanInt64Set(ctx, r.db,
		`SELECT id FROM meal_cheat_items WHERE meal_id = ?`, mealID)
	if err != nil {
		return fmt.Errorf("load current cheats: %w", err)
	}
	if !sameInt64Set(currentIDs, ids) {
		return ErrReorderMismatch
	}
	return runReorderTx(ctx, r.db,
		`UPDATE meal_cheat_items SET sort_order = ? WHERE id = ? AND meal_id = ?`,
		ids, mealID)
}
