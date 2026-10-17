package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
)

var ErrCategoryNotFound = errors.New("categoria non trovata")

// ErrCategoryExists: the user already has a category with that name.
var ErrCategoryExists = errors.New("category already exists")

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
			return 0, ErrCategoryExists
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
		`SELECT id, COALESCE(code, ''), name FROM allergen_tags ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list allergens: %w", err)
	}
	defer rows.Close()

	var tags []domain.AllergenTag
	for rows.Next() {
		var t domain.AllergenTag
		if err := rows.Scan(&t.ID, &t.Code, &t.Name); err != nil {
			return nil, fmt.Errorf("scan allergen: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// GetIngredientAllergenIDs returns the IDs of the allergens tagged on an ingredient.
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

// SetIngredientAllergens replaces an ingredient's allergens.
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

// AllergensForIngredients loads the allergens of many ingredients in one batch,
// avoiding the N+1 pattern in pickers and lists. It returns a map of
// ingredient_id -> allergens (sorted by name). Ingredients without allergens
// are simply absent from the map.
func (c *Categories) AllergensForIngredients(ctx context.Context, ingredientIDs []int64) (map[int64][]domain.AllergenTag, error) {
	out := make(map[int64][]domain.AllergenTag)
	if len(ingredientIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(ingredientIDs))
	args := make([]any, len(ingredientIDs))
	for i, id := range ingredientIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := c.db.QueryContext(ctx,
		`SELECT ia.ingredient_id, at.id, COALESCE(at.code, ''), at.name
		   FROM ingredient_allergens ia
		   JOIN allergen_tags at ON at.id = ia.allergen_id
		  WHERE ia.ingredient_id IN (`+strings.Join(placeholders, ",")+`)
		  ORDER BY at.name`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("allergens for ingredients: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ingID int64
		var tag domain.AllergenTag
		if err := rows.Scan(&ingID, &tag.ID, &tag.Code, &tag.Name); err != nil {
			return nil, fmt.Errorf("scan allergen row: %w", err)
		}
		out[ingID] = append(out[ingID], tag)
	}
	return out, rows.Err()
}

// CategoriesForIngredients loads in one batch the category userID gave to
// many ingredients: ingredient_id -> category. Ingredients with no category
// for that user are absent; other users' choices do not count.
func (c *Categories) CategoriesForIngredients(ctx context.Context, userID int64, ingredientIDs []int64) (map[int64]domain.IngredientCategory, error) {
	out := make(map[int64]domain.IngredientCategory)
	if len(ingredientIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ingredientIDs))
	args := make([]any, 0, len(ingredientIDs)+1)
	args = append(args, userID)
	for i, id := range ingredientIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT uc.ingredient_id, ic.id, ic.user_id, ic.name, ic.sort_order
		   FROM ingredient_user_categories uc
		   JOIN ingredient_categories ic ON ic.id = uc.category_id
		  WHERE uc.user_id = ? AND uc.ingredient_id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("categories for ingredients: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ingID int64
		var cat domain.IngredientCategory
		if err := rows.Scan(&ingID, &cat.ID, &cat.UserID, &cat.Name, &cat.SortOrder); err != nil {
			return nil, fmt.Errorf("scan category row: %w", err)
		}
		out[ingID] = cat
	}
	return out, rows.Err()
}

// IngredientIDsByAllergen returns the set of ingredient IDs tagged with one
// allergen. Used by the filter of the /ingredients list.
func (c *Categories) IngredientIDsByAllergen(ctx context.Context, allergenID int64) (map[int64]bool, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT ingredient_id FROM ingredient_allergens WHERE allergen_id = ?`, allergenID,
	)
	if err != nil {
		return nil, fmt.Errorf("ingredient ids by allergen: %w", err)
	}
	defer rows.Close()

	ids := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan ingredient id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// SetIngredientCategory puts an ingredient in one of userID's categories (nil
// removes it). It applies to that user only; a category that is not theirs
// answers ErrCategoryNotFound and changes nothing.
func (c *Categories) SetIngredientCategory(ctx context.Context, userID, ingredientID int64, categoryID *int64) error {
	if categoryID == nil {
		if _, err := c.db.ExecContext(ctx,
			`DELETE FROM ingredient_user_categories WHERE user_id = ? AND ingredient_id = ?`,
			userID, ingredientID,
		); err != nil {
			return fmt.Errorf("clear ingredient category: %w", err)
		}
		return nil
	}
	res, err := c.db.ExecContext(ctx,
		`INSERT INTO ingredient_user_categories (user_id, ingredient_id, category_id)
		 SELECT ?, ?, id FROM ingredient_categories WHERE id = ? AND user_id = ?
		 ON CONFLICT (user_id, ingredient_id) DO UPDATE SET category_id = excluded.category_id`,
		userID, ingredientID, *categoryID, userID,
	)
	if err != nil {
		return fmt.Errorf("set ingredient category: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCategoryNotFound
	}
	return nil
}
