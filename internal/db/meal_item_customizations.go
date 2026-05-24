package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

// MealItemCustomizations gestisce le variazioni personalizzate degli ingredienti
// per una singola occorrenza di ricetta in un pasto.
type MealItemCustomizations struct {
	db *sql.DB
}

func NewMealItemCustomizations(db *sql.DB) *MealItemCustomizations {
	return &MealItemCustomizations{db: db}
}

// CustomizationInput e' la coppia ingrediente+quantita' da salvare.
type CustomizationInput struct {
	IngredientID int64
	Quantity     float64
}

// Replace sostituisce atomicamente tutte le personalizzazioni di un meal_item.
// Se rows e' vuoto, rimuove tutte le personalizzazioni esistenti (reset).
// Verifica ownership: l'item deve appartenere a un pasto del piano dell'utente.
func (r *MealItemCustomizations) Replace(
	ctx context.Context,
	mealItemID, userID int64,
	rows []CustomizationInput,
) error {
	// Verifica che meal_item appartenga all'utente tramite la catena
	// meal_items → meals → weekly_plans WHERE user_id = ?.
	var ownerID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT wp.user_id
		FROM meal_items mi
		JOIN meals m ON m.id = mi.meal_id
		JOIN weekly_plans wp ON wp.id = m.weekly_plan_id
		WHERE mi.id = ?
	`, mealItemID).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return ErrMealItemNotFound
	}
	if err != nil {
		return fmt.Errorf("meal_item owner: %w", err)
	}
	if ownerID != userID {
		return ErrMealItemNotFound
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM meal_item_customizations WHERE meal_item_id = ?`, mealItemID,
	); err != nil {
		return fmt.Errorf("clear customizations: %w", err)
	}

	if len(rows) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO meal_item_customizations (meal_item_id, ingredient_id, quantity, sort_order)
			VALUES (?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("prepare insert: %w", err)
		}
		defer stmt.Close()
		for i, row := range rows {
			if _, err := stmt.ExecContext(ctx, mealItemID, row.IngredientID, row.Quantity, i); err != nil {
				return fmt.Errorf("insert customization: %w", err)
			}
		}
	}

	return tx.Commit()
}

// ListForItems carica le personalizzazioni per una lista di meal_item_id,
// inclusi i dati dell'ingrediente. Usato da LoadFull per la dashboard.
// Ritorna una mappa meal_item_id → []MealItemCustomization.
func (r *MealItemCustomizations) ListForItems(
	ctx context.Context,
	mealItemIDs []int64,
) (map[int64][]domain.MealItemCustomization, error) {
	if len(mealItemIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(mealItemIDs))
	for i, id := range mealItemIDs {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT mic.id, mic.meal_item_id, mic.ingredient_id, mic.quantity, mic.sort_order,
		       i.id, i.source, i.off_barcode, i.name, i.brand, i.unit,
		       i.kcal_per_100, i.protein_per_100, i.carbs_per_100, i.fat_per_100,
		       i.created_by, i.created_at, i.last_verified_at, i.superseded_by
		FROM meal_item_customizations mic
		JOIN ingredients i ON i.id = mic.ingredient_id
		WHERE mic.meal_item_id IN (`+buildPlaceholders(len(mealItemIDs))+`)
		ORDER BY mic.meal_item_id, mic.sort_order, mic.id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load customizations: %w", err)
	}
	defer rows.Close()

	out := make(map[int64][]domain.MealItemCustomization)
	for rows.Next() {
		var c domain.MealItemCustomization
		var ing domain.Ingredient
		var src, unit string
		err := rows.Scan(
			&c.ID, &c.MealItemID, &c.IngredientID, &c.Quantity, &c.SortOrder,
			&ing.ID, &src, &ing.OFFBarcode, &ing.Name, &ing.Brand, &unit,
			&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100,
			&ing.CreatedBy, &ing.CreatedAt, &ing.LastVerifiedAt, &ing.SupersededBy,
		)
		if err != nil {
			return nil, fmt.Errorf("scan customization: %w", err)
		}
		ing.Source = domain.IngredientSource(src)
		ing.Unit = domain.IngredientUnit(unit)
		c.Ingredient = &ing
		out[c.MealItemID] = append(out[c.MealItemID], c)
	}
	return out, rows.Err()
}

// buildPlaceholders genera N placeholder "?" separati da virgola.
func buildPlaceholders(n int) string {
	if n == 0 {
		return ""
	}
	b := make([]byte, 0, n*2-1)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}
