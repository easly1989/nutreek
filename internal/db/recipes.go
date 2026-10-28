package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

// ErrRecipeNotFound is returned by the Get methods when the recipe does not exist.
var ErrRecipeNotFound = errors.New("ricetta non trovata")

// ErrRecipeForbidden reports an access that is not allowed (e.g. a user
// trying to update/delete a recipe that is not theirs).
var ErrRecipeForbidden = errors.New("operazione non consentita su questa ricetta")

// ErrRecipeItemNotFound means the recipe_ingredients item does not exist or is not the user's.
var ErrRecipeItemNotFound = errors.New("voce ricetta non trovata")

// ErrRecipeNameTaken reports that the same user already has a recipe with the
// same name (case-insensitive). Enforced both in the DB (UNIQUE INDEX) and in
// the handlers, for readable messages.
var ErrRecipeNameTaken = errors.New("esiste già una ricetta con questo nome")

// ErrRecipeIdenticalContent reports that the recipe's content (set of
// ingredients+quantities) matches another recipe of the user.
var ErrRecipeIdenticalContent = errors.New("esiste già una ricetta con gli stessi ingredienti e quantità")

// Recipes is the recipes repository.
type Recipes struct {
	db *sql.DB
}

func NewRecipes(db *sql.DB) *Recipes {
	return &Recipes{db: db}
}

// Create inserts a new empty recipe (items are added later).
// It returns the assigned id. ErrRecipeNameTaken when the same user already
// has a recipe with the same name (case-insensitive).
func (r *Recipes) Create(
	ctx context.Context,
	userID int64,
	name string, description *string,
	servings int, isPublic bool,
) (int64, error) {
	if servings <= 0 {
		servings = 1
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO recipes (user_id, name, description, servings, is_public)
		VALUES (?, ?, ?, ?, ?)
	`, userID, strings.TrimSpace(name), description, servings, boolToInt(isPublic))
	if err != nil {
		if isUniqueRecipeNameErr(err) {
			return 0, ErrRecipeNameTaken
		}
		return 0, fmt.Errorf("insert recipe: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// recipeVisibleTo checks that the user may use the recipe in an item: their
// own (drafts included) or a confirmed public one in their catalog. Otherwise
// ErrRecipeNotFound, without telling a missing recipe from another user's
// private one.
func recipeVisibleTo(ctx context.Context, db *sql.DB, recipeID, userID int64) error {
	scope, args := inCatalog(db, "user_id", userID)
	var ok int
	err := db.QueryRowContext(ctx, `
		SELECT 1 FROM recipes
		WHERE id = ? AND (user_id = ? OR (is_public = 1 AND is_draft = 0 AND `+scope+`))`,
		append([]any{recipeID, userID}, args...)...,
	).Scan(&ok)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecipeNotFound
	}
	if err != nil {
		return fmt.Errorf("recipe visibility: %w", err)
	}
	return nil
}

// Update changes the base fields. Checks the owner: when userID is not the
// owner it returns ErrRecipeForbidden without touching the row.
func (r *Recipes) Update(
	ctx context.Context,
	id, userID int64,
	name string, description *string,
	servings int, isPublic bool,
) error {
	if servings <= 0 {
		servings = 1
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE recipes
		SET name = ?, description = ?, servings = ?, is_public = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, strings.TrimSpace(name), description, servings, boolToInt(isPublic), id, userID)
	if err != nil {
		if isUniqueRecipeNameErr(err) {
			return ErrRecipeNameTaken
		}
		return fmt.Errorf("update recipe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return r.notFoundOrForbidden(ctx, id, userID)
	}
	return nil
}

// Delete deletes the recipe (its items and photo go through ON DELETE CASCADE).
func (r *Recipes) Delete(ctx context.Context, id, userID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM recipes WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete recipe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return r.notFoundOrForbidden(ctx, id, userID)
	}
	return nil
}

// notFoundOrForbidden tells 404 from 403 when an UPDATE/DELETE touches no rows.
func (r *Recipes) notFoundOrForbidden(ctx context.Context, id, userID int64) error {
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM recipes WHERE id = ?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecipeNotFound
	}
	if err != nil {
		return fmt.Errorf("check owner: %w", err)
	}
	if owner != userID {
		return ErrRecipeForbidden
	}
	// The owner matches but RowsAffected=0: should not happen → fallback.
	return ErrRecipeNotFound
}

// GetWithItems loads the recipe + every item joined with its ingredient.
// For another user's recipe it still returns the data when isPublic;
// otherwise ErrRecipeForbidden. ErrRecipeNotFound when the id does not exist
// or the recipe is outside the viewer's catalog (see ScopeCatalog).
func (r *Recipes) GetWithItems(ctx context.Context, id, viewerID int64) (*domain.RecipeWithItems, error) {
	scope, args := inCatalog(r.db, "r.user_id", viewerID)
	row := r.db.QueryRowContext(ctx, `
		SELECT r.id, r.user_id, r.name, r.description, r.servings, r.is_public,
		       r.is_draft, r.created_at, r.updated_at, u.display_name, (`+scope+`)
		FROM recipes r
		JOIN users u ON u.id = r.user_id
		WHERE r.id = ?`, append(args, id)...)
	var (
		out                domain.RecipeWithItems
		isPubInt, draftInt int
		inScope            bool
	)
	err := row.Scan(
		&out.ID, &out.UserID, &out.Name, &out.Description, &out.Servings, &isPubInt,
		&draftInt, &out.CreatedAt, &out.UpdatedAt, &out.AuthorName, &inScope,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecipeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan recipe: %w", err)
	}
	out.IsPublic = isPubInt == 1
	out.IsDraft = draftInt == 1
	if out.UserID != viewerID && !inScope {
		return nil, ErrRecipeNotFound
	}
	if out.UserID != viewerID && !out.IsPublic {
		return nil, ErrRecipeForbidden
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT ri.id, ri.recipe_id, ri.ingredient_id, ri.quantity, ri.sort_order,
		       i.id, i.source, i.off_barcode, i.name, i.brand, i.unit,
		       i.kcal_per_100, i.protein_per_100, i.carbs_per_100, i.fat_per_100,
		       i.created_by, i.created_at, i.last_verified_at, i.superseded_by
		FROM recipe_ingredients ri
		JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE ri.recipe_id = ?
		ORDER BY ri.sort_order, ri.id
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it domain.RecipeItemFull
		var srcStr, unitStr string
		err := rows.Scan(
			&it.ID, &it.RecipeID, &it.IngredientID, &it.Quantity, &it.SortOrder,
			&it.Ingredient.ID, &srcStr, &it.Ingredient.OFFBarcode, &it.Ingredient.Name,
			&it.Ingredient.Brand, &unitStr,
			&it.Ingredient.KcalPer100, &it.Ingredient.ProteinPer100,
			&it.Ingredient.CarbsPer100, &it.Ingredient.FatPer100,
			&it.Ingredient.CreatedBy, &it.Ingredient.CreatedAt,
			&it.Ingredient.LastVerifiedAt, &it.Ingredient.SupersededBy,
		)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		it.Ingredient.Source = domain.IngredientSource(srcStr)
		it.Ingredient.Unit = domain.IngredientUnit(unitStr)
		out.Items = append(out.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate items: %w", err)
	}
	return &out, nil
}

// ListVisible returns the recipes visible to viewerID: all of theirs + the
// public ones in their catalog. Drafts (unconfirmed) are excluded. Sorted by name.
func (r *Recipes) ListVisible(ctx context.Context, viewerID int64) ([]*domain.Recipe, error) {
	scope, args := inCatalog(r.db, "user_id", viewerID)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, servings, is_public, is_draft, created_at, updated_at
		FROM recipes
		WHERE is_draft = 0 AND (user_id = ? OR (is_public = 1 AND `+scope+`))
		ORDER BY name COLLATE NOCASE
	`, append([]any{viewerID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("list visible recipes: %w", err)
	}
	defer rows.Close()
	out := make([]*domain.Recipe, 0, 16)
	for rows.Next() {
		var rec domain.Recipe
		var pubInt, draftInt int
		err := rows.Scan(&rec.ID, &rec.UserID, &rec.Name, &rec.Description,
			&rec.Servings, &pubInt, &draftInt, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan recipe: %w", err)
		}
		rec.IsPublic = pubInt == 1
		rec.IsDraft = draftInt == 1
		out = append(out, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// AddItem inserts a new ingredient into the recipe. sortOrder defaults to
// max(sort_order)+1. Checks the owner.
func (r *Recipes) AddItem(
	ctx context.Context,
	recipeID, userID, ingredientID int64,
	quantity float64,
) (int64, error) {
	if err := r.assertOwner(ctx, recipeID, userID); err != nil {
		return 0, err
	}
	if err := ingredientVisibleTo(ctx, r.db, ingredientID, userID); err != nil {
		return 0, err
	}
	if quantity <= 0 {
		return 0, fmt.Errorf("quantita' deve essere > 0")
	}
	var nextOrder int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order)+1, 0) FROM recipe_ingredients WHERE recipe_id = ?`,
		recipeID,
	).Scan(&nextOrder); err != nil {
		return 0, fmt.Errorf("compute sort_order: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, sort_order)
		VALUES (?, ?, ?, ?)
	`, recipeID, ingredientID, quantity, nextOrder)
	if err != nil {
		return 0, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	// touch updated_at
	if _, err := r.db.ExecContext(ctx,
		`UPDATE recipes SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, recipeID); err != nil {
		return 0, fmt.Errorf("touch recipe: %w", err)
	}
	return id, nil
}

// RemoveItem deletes an item; checks that the item belongs to a recipe
// of userID.
func (r *Recipes) RemoveItem(ctx context.Context, itemID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM recipe_ingredients
		WHERE id = ? AND recipe_id IN (SELECT id FROM recipes WHERE user_id = ?)
	`, itemID, userID)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRecipeItemNotFound
	}
	return nil
}

func (r *Recipes) assertOwner(ctx context.Context, recipeID, userID int64) error {
	var owner int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM recipes WHERE id = ?`, recipeID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRecipeNotFound
	}
	if err != nil {
		return fmt.Errorf("assert owner: %w", err)
	}
	if owner != userID {
		return ErrRecipeForbidden
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CountBy returns how many recipes userID has, drafts included.
func (r *Recipes) CountBy(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM recipes WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// NameExists reports whether the user already has a recipe with this name
// (case-insensitive). excludeID = an id to skip in the check (useful on update).
func (r *Recipes) NameExists(ctx context.Context, userID int64, name string, excludeID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recipes
		WHERE user_id = ? AND name = ? COLLATE NOCASE AND id != ?
	`, userID, strings.TrimSpace(name), excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("name exists: %w", err)
	}
	return n > 0, nil
}

// CountMealUsages returns the number of meal_items referencing this recipe.
// Used to warn before deleting it.
func (r *Recipes) CountMealUsages(ctx context.Context, recipeID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meal_items WHERE recipe_id = ?`, recipeID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count meal usages: %w", err)
	}
	return n, nil
}

// IdenticalContentRecipe checks whether another recipe of the same user has
// exactly the same set of (ingredient_id, quantity) ingredients.
// It returns that recipe's name (or "") and nil when there is none.
// excludeID = an id to skip (useful for duplicate/update).
//
// Implementation: two recipes are "identical" iff they have the same number
// of items and every (ingredient_id, quantity) matches. The comparison uses a
// serialized "id:qty;id:qty;..." signature over sorted tuples.
func (r *Recipes) IdenticalContentRecipe(
	ctx context.Context,
	userID int64,
	items []domain.RecipeItem,
	excludeID int64,
) (string, error) {
	target := contentSignature(items)
	if target == "" {
		// A recipe without ingredients is never identical to another one.
		return "", nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name FROM recipes WHERE user_id = ? AND id != ?
	`, userID, excludeID)
	if err != nil {
		return "", fmt.Errorf("list recipes for identity: %w", err)
	}
	defer rows.Close()
	type pair struct {
		id   int64
		name string
	}
	var candidates []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.name); err != nil {
			return "", fmt.Errorf("scan candidate: %w", err)
		}
		candidates = append(candidates, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	for _, c := range candidates {
		sig, err := r.signatureForRecipe(ctx, c.id)
		if err != nil {
			return "", err
		}
		if sig == target {
			return c.name, nil
		}
	}
	return "", nil
}

// signatureForRecipe reads the recipe's items and returns the normalized signature.
func (r *Recipes) signatureForRecipe(ctx context.Context, recipeID int64) (string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ingredient_id, quantity FROM recipe_ingredients WHERE recipe_id = ?
	`, recipeID)
	if err != nil {
		return "", fmt.Errorf("load items for signature: %w", err)
	}
	defer rows.Close()
	var items []domain.RecipeItem
	for rows.Next() {
		var it domain.RecipeItem
		if err := rows.Scan(&it.IngredientID, &it.Quantity); err != nil {
			return "", fmt.Errorf("scan item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return contentSignature(items), nil
}

// contentSignature builds a normalized signature of a list of items:
// sorted, concatenated (ingredient_id, quantity rounded to 3 decimals) tuples.
// Two lists with the same set of tuples → same signature (whatever the sort_order).
func contentSignature(items []domain.RecipeItem) string {
	if len(items) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(items))
	for _, it := range items {
		pairs = append(pairs, fmt.Sprintf("%d:%.3f", it.IngredientID, it.Quantity))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ";")
}

// Duplicate deep-copies a recipe (metadata + every recipe_ingredients row +
// its photo) under the given new name. Transactional.
//
// Errors:
//   - ErrRecipeNotFound: the source does not exist
//   - ErrRecipeForbidden: the source is not userID's and is not public
//   - ErrRecipeNameTaken: the user already has that name
func (r *Recipes) Duplicate(ctx context.Context, srcID, userID int64, newName string) (int64, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return 0, errors.New("nome ricetta vuoto")
	}
	src, err := r.GetWithItems(ctx, srcID, userID)
	if err != nil {
		return 0, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var descPtr *string
	if src.Description != nil {
		s := *src.Description
		descPtr = &s
	}
	// is_draft = 1: the copy starts as an unconfirmed draft. It is promoted on
	// the first ingredient add/remove that makes it differ from the other recipes.
	res, err := tx.ExecContext(ctx, `
		INSERT INTO recipes (user_id, name, description, servings, is_public, is_draft)
		VALUES (?, ?, ?, ?, ?, 1)
	`, userID, newName, descPtr, src.Servings, boolToInt(src.IsPublic))
	if err != nil {
		if isUniqueRecipeNameErr(err) {
			return 0, ErrRecipeNameTaken
		}
		return 0, fmt.Errorf("insert duplicate: %w", err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	for _, it := range src.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity, sort_order)
			VALUES (?, ?, ?, ?)
		`, newID, it.IngredientID, it.Quantity, it.SortOrder); err != nil {
			return 0, fmt.Errorf("copy item: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recipe_photos (recipe_id, photo, thumb, etag)
		SELECT ?, photo, thumb, etag FROM recipe_photos WHERE recipe_id = ?
	`, newID, srcID); err != nil {
		return 0, fmt.Errorf("copy photo: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit duplicate: %w", err)
	}
	return newID, nil
}

// BulkDelete deletes several of the user's recipes. It returns how many were
// actually deleted (the others were forbidden / missing, and are ignored).
// The meal_items linked to the deleted recipes are removed by cascade
// (FK ON DELETE CASCADE, see migration 0008).
func (r *Recipes) BulkDelete(ctx context.Context, ids []int64, userID int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, userID)
	q := `DELETE FROM recipes WHERE id IN (` + placeholders + `) AND user_id = ?`
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("bulk delete recipes: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// UsagesByRecipe returns a map recipe_id → number of meal_items referencing
// it, limited to the user's recipes. Recipes with no uses are absent from the
// map (the caller can assume 0).
func (r *Recipes) UsagesByRecipe(ctx context.Context, userID int64) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT mi.recipe_id, COUNT(*) AS n
		FROM meal_items mi
		JOIN recipes r ON r.id = mi.recipe_id
		WHERE r.user_id = ? AND mi.recipe_id IS NOT NULL
		GROUP BY mi.recipe_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("usages by recipe: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]int, 8)
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan usage: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// PerServingByRecipe returns the per-serving macros of every recipe visible
// to viewerID (theirs + the public ones in their catalog, drafts excluded) with one
// aggregate query. Recipe ingredient quantities are already in g/ml ("pz"
// is converted on insert). Recipes without ingredients appear with zero
// macros.
func (r *Recipes) PerServingByRecipe(ctx context.Context, viewerID int64) (map[int64]macros.Macros, error) {
	scope, args := inCatalog(r.db, "r.user_id", viewerID)
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.servings,
		       COALESCE(SUM(ri.quantity * i.kcal_per_100), 0) / 100.0,
		       COALESCE(SUM(ri.quantity * i.protein_per_100), 0) / 100.0,
		       COALESCE(SUM(ri.quantity * i.carbs_per_100), 0) / 100.0,
		       COALESCE(SUM(ri.quantity * i.fat_per_100), 0) / 100.0
		FROM recipes r
		LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
		LEFT JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE r.is_draft = 0 AND (r.user_id = ? OR (r.is_public = 1 AND `+scope+`))
		GROUP BY r.id, r.servings
	`, append([]any{viewerID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("per serving by recipe: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]macros.Macros, 16)
	for rows.Next() {
		var (
			id       int64
			servings int
			total    macros.Macros
		)
		if err := rows.Scan(&id, &servings, &total.Kcal, &total.Protein, &total.Carbs, &total.Fat); err != nil {
			return nil, fmt.Errorf("scan recipe macros: %w", err)
		}
		out[id] = macros.PerServing(total, servings)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CountMealUsagesMulti sums the meal_items occurrences over several recipes.
func (r *Recipes) CountMealUsagesMulti(ctx context.Context, recipeIDs []int64) (int, error) {
	if len(recipeIDs) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(recipeIDs)), ",")
	args := make([]any, 0, len(recipeIDs))
	for _, id := range recipeIDs {
		args = append(args, id)
	}
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM meal_items WHERE recipe_id IN (`+placeholders+`)`, args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count meal usages multi: %w", err)
	}
	return n, nil
}

// PromoteDraft clears the is_draft flag. Idempotent: calling it on a recipe
// that is not a draft has no effect. Checks ownership.
func (r *Recipes) PromoteDraft(ctx context.Context, id, userID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE recipes SET is_draft = 0, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND is_draft = 1
	`, id, userID)
	if err != nil {
		return fmt.Errorf("promote draft: %w", err)
	}
	_ = res
	return nil
}

// isUniqueRecipeNameErr detects a violation of the uniq_recipes_user_name constraint.
// modernc.org/sqlite returns the error as text containing the index name.
func isUniqueRecipeNameErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "uniq_recipes_user_name") ||
		(strings.Contains(s, "UNIQUE constraint failed") && strings.Contains(s, "recipes"))
}
