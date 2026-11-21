package db

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// ManualSuggestion is an item the user added by hand before (#234): the text
// and the quantity of the last time, and how many times it appears.
type ManualSuggestion struct {
	Description string
	Quantity    float64 // 0 when the last one had none
	Unit        string
	Uses        int
}

// PastManualList is a past shopping list with the items added by hand to it.
type PastManualList struct {
	ListID    int64
	WeekStart time.Time
	WeekCount int
	Items     []domain.ShoppingUserItem
}

// MaxCopyItems bounds the items one copy takes over.
const MaxCopyItems = 100

// SuggestManualItems returns what the user added by hand before, whose text
// starts with prefix or has a word that does: the most used first, then the
// most recent. One entry per text, whatever its case.
func (r *ShoppingLists) SuggestManualItems(ctx context.Context, userID int64, prefix string, limit int) ([]ManualSuggestion, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" || limit <= 0 {
		return nil, nil
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	rows, err := r.db.QueryContext(ctx, `
		WITH m AS (
			SELECT i.description, i.quantity, i.unit, i.created_at,
			       ROW_NUMBER() OVER (PARTITION BY lower(i.description) ORDER BY i.created_at DESC, i.id DESC) AS rn,
			       COUNT(*) OVER (PARTITION BY lower(i.description)) AS uses
			FROM shopping_user_items i
			JOIN shopping_lists l ON l.id = i.shopping_list_id
			WHERE l.user_id = ?
			  AND (lower(i.description) LIKE ? ESCAPE '\' OR lower(i.description) LIKE ? ESCAPE '\')
		)
		SELECT description, quantity, unit, uses FROM m WHERE rn = 1
		ORDER BY uses DESC, created_at DESC, description LIMIT ?`,
		userID, esc+"%", "% "+esc+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("suggest manual items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ManualSuggestion
	for rows.Next() {
		var s ManualSuggestion
		var qty *float64
		var unit *string
		if err := rows.Scan(&s.Description, &qty, &unit, &s.Uses); err != nil {
			return nil, fmt.Errorf("scan suggestion: %w", err)
		}
		if qty != nil && unit != nil {
			s.Quantity, s.Unit = *qty, *unit
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PastManualLists returns the user's lists of weeks before the given one that
// have items added by hand, the latest first, with those items.
func (r *ShoppingLists) PastManualLists(ctx context.Context, userID int64, before time.Time, limit int) ([]PastManualList, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT l.id, l.week_start, l.week_count
		FROM shopping_lists l
		WHERE l.user_id = ? AND l.week_start < ?
		  AND EXISTS (SELECT 1 FROM shopping_user_items i WHERE i.shopping_list_id = l.id)
		ORDER BY l.week_start DESC LIMIT ?`,
		userID, before.Format("2006-01-02"), limit)
	if err != nil {
		return nil, fmt.Errorf("past manual lists: %w", err)
	}
	var out []PastManualList
	for rows.Next() {
		var p PastManualList
		var date string
		if err := rows.Scan(&p.ListID, &date, &p.WeekCount); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan past list: %w", err)
		}
		p.WeekStart, _ = time.Parse("2006-01-02", strings.Split(date, "T")[0])
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	for i := range out {
		items, err := r.manualItemsOf(ctx, out[i].ListID)
		if err != nil {
			return nil, err
		}
		out[i].Items = items
	}
	return out, nil
}

// manualItemsOf reads the items added by hand to a list, in their order.
func (r *ShoppingLists) manualItemsOf(ctx context.Context, listID int64) ([]domain.ShoppingUserItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, shopping_list_id, description, COALESCE(quantity_text, ''), COALESCE(quantity, 0), COALESCE(unit, ''), checked, ignored, sort_order, created_at
		FROM shopping_user_items WHERE shopping_list_id = ? ORDER BY sort_order, id`, listID)
	if err != nil {
		return nil, fmt.Errorf("load manual items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.ShoppingUserItem
	for rows.Next() {
		var it domain.ShoppingUserItem
		var checked, ignored int
		if err := rows.Scan(&it.ID, &it.ShoppingListID, &it.Description, &it.QuantityText, &it.Quantity, &it.Unit, &checked, &ignored, &it.SortOrder, &it.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan manual item: %w", err)
		}
		it.Checked, it.Ignored = checked == 1, ignored == 1
		out = append(out, it)
	}
	return out, rows.Err()
}

// CopyManualItems adds to a list the items added by hand to the user's other
// lists, keeping their quantity. It returns how many were added. Items of
// another user, of the list itself or that do not exist are skipped.
func (r *ShoppingLists) CopyManualItems(ctx context.Context, listID, userID int64, itemIDs []int64) (int, error) {
	full, err := r.LoadFull(ctx, listID, userID)
	if err != nil {
		return 0, err
	}
	if len(itemIDs) > MaxCopyItems {
		itemIDs = itemIDs[:MaxCopyItems]
	}
	sort.Slice(itemIDs, func(a, b int) bool { return itemIDs[a] < itemIDs[b] })
	have := map[string]bool{} // a text already added by hand to this list is not added twice
	for _, it := range full.UserItems {
		have[strings.ToLower(strings.TrimSpace(it.Description))] = true
	}
	n := 0
	for _, id := range itemIDs {
		var it domain.ShoppingUserItem
		var text *string
		var qty *float64
		var unit *string
		err := r.db.QueryRowContext(ctx, `
			SELECT i.description, i.quantity_text, i.quantity, i.unit
			FROM shopping_user_items i JOIN shopping_lists l ON l.id = i.shopping_list_id
			WHERE i.id = ? AND l.user_id = ? AND l.id <> ?`, id, userID, listID).
			Scan(&it.Description, &text, &qty, &unit)
		if err != nil {
			continue // not found, someone else's, or from this list
		}
		key := strings.ToLower(strings.TrimSpace(it.Description))
		if have[key] {
			continue
		}
		have[key] = true
		if qty != nil && unit != nil && *qty > 0 {
			_, err = r.AddManualItem(ctx, listID, userID, it.Description, *qty, *unit)
		} else {
			t := ""
			if text != nil {
				t = *text
			}
			_, err = r.AddUserItem(ctx, listID, userID, it.Description, t)
		}
		if err != nil {
			return n, fmt.Errorf("copy item: %w", err)
		}
		n++
	}
	return n, nil
}
