package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

// MergeUndo is what a merge changed, enough to put it back (UndoMerge). It is
// plain data so the caller can keep it between requests.
type MergeUndo struct {
	Kind   string // "plan", "manual-auto" or "manual-manual"
	ListID int64
	// "plan": the plan rows that were rewritten and the merge records.
	Rows       []UndoPlanRow
	From, To   int64   // ingredient dropped and kept
	PrevTo     int64   // where From pointed before, 0 when it did not
	Retargeted []int64 // merges that pointed at From and now point at To
	// "manual-auto" and "manual-manual": the row that grew and the item that went.
	AutoIngredient int64 // the row from the meals that grew, by ingredient: a regenerate changes row ids
	AutoQty        float64
	KeepID         int64
	KeepQty        float64
	KeepText       string
	Dropped        *domain.ShoppingUserItem
}

// UndoPlanRow is a plan row as it was before a merge rewrote it.
type UndoPlanRow struct {
	ID           int64
	IngredientID int64
	Quantity     float64
	Unit         string
}

// UndoMerge puts back what MergeRows changed: the plan rows, the merge records
// and the items go back as they were, and the list is generated again.
func (r *ShoppingLists) UndoMerge(ctx context.Context, listID, userID int64, u *MergeUndo) error {
	if u == nil || u.ListID != listID {
		return ErrShoppingMergeInvalid
	}
	if _, err := r.LoadFull(ctx, listID, userID); err != nil { // ownership
		return err
	}
	switch u.Kind {
	case "plan":
		err := r.inTx(ctx, func(tx *sql.Tx) error {
			for _, row := range u.Rows {
				if _, err := tx.ExecContext(ctx, `
					UPDATE meal_items SET ingredient_id = ?, quantity = ?, quantity_unit = ?
					WHERE id = ? AND ingredient_id = ? AND meal_id IN (SELECT m.id FROM meals m JOIN weekly_plans p ON p.id = m.weekly_plan_id WHERE p.user_id = ?)`,
					row.IngredientID, row.Quantity, row.Unit, row.ID, u.To, userID); err != nil {
					return fmt.Errorf("restore plan row: %w", err)
				}
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM shopping_list_merges WHERE shopping_list_id = ? AND from_ingredient_id = ?`, listID, u.From); err != nil {
				return fmt.Errorf("forget merge: %w", err)
			}
			if u.PrevTo != 0 {
				if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO shopping_list_merges (shopping_list_id, from_ingredient_id, to_ingredient_id) VALUES (?, ?, ?)`, listID, u.From, u.PrevTo); err != nil {
					return fmt.Errorf("restore previous merge: %w", err)
				}
			}
			for _, from := range u.Retargeted {
				if _, err := tx.ExecContext(ctx, `UPDATE shopping_list_merges SET to_ingredient_id = ? WHERE shopping_list_id = ? AND from_ingredient_id = ?`, u.From, listID, from); err != nil {
					return fmt.Errorf("restore retargeted merge: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		return r.Generate(ctx, listID, userID)

	case "manual-auto", "manual-manual":
		if u.Dropped == nil {
			return ErrShoppingMergeInvalid
		}
		d := u.Dropped
		return r.inTx(ctx, func(tx *sql.Tx) error {
			if u.Kind == "manual-auto" {
				if _, err := tx.ExecContext(ctx, `UPDATE shopping_list_items SET total_quantity = ? WHERE ingredient_id = ? AND shopping_list_id = ?`, u.AutoQty, u.AutoIngredient, listID); err != nil {
					return fmt.Errorf("restore quantity: %w", err)
				}
			} else {
				var text *string
				if u.KeepText != "" {
					text = &u.KeepText
				}
				if _, err := tx.ExecContext(ctx, `UPDATE shopping_user_items SET quantity = ?, quantity_text = ? WHERE id = ? AND shopping_list_id = ?`, u.KeepQty, text, u.KeepID, listID); err != nil {
					return fmt.Errorf("restore quantity: %w", err)
				}
			}
			var text *string
			if d.QuantityText != "" {
				text = &d.QuantityText
			}
			var qty *float64
			var unit *string
			if d.Quantity > 0 && d.Unit != "" {
				qty, unit = &d.Quantity, &d.Unit
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO shopping_user_items (shopping_list_id, description, quantity_text, quantity, unit, checked, ignored, sort_order, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				listID, d.Description, text, qty, unit, d.Checked, d.Ignored, d.SortOrder, d.CreatedAt); err != nil {
				return fmt.Errorf("restore the item: %w", err)
			}
			return nil
		})
	}
	return ErrShoppingMergeInvalid
}
