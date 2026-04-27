package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrRecipeNotFound e' restituito dalle Get quando la ricetta non esiste.
var ErrRecipeNotFound = errors.New("ricetta non trovata")

// ErrRecipeForbidden segnala un accesso non consentito (es. utente che
// tenta update/delete di una ricetta non sua).
var ErrRecipeForbidden = errors.New("operazione non consentita su questa ricetta")

// ErrRecipeItemNotFound = item recipe_ingredients inesistente / non appartiene.
var ErrRecipeItemNotFound = errors.New("voce ricetta non trovata")

// Recipes e' il repository delle ricette.
type Recipes struct {
	db *sql.DB
}

func NewRecipes(db *sql.DB) *Recipes {
	return &Recipes{db: db}
}

// Create inserisce una nuova ricetta vuota (gli items si aggiungono dopo).
// Ritorna l'id assegnato.
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
		return 0, fmt.Errorf("insert recipe: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// Update modifica i campi base. Verifica owner: se userID non e' il
// proprietario, ritorna ErrRecipeForbidden senza toccare la riga.
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
		return fmt.Errorf("update recipe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return r.notFoundOrForbidden(ctx, id, userID)
	}
	return nil
}

// Delete cancella la ricetta (gli items vanno via per ON DELETE CASCADE).
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

// notFoundOrForbidden distingue 404 da 403 quando UPDATE/DELETE non tocca righe.
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
	// Owner combacia ma RowsAffected=0: non dovrebbe succedere → fallback.
	return ErrRecipeNotFound
}

// GetWithItems carica la ricetta + tutti gli item joinati con l'ingrediente.
// Per ricette altrui ritorna comunque i dati se isPublic; altrimenti
// ErrRecipeForbidden. ErrRecipeNotFound se l'id non esiste.
func (r *Recipes) GetWithItems(ctx context.Context, id, viewerID int64) (*domain.RecipeWithItems, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT r.id, r.user_id, r.name, r.description, r.servings, r.is_public,
		       r.created_at, r.updated_at, u.display_name
		FROM recipes r
		JOIN users u ON u.id = r.user_id
		WHERE r.id = ?`, id)
	var (
		out      domain.RecipeWithItems
		isPubInt int
	)
	err := row.Scan(
		&out.ID, &out.UserID, &out.Name, &out.Description, &out.Servings, &isPubInt,
		&out.CreatedAt, &out.UpdatedAt, &out.AuthorName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRecipeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan recipe: %w", err)
	}
	out.IsPublic = isPubInt == 1
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

// ListVisible ritorna ricette visibili a viewerID: tutte le sue + le pubbliche
// di altri. Ordinate alfabetico per nome.
func (r *Recipes) ListVisible(ctx context.Context, viewerID int64) ([]*domain.Recipe, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, servings, is_public, created_at, updated_at
		FROM recipes
		WHERE user_id = ? OR is_public = 1
		ORDER BY name COLLATE NOCASE
	`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("list visible recipes: %w", err)
	}
	defer rows.Close()
	out := make([]*domain.Recipe, 0, 16)
	for rows.Next() {
		var rec domain.Recipe
		var pubInt int
		err := rows.Scan(&rec.ID, &rec.UserID, &rec.Name, &rec.Description,
			&rec.Servings, &pubInt, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan recipe: %w", err)
		}
		rec.IsPublic = pubInt == 1
		out = append(out, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// AddItem inserisce un nuovo ingrediente nella ricetta. sortOrder default
// = max(sort_order)+1. Verifica owner.
func (r *Recipes) AddItem(
	ctx context.Context,
	recipeID, userID, ingredientID int64,
	quantity float64,
) (int64, error) {
	if err := r.assertOwner(ctx, recipeID, userID); err != nil {
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

// RemoveItem cancella un item; verifica che l'item appartenga a una ricetta
// di userID.
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
