package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// The three answers of the check before shopping (#235).
const (
	PrecheckHave = "have" // I have it: the row is set aside (ignored)
	PrecheckSome = "some" // I have some: the quantity goes down by what the user has
	PrecheckNeed = "need" // I need it all: the row counts again, as it is
)

// ErrShoppingPrecheckInvalid means an answer that cannot be applied: an unknown
// row or answer, no quantity for «some», or a unit the row cannot take.
var ErrShoppingPrecheckInvalid = errors.New("invalid pre-shopping answer")

// PrecheckChoice is what the user said about one row of the list.
type PrecheckChoice struct {
	Key   string  // the row's key: «i<ingredient id>» or «u<id>»
	State string  // PrecheckHave, PrecheckSome or PrecheckNeed
	Have  float64 // PrecheckSome: how much the user has, in Unit
	Unit  string  // g, ml or pz
}

// ApplyPrecheck applies the user's answers to a list in one go, or none of
// them. «Have it» sets the row aside; «some» lowers its quantity by what the
// user has, and sets it aside when that covers all of it; «need it all» makes a
// row set aside count again. Rows are never deleted.
func (r *ShoppingLists) ApplyPrecheck(ctx context.Context, listID, userID int64, choices []PrecheckChoice) error {
	full, err := r.LoadFull(ctx, listID, userID)
	if err != nil {
		return err
	}
	auto := map[int64]int{}
	for i, it := range full.Items {
		auto[it.IngredientID] = i
	}
	manual := map[int64]int{}
	for i, it := range full.UserItems {
		manual[it.ID] = i
	}
	return r.inTx(ctx, func(tx *sql.Tx) error {
		seen := map[string]bool{}
		for _, c := range choices {
			if seen[c.Key] {
				continue
			}
			seen[c.Key] = true
			kind, id, ok := ParseRowKey(c.Key)
			if !ok || (c.State != PrecheckHave && c.State != PrecheckSome && c.State != PrecheckNeed) {
				return ErrShoppingPrecheckInvalid
			}
			ignored := c.State == PrecheckHave
			qty := -1.0 // -1: the quantity stays
			if kind == 'i' {
				i, found := auto[id]
				if !found {
					continue // the list changed since the page was shown
				}
				it := full.Items[i]
				if c.State == PrecheckSome {
					have, err := haveInBaseUnit(c, string(it.Ingredient.Unit), it.Ingredient.PieceWeight())
					if err != nil {
						return err
					}
					if left := it.TotalQuantity - have; left > 0.05 {
						qty = left
					} else {
						ignored = true
					}
				}
				if _, err := tx.ExecContext(ctx, `UPDATE shopping_list_items SET ignored = ?, total_quantity = CASE WHEN ? >= 0 THEN ? ELSE total_quantity END WHERE id = ? AND shopping_list_id = ?`,
					boolInt(ignored), qty, qty, it.ID, listID); err != nil {
					return fmt.Errorf("apply pre-shopping answer: %w", err)
				}
				continue
			}
			i, found := manual[id]
			if !found {
				continue
			}
			it := full.UserItems[i]
			text := ""
			if c.State == PrecheckSome {
				if it.Quantity <= 0 || it.Unit == "" || c.Unit != it.Unit || c.Have <= 0 || c.Have > maxShoppingQuantity {
					return ErrShoppingPrecheckInvalid
				}
				if left := it.Quantity - c.Have; left > 0.005 {
					qty, text = left, formatManualQuantity(left, it.Unit)
				} else {
					ignored = true
				}
			}
			if qty >= 0 {
				if _, err := tx.ExecContext(ctx, `UPDATE shopping_user_items SET ignored = ?, quantity = ?, quantity_text = ? WHERE id = ? AND shopping_list_id = ?`,
					boolInt(ignored), qty, text, it.ID, listID); err != nil {
					return fmt.Errorf("apply pre-shopping answer: %w", err)
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE shopping_user_items SET ignored = ? WHERE id = ? AND shopping_list_id = ?`, boolInt(ignored), it.ID, listID); err != nil {
				return fmt.Errorf("apply pre-shopping answer: %w", err)
			}
		}
		return nil
	})
}

// haveInBaseUnit reads what the user has of an item from the meals in the
// item's own unit (g or ml): pieces count by the weight of a piece, which the
// ingredient must have.
func haveInBaseUnit(c PrecheckChoice, baseUnit string, pieceWeight float64) (float64, error) {
	if c.Have <= 0 || c.Have > maxShoppingQuantity {
		return 0, ErrShoppingPrecheckInvalid
	}
	switch strings.TrimSpace(c.Unit) {
	case "pz":
		if pieceWeight <= 0 {
			return 0, ErrShoppingPrecheckInvalid
		}
		return c.Have * pieceWeight, nil
	case baseUnit:
		return c.Have, nil
	}
	return 0, ErrShoppingPrecheckInvalid
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
