package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/amon2126/nutreek/internal/domain"
)

var ErrCategoryNotFound = errors.New("categoria non trovata")

type Categories struct {
	db *sql.DB
}

func NewCategories(db *sql.DB) *Categories {
	return &Categories{db: db}
}

func (c *Categories) ListCategories(ctx context.Context, userID int64) ([]domain.IngredientCategory, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT id, user_id, name, sort_order FROM ingredient_categories
		 WHERE user_id = ? ORDER BY sort_order, name COLLATE NOCASE`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var cats []domain.IngredientCategory
	for rows.Next() {
		var cat domain.IngredientCategory
		if err := rows.Scan(&cat.ID, &cat.UserID, &cat.Name, &cat.SortOrder); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}

func (c *Categories) CreateCategory(ctx context.Context, userID int64, name string) (int64, error) {
	res, err := c.db.ExecContext(ctx,
		`INSERT INTO ingredient_categories (user_id, name) VALUES (?, ?)`,
		userID, name,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, fmt.Errorf("categoria già esistente")
		}
		return 0, fmt.Errorf("create category: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (c *Categories) DeleteCategory(ctx context.Context, id, userID int64) error {
	res, err := c.db.ExecContext(ctx,
		`DELETE FROM ingredient_categories WHERE id = ? AND user_id = ?`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCategoryNotFound
	}
	return nil
}

func (c *Categories) ListAllergens(ctx context.Context) ([]domain.AllergenTag, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT id, name FROM allergen_tags ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list allergens: %w", err)
	}
	defer rows.Close()

	var tags []domain.AllergenTag
	for rows.Next() {
		var t domain.AllergenTag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan allergen: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// GetIngredientAllergenIDs ritorna gli ID degli allergeni associati a un ingrediente.
func (c *Categories) GetIngredientAllergenIDs(ctx context.Context, ingredientID int64) (map[int64]bool, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT allergen_id FROM ingredient_allergens WHERE ingredient_id = ?`, ingredientID,
	)
	if err != nil {
		return nil, fmt.Errorf("get ingredient allergens: %w", err)
	}
	defer rows.Close()

	ids := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan allergen id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// SetIngredientAllergens sostituisce gli allergeni di un ingrediente.
func (c *Categories) SetIngredientAllergens(ctx context.Context, ingredientID int64, allergenIDs []int64) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM ingredient_allergens WHERE ingredient_id = ?`, ingredientID,
	); err != nil {
		return fmt.Errorf("delete allergens: %w", err)
	}

	for _, aid := range allergenIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO ingredient_allergens (ingredient_id, allergen_id) VALUES (?, ?)`,
			ingredientID, aid,
		); err != nil {
			return fmt.Errorf("insert allergen: %w", err)
		}
	}

	return tx.Commit()
}

// SetIngredientCategory aggiorna la categoria di un ingrediente (NULL per rimuoverla).
func (c *Categories) SetIngredientCategory(ctx context.Context, ingredientID int64, categoryID *int64) error {
	_, err := c.db.ExecContext(ctx,
		`UPDATE ingredients SET category_id = ? WHERE id = ?`, categoryID, ingredientID,
	)
	if err != nil {
		return fmt.Errorf("set ingredient category: %w", err)
	}
	return nil
}
