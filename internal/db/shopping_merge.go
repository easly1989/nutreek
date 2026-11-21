package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// ErrShoppingMergeInvalid is returned when two rows cannot be merged: not both
// in the list, in units that do not add up, or a manual row asked to absorb a
// row from the meals.
var ErrShoppingMergeInvalid = errors.New("these two items cannot be merged")

// ParseRowKey splits a row key: kind 'i' (an ingredient of the meals) or 'u'
// (an item added by hand), and its id.
func ParseRowKey(key string) (kind byte, id int64, ok bool) {
	if len(key) < 2 || (key[0] != 'i' && key[0] != 'u') {
		return 0, 0, false
	}
	n, err := strconv.ParseInt(key[1:], 10, 64)
	// Only the canonical spelling: «i05» and «i+5» are not a second name for «i5».
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != key[1:] {
		return 0, 0, false
	}
	return key[0], n, true
}

// PairKey names a pair of rows whatever the order they are given in.
func PairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// DismissSimilar remembers that the user wants two rows kept apart, so the list
// does not offer to merge them again.
func (r *ShoppingLists) DismissSimilar(ctx context.Context, listID, userID int64, keyA, keyB string) error {
	if _, _, ok := ParseRowKey(keyA); !ok {
		return ErrShoppingMergeInvalid
	}
	if _, _, ok := ParseRowKey(keyB); !ok {
		return ErrShoppingMergeInvalid
	}
	if _, err := r.LoadFull(ctx, listID, userID); err != nil { // ownership
		return err
	}
	if keyA > keyB {
		keyA, keyB = keyB, keyA
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO shopping_similar_dismissed (shopping_list_id, key_a, key_b) VALUES (?, ?, ?)`,
		listID, keyA, keyB); err != nil {
		return fmt.Errorf("dismiss similar: %w", err)
	}
	return nil
}

// IngredientUsage counts, for each ingredient, the rows of the user's weekly
// plans that use it: the more used of two similar ingredients is the one the
// merge proposes to keep.
func (r *ShoppingLists) IngredientUsage(ctx context.Context, userID int64, ids []int64) (map[int64]int, error) {
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT mi.ingredient_id, COUNT(*) FROM meal_items mi
		JOIN meals m ON m.id = mi.meal_id
		JOIN weekly_plans p ON p.id = m.weekly_plan_id
		WHERE p.user_id = ? AND mi.ingredient_id IS NOT NULL
		GROUP BY mi.ingredient_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("count ingredient usage: %w", err)
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan ingredient usage: %w", err)
		}
		if want[id] {
			out[id] = n
		}
	}
	return out, rows.Err()
}

// MergeRows merges the row dropKey into the row keepKey of a list.
//   - two ingredients of the meals: the weekly plan of the weeks the list covers
//     is changed to use keepKey's ingredient, and the list remembers the merge,
//     so what comes from a recipe is merged too;
//   - an item added by hand into an ingredient of the meals: its quantity is
//     added to that row and the item goes;
//   - two items added by hand: the quantities are added into keepKey.
//
// Both rows must be in units that add up (g with g, ml with ml, or pieces of an
// ingredient with a weight per piece).
func (r *ShoppingLists) MergeRows(ctx context.Context, listID, userID int64, keepKey, dropKey string) (*MergeUndo, error) {
	kKind, kID, kOK := ParseRowKey(keepKey)
	dKind, dID, dOK := ParseRowKey(dropKey)
	if !kOK || !dOK || (kKind == dKind && kID == dID) {
		return nil, ErrShoppingMergeInvalid
	}
	full, err := r.LoadFull(ctx, listID, userID)
	if err != nil {
		return nil, err
	}
	autoByIng := map[int64]domain.ShoppingListItem{}
	for _, it := range full.Items {
		autoByIng[it.IngredientID] = it
	}
	userByID := map[int64]domain.ShoppingUserItem{}
	for _, it := range full.UserItems {
		userByID[it.ID] = it
	}

	switch {
	case kKind == 'i' && dKind == 'i':
		keep, ok1 := autoByIng[kID]
		drop, ok2 := autoByIng[dID]
		if !ok1 || !ok2 || keep.Ingredient == nil || drop.Ingredient == nil || keep.Ingredient.Unit != drop.Ingredient.Unit {
			return nil, ErrShoppingMergeInvalid
		}
		undo, err := r.mergeIngredients(ctx, full, userID, keep.Ingredient, drop.Ingredient)
		if err != nil {
			return nil, err
		}
		return undo, r.Generate(ctx, listID, userID)

	case kKind == 'i' && dKind == 'u':
		keep, ok1 := autoByIng[kID]
		drop, ok2 := userByID[dID]
		if !ok1 || !ok2 || keep.Ingredient == nil || drop.Quantity <= 0 {
			return nil, ErrShoppingMergeInvalid
		}
		if drop.Unit == "pz" && keep.Ingredient.PieceWeight() <= 0 || drop.Unit != "pz" && drop.Unit != string(keep.Ingredient.Unit) {
			return nil, ErrShoppingMergeInvalid
		}
		add := macros.Grams(drop.Quantity, drop.Unit, keep.Ingredient)
		undo := &MergeUndo{Kind: "manual-auto", ListID: listID, AutoID: keep.ID, AutoQty: keep.TotalQuantity, Dropped: &drop}
		return undo, r.inTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `UPDATE shopping_list_items SET total_quantity = total_quantity + ? WHERE id = ?`, add, keep.ID); err != nil {
				return fmt.Errorf("add quantity: %w", err)
			}
			_, err := tx.ExecContext(ctx, `DELETE FROM shopping_user_items WHERE id = ?`, drop.ID)
			return err
		})

	case kKind == 'u' && dKind == 'u':
		keep, ok1 := userByID[kID]
		drop, ok2 := userByID[dID]
		if !ok1 || !ok2 || keep.Quantity <= 0 || drop.Quantity <= 0 || keep.Unit != drop.Unit {
			return nil, ErrShoppingMergeInvalid
		}
		sum := math.Round((keep.Quantity+drop.Quantity)*1000) / 1000
		undo := &MergeUndo{Kind: "manual-manual", ListID: listID, KeepID: keep.ID, KeepQty: keep.Quantity, KeepText: keep.QuantityText, Dropped: &drop}
		return undo, r.inTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `UPDATE shopping_user_items SET quantity = ?, quantity_text = ? WHERE id = ?`,
				sum, formatManualQuantity(sum, keep.Unit), keep.ID); err != nil {
				return fmt.Errorf("add quantity: %w", err)
			}
			_, err := tx.ExecContext(ctx, `DELETE FROM shopping_user_items WHERE id = ?`, drop.ID)
			return err
		})
	}
	return nil, ErrShoppingMergeInvalid
}

func (r *ShoppingLists) inTx(ctx context.Context, f func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// mergeIngredients records that drop is now keep for this list and rewrites the
// rows of the plan, in the weeks the list covers, that use drop. Meals with a
// cheat logged are left alone: the list does not count them either. A quantity in
// pieces keeps its weight: 2 pieces of 150 g become the pieces of keep that
// weigh the same, or grams when keep has no weight per piece.
func (r *ShoppingLists) mergeIngredients(ctx context.Context, full *domain.ShoppingListFull, userID int64, keep, drop *domain.Ingredient) (*MergeUndo, error) {
	first := full.WeekStart.Format("2006-01-02")
	last := full.WeekStart.AddDate(0, 0, 7*(full.WeekCount-1)).Format("2006-01-02")
	undo := &MergeUndo{Kind: "plan", ListID: full.ID, From: drop.ID, To: keep.ID}
	return undo, r.inTx(ctx, func(tx *sql.Tx) error {
		// What the merge is about to change, for the undo.
		var prev sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT to_ingredient_id FROM shopping_list_merges WHERE shopping_list_id = ? AND from_ingredient_id = ?`, full.ID, drop.ID).Scan(&prev); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read previous merge: %w", err)
		}
		undo.PrevTo = prev.Int64
		pointing, err := tx.QueryContext(ctx, `SELECT from_ingredient_id FROM shopping_list_merges WHERE shopping_list_id = ? AND to_ingredient_id = ?`, full.ID, drop.ID)
		if err != nil {
			return fmt.Errorf("read merges into the dropped ingredient: %w", err)
		}
		for pointing.Next() {
			var id int64
			if err := pointing.Scan(&id); err != nil {
				pointing.Close()
				return fmt.Errorf("scan merge: %w", err)
			}
			undo.Retargeted = append(undo.Retargeted, id)
		}
		pointing.Close()
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO shopping_list_merges (shopping_list_id, from_ingredient_id, to_ingredient_id) VALUES (?, ?, ?)`,
			full.ID, drop.ID, keep.ID); err != nil {
			return fmt.Errorf("remember merge: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE shopping_list_merges SET to_ingredient_id = ? WHERE shopping_list_id = ? AND to_ingredient_id = ?`,
			keep.ID, full.ID, drop.ID); err != nil {
			return fmt.Errorf("follow merge: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT mi.id, mi.quantity, mi.quantity_unit FROM meal_items mi
			JOIN meals m ON m.id = mi.meal_id
			JOIN weekly_plans p ON p.id = m.weekly_plan_id
			WHERE p.user_id = ? AND mi.ingredient_id = ? AND substr(p.week_start, 1, 10) BETWEEN ? AND ?
			  AND NOT EXISTS (SELECT 1 FROM meal_cheat_items c WHERE c.meal_id = mi.meal_id)`,
			userID, drop.ID, first, last)
		if err != nil {
			return fmt.Errorf("find plan rows: %w", err)
		}
		type change struct {
			id   int64
			qty  float64
			unit string
		}
		var changes []change
		for rows.Next() {
			var id int64
			var q float64
			var unit string
			if err := rows.Scan(&id, &q, &unit); err != nil {
				rows.Close()
				return fmt.Errorf("scan plan row: %w", err)
			}
			undo.Rows = append(undo.Rows, UndoPlanRow{ID: id, IngredientID: drop.ID, Quantity: q, Unit: unit})
			if unit == "pz" {
				wd, wk := drop.PieceWeight(), keep.PieceWeight()
				switch {
				case wd > 0 && wk > 0:
					q = q * wd / wk
				case wk <= 0 && wd > 0:
					q, unit = q*wd, string(keep.Unit)
				case wk <= 0:
					unit = string(keep.Unit)
				}
			}
			changes = append(changes, change{id, q, unit})
		}
		rows.Close()
		for _, c := range changes {
			if _, err := tx.ExecContext(ctx,
				`UPDATE meal_items SET ingredient_id = ?, quantity = ?, quantity_unit = ? WHERE id = ?`,
				keep.ID, c.qty, c.unit, c.id); err != nil {
				return fmt.Errorf("update plan row: %w", err)
			}
		}
		return nil
	})
}

// loadMerges returns the list's remembered merges: dropped ingredient to kept one.
func (r *ShoppingLists) loadMerges(ctx context.Context, listID int64) (map[int64]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT from_ingredient_id, to_ingredient_id FROM shopping_list_merges WHERE shopping_list_id = ?`, listID)
	if err != nil {
		return nil, fmt.Errorf("load merges: %w", err)
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var from, to int64
		if err := rows.Scan(&from, &to); err != nil {
			return nil, fmt.Errorf("scan merge: %w", err)
		}
		if from != to {
			out[from] = to
		}
	}
	return out, rows.Err()
}

// loadDismissed returns the pairs of rows the user keeps apart, by PairKey.
func (r *ShoppingLists) loadDismissed(ctx context.Context, listID int64) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT key_a, key_b FROM shopping_similar_dismissed WHERE shopping_list_id = ?`, listID)
	if err != nil {
		return nil, fmt.Errorf("load dismissed pairs: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, fmt.Errorf("scan dismissed pair: %w", err)
		}
		out[PairKey(a, b)] = true
	}
	return out, rows.Err()
}
