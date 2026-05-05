package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
)

// Errori del repo shopping.
var (
	ErrShoppingListNotFound = errors.New("lista della spesa non trovata")
	ErrShoppingItemNotFound = errors.New("voce della lista non trovata")
)

// ShoppingLists e' il repo delle liste della spesa.
type ShoppingLists struct {
	db    *sql.DB
	plans *WeeklyPlans
}

// NewShoppingLists costruisce il repo. Riceve anche WeeklyPlans perche'
// Generate ha bisogno di caricare i piani settimanali per aggregare.
func NewShoppingLists(db *sql.DB, plans *WeeklyPlans) *ShoppingLists {
	return &ShoppingLists{db: db, plans: plans}
}

// GetOrCreateList ritorna la lista per (user, weekStart). Se non esiste
// la crea con weekCount specificato; se esiste e weekCount differisce
// dal record, aggiorna in DB (l'utente ha cambiato il numero di
// settimane via dropdown).
func (r *ShoppingLists) GetOrCreateList(ctx context.Context, userID int64, weekStart time.Time, weekCount int) (*domain.ShoppingList, error) {
	if weekCount < 1 {
		weekCount = 1
	}
	if weekCount > 4 {
		weekCount = 4
	}
	weekStart = MondayOf(weekStart)
	dateStr := weekStart.Format("2006-01-02")

	if _, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO shopping_lists (user_id, week_start, week_count) VALUES (?, ?, ?)`,
		userID, dateStr, weekCount,
	); err != nil {
		return nil, fmt.Errorf("upsert shopping list: %w", err)
	}
	// Aggiorna week_count se il record esisteva con valore diverso.
	if _, err := r.db.ExecContext(ctx,
		`UPDATE shopping_lists SET week_count = ? WHERE user_id = ? AND week_start = ? AND week_count != ?`,
		weekCount, userID, dateStr, weekCount,
	); err != nil {
		return nil, fmt.Errorf("update shopping list week_count: %w", err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, week_start, week_count, created_at
		 FROM shopping_lists WHERE user_id = ? AND week_start = ?`,
		userID, dateStr,
	)
	var list domain.ShoppingList
	var dateScan string
	if err := row.Scan(&list.ID, &list.UserID, &dateScan, &list.WeekCount, &list.CreatedAt); err != nil {
		return nil, fmt.Errorf("read shopping list: %w", err)
	}
	if t, err := time.Parse("2006-01-02", strings.Split(dateScan, "T")[0]); err == nil {
		list.WeekStart = t
	} else {
		list.WeekStart = weekStart
	}
	return &list, nil
}

// Generate ricalcola gli shopping_list_items aggregando gli ingredienti
// dei piani delle weekCount settimane consecutive a partire da
// list.WeekStart. Ignora i meal_cheat_items (gli sgarri sono gia'
// consumati). Espande le ricette in ingredienti via
// RecipeItem.Quantity * (mealItem.Quantity / Recipe.Servings). Aggrega
// per ingredient_id sommando le quantita'. DELETE-then-INSERT degli
// items, ma preserva checked=true se l'ingrediente esisteva con la
// stessa quantita' (entro epsilon 0.01).
func (r *ShoppingLists) Generate(ctx context.Context, listID, userID int64) error {
	// Ownership + carica lista.
	var owner int64
	var weekStartStr string
	var weekCount int
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, week_start, week_count FROM shopping_lists WHERE id = ?`, listID,
	).Scan(&owner, &weekStartStr, &weekCount)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrShoppingListNotFound
	}
	if err != nil {
		return fmt.Errorf("load shopping list: %w", err)
	}
	if owner != userID {
		return ErrShoppingListNotFound
	}
	weekStart, err := time.Parse("2006-01-02", strings.Split(weekStartStr, "T")[0])
	if err != nil {
		return fmt.Errorf("parse week_start: %w", err)
	}
	if weekCount < 1 {
		weekCount = 1
	}

	// Aggrega ingredienti su tutte le settimane coperte.
	totals := make(map[int64]float64) // ingredientID -> quantita' totale
	for w := 0; w < weekCount; w++ {
		ws := weekStart.AddDate(0, 0, 7*w)
		plan, err := r.plans.GetOrCreateForWeek(ctx, userID, ws)
		if err != nil {
			return fmt.Errorf("load plan week %s: %w", ws.Format("2006-01-02"), err)
		}
		full, err := r.plans.LoadFull(ctx, plan)
		if err != nil {
			return fmt.Errorf("load full week %s: %w", ws.Format("2006-01-02"), err)
		}
		for d := 0; d < 7; d++ {
			for _, meal := range full.Days[d] {
				if meal.HasCheats() {
					continue // sgarri ignorati: gia' consumati, non vanno comprati
				}
				for _, it := range meal.Items {
					if it.Ingredient != nil {
						totals[it.Ingredient.ID] += it.Quantity
					}
					if it.Recipe != nil && it.Recipe.Servings > 0 {
						scale := it.Quantity / float64(it.Recipe.Servings)
						for _, ri := range it.RecipeItems {
							totals[ri.IngredientID] += ri.Quantity * scale
						}
					}
				}
			}
		}
	}

	// Carica i checked esistenti per merge.
	existing := make(map[int64]struct {
		Quantity float64
		Checked  bool
	})
	rows, err := r.db.QueryContext(ctx,
		`SELECT ingredient_id, total_quantity, checked
		 FROM shopping_list_items WHERE shopping_list_id = ?`, listID,
	)
	if err != nil {
		return fmt.Errorf("load existing items: %w", err)
	}
	for rows.Next() {
		var ingID int64
		var qty float64
		var checked int
		if err := rows.Scan(&ingID, &qty, &checked); err != nil {
			rows.Close()
			return fmt.Errorf("scan existing item: %w", err)
		}
		existing[ingID] = struct {
			Quantity float64
			Checked  bool
		}{qty, checked == 1}
	}
	rows.Close()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM shopping_list_items WHERE shopping_list_id = ?`, listID,
	); err != nil {
		return fmt.Errorf("clear items: %w", err)
	}

	if len(totals) == 0 {
		return tx.Commit()
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO shopping_list_items (shopping_list_id, ingredient_id, total_quantity, checked, sort_order) VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	idx := 0
	for ingID, qty := range totals {
		// Preserva checked se quantita' identica (epsilon 0.01).
		checked := 0
		if prev, ok := existing[ingID]; ok && prev.Checked {
			if diff := qty - prev.Quantity; diff < 0.01 && diff > -0.01 {
				checked = 1
			}
		}
		if _, err := stmt.ExecContext(ctx, listID, ingID, qty, checked, idx); err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
		idx++
	}
	return tx.Commit()
}

// LoadFull carica la lista, gli items auto (con Ingredient resolved
// alfabeticamente) e gli items utente. Ownership obbligatoria.
func (r *ShoppingLists) LoadFull(ctx context.Context, listID, userID int64) (*domain.ShoppingListFull, error) {
	var list domain.ShoppingList
	var dateScan string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, week_start, week_count, created_at
		 FROM shopping_lists WHERE id = ?`, listID,
	).Scan(&list.ID, &list.UserID, &dateScan, &list.WeekCount, &list.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShoppingListNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load list: %w", err)
	}
	if list.UserID != userID {
		return nil, ErrShoppingListNotFound
	}
	if t, err := time.Parse("2006-01-02", strings.Split(dateScan, "T")[0]); err == nil {
		list.WeekStart = t
	}
	out := &domain.ShoppingListFull{ShoppingList: list}

	// Items auto + ingredienti in JOIN, ordinati alfabeticamente per nome.
	itemRows, err := r.db.QueryContext(ctx, `
		SELECT sli.id, sli.shopping_list_id, sli.ingredient_id, sli.total_quantity, sli.checked, sli.sort_order,
		       i.id, i.source, i.off_barcode, i.name, i.brand, i.unit,
		       i.kcal_per_100, i.protein_per_100, i.carbs_per_100, i.fat_per_100,
		       i.created_by, i.created_at, i.last_verified_at, i.superseded_by
		FROM shopping_list_items sli
		JOIN ingredients i ON i.id = sli.ingredient_id
		WHERE sli.shopping_list_id = ?
		ORDER BY LOWER(i.name), i.id
	`, listID)
	if err != nil {
		return nil, fmt.Errorf("load list items: %w", err)
	}
	for itemRows.Next() {
		var it domain.ShoppingListItem
		var ing domain.Ingredient
		var src, unit string
		var checked int
		err := itemRows.Scan(
			&it.ID, &it.ShoppingListID, &it.IngredientID, &it.TotalQuantity, &checked, &it.SortOrder,
			&ing.ID, &src, &ing.OFFBarcode, &ing.Name, &ing.Brand, &unit,
			&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100,
			&ing.CreatedBy, &ing.CreatedAt, &ing.LastVerifiedAt, &ing.SupersededBy,
		)
		if err != nil {
			itemRows.Close()
			return nil, fmt.Errorf("scan list item: %w", err)
		}
		it.Checked = checked == 1
		ing.Source = domain.IngredientSource(src)
		ing.Unit = domain.IngredientUnit(unit)
		it.Ingredient = &ing
		out.Items = append(out.Items, it)
	}
	itemRows.Close()

	// Items utente.
	userRows, err := r.db.QueryContext(ctx, `
		SELECT id, shopping_list_id, description, COALESCE(quantity_text, ''), checked, sort_order, created_at
		FROM shopping_user_items
		WHERE shopping_list_id = ?
		ORDER BY sort_order, id
	`, listID)
	if err != nil {
		return nil, fmt.Errorf("load user items: %w", err)
	}
	for userRows.Next() {
		var it domain.ShoppingUserItem
		var checked int
		if err := userRows.Scan(&it.ID, &it.ShoppingListID, &it.Description, &it.QuantityText, &checked, &it.SortOrder, &it.CreatedAt); err != nil {
			userRows.Close()
			return nil, fmt.Errorf("scan user item: %w", err)
		}
		it.Checked = checked == 1
		out.UserItems = append(out.UserItems, it)
	}
	userRows.Close()

	return out, nil
}

// ToggleItem inverte lo stato checked di un item auto.
func (r *ShoppingLists) ToggleItem(ctx context.Context, itemID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE shopping_list_items
		SET checked = CASE checked WHEN 1 THEN 0 ELSE 1 END
		WHERE id = ?
		  AND shopping_list_id IN (SELECT id FROM shopping_lists WHERE user_id = ?)
	`, itemID, userID)
	if err != nil {
		return fmt.Errorf("toggle item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrShoppingItemNotFound
	}
	return nil
}

// AddUserItem inserisce un item manuale. Description obbligatoria, qty
// libera. SortOrder = MAX+1 per la lista.
func (r *ShoppingLists) AddUserItem(ctx context.Context, listID, userID int64, description, qtyText string) (int64, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return 0, fmt.Errorf("descrizione obbligatoria")
	}
	// Ownership.
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM shopping_lists WHERE id = ?`, listID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrShoppingListNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("list owner: %w", err)
	}
	if owner != userID {
		return 0, ErrShoppingListNotFound
	}
	var nextOrder int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order)+1, 0) FROM shopping_user_items WHERE shopping_list_id = ?`,
		listID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	var qtyPtr *string
	if s := strings.TrimSpace(qtyText); s != "" {
		qtyPtr = &s
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO shopping_user_items (shopping_list_id, description, quantity_text, sort_order) VALUES (?, ?, ?, ?)`,
		listID, description, qtyPtr, nextOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("insert user item: %w", err)
	}
	return res.LastInsertId()
}

// ToggleUserItem inverte checked su un item manuale.
func (r *ShoppingLists) ToggleUserItem(ctx context.Context, itemID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE shopping_user_items
		SET checked = CASE checked WHEN 1 THEN 0 ELSE 1 END
		WHERE id = ?
		  AND shopping_list_id IN (SELECT id FROM shopping_lists WHERE user_id = ?)
	`, itemID, userID)
	if err != nil {
		return fmt.Errorf("toggle user item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrShoppingItemNotFound
	}
	return nil
}

// RemoveUserItem cancella un item manuale.
func (r *ShoppingLists) RemoveUserItem(ctx context.Context, itemID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM shopping_user_items
		WHERE id = ?
		  AND shopping_list_id IN (SELECT id FROM shopping_lists WHERE user_id = ?)
	`, itemID, userID)
	if err != nil {
		return fmt.Errorf("delete user item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrShoppingItemNotFound
	}
	return nil
}

// ReorderUserItems persiste l'ordine degli items utente di una lista.
func (r *ShoppingLists) ReorderUserItems(ctx context.Context, listID, userID int64, ids []int64) error {
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM shopping_lists WHERE id = ?`, listID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrShoppingListNotFound
	}
	if err != nil {
		return fmt.Errorf("list owner: %w", err)
	}
	if owner != userID {
		return ErrShoppingListNotFound
	}
	currentIDs, err := scanInt64Set(ctx, r.db,
		`SELECT id FROM shopping_user_items WHERE shopping_list_id = ?`, listID)
	if err != nil {
		return fmt.Errorf("load current user items: %w", err)
	}
	if !sameInt64Set(currentIDs, ids) {
		return ErrReorderMismatch
	}
	return runReorderTx(ctx, r.db,
		`UPDATE shopping_user_items SET sort_order = ? WHERE id = ? AND shopping_list_id = ?`,
		ids, listID)
}
