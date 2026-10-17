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

// ErrIngredientNotFound is returned by the Get methods when the row does not exist.
var ErrIngredientNotFound = errors.New("ingrediente non trovato")

// ErrBarcodeAlreadyUsed is returned by CreateOFF when the barcode already exists.
// The caller can fetch the existing row with GetByBarcode.
var ErrBarcodeAlreadyUsed = errors.New("barcode gia' presente")

// Ingredients is the ingredients repository.
type Ingredients struct {
	db *sql.DB
}

func NewIngredients(db *sql.DB) *Ingredients {
	return &Ingredients{db: db}
}

// CreateLocal inserts a new ingredient with source='local'. brand/createdBy
// and unitWeightG may be nil. It returns the id the DB assigned.
func (r *Ingredients) CreateLocal(
	ctx context.Context,
	name string,
	brand *string,
	unit domain.IngredientUnit,
	kcal, protein, carbs, fat float64,
	createdBy *int64,
	unitWeightG *float64,
	defaultQuantity *float64,
	fiber, sugars, saturatedFat, sodium *float64,
) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ingredients (
			source, name, brand, unit,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			created_by, unit_weight_g, default_quantity,
			fiber_per_100, sugars_per_100, saturated_fat_per_100, sodium_per_100
		) VALUES ('local', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(name), brand, string(unit), kcal, protein, carbs, fat, createdBy, unitWeightG, defaultQuantity,
		fiber, sugars, saturatedFat, sodium)
	if err != nil {
		return 0, fmt.Errorf("insert ingredient: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// Update updates every editable field of an ingredient.
// Valid for local ingredients and for imported OFF ingredients alike.
func (r *Ingredients) Update(
	ctx context.Context,
	id int64,
	name string,
	brand *string,
	unit domain.IngredientUnit,
	kcal, protein, carbs, fat float64,
	unitWeightG, defaultQuantity *float64,
	fiber, sugars, saturatedFat, sodium *float64,
) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE ingredients SET
			name=?, brand=?, unit=?,
			kcal_per_100=?, protein_per_100=?, carbs_per_100=?, fat_per_100=?,
			unit_weight_g=?, default_quantity=?,
			fiber_per_100=?, sugars_per_100=?, saturated_fat_per_100=?, sodium_per_100=?
		WHERE id=?`,
		strings.TrimSpace(name), brand, string(unit),
		kcal, protein, carbs, fat,
		unitWeightG, defaultQuantity,
		fiber, sugars, saturatedFat, sodium,
		id,
	)
	if err != nil {
		return fmt.Errorf("update ingredient: %w", err)
	}
	return nil
}

// GetByID returns an ingredient or ErrIngredientNotFound.
func (r *Ingredients) GetByID(ctx context.Context, id int64) (*domain.Ingredient, error) {
	row := r.db.QueryRowContext(ctx, selectIngredientCols+` FROM ingredients WHERE id = ?`, id)
	return scanIngredient(row.Scan)
}

// VisibleTo returns ErrIngredientNotFound when the ingredient is outside
// viewerID's catalog (see ScopeCatalog).
func (r *Ingredients) VisibleTo(ctx context.Context, id, viewerID int64) error {
	return ingredientVisibleTo(ctx, r.db, id, viewerID)
}

// GetVisible is GetByID for an ingredient in viewerID's catalog.
func (r *Ingredients) GetVisible(ctx context.Context, id, viewerID int64) (*domain.Ingredient, error) {
	if err := r.VisibleTo(ctx, id, viewerID); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// CountLocalBy returns how many ingredients userID made by hand.
func (r *Ingredients) CountLocalBy(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM ingredients WHERE created_by = ? AND source = 'local'`, userID).Scan(&n)
	return n, err
}

// SetLocalBarcode links a barcode to a local ingredient, created by hand
// because the product is not on OpenFoodFacts (the picker's scanner). The
// code lives in the same column as OFF imports, so it stays unique:
// ErrBarcodeAlreadyUsed when it is taken, ErrIngredientNotFound when id is
// not a local ingredient.
func (r *Ingredients) SetLocalBarcode(ctx context.Context, id int64, barcode string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE ingredients SET off_barcode = ? WHERE id = ? AND source = 'local'`,
		strings.TrimSpace(barcode), id)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrBarcodeAlreadyUsed
		}
		return fmt.Errorf("set local barcode: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrIngredientNotFound
	}
	return nil
}

// GetByBarcode returns the ingredient with that barcode (an OFF import, or a
// local one linked with SetLocalBarcode) or ErrIngredientNotFound.
func (r *Ingredients) GetByBarcode(ctx context.Context, barcode string) (*domain.Ingredient, error) {
	row := r.db.QueryRowContext(ctx,
		selectIngredientCols+` FROM ingredients WHERE off_barcode = ?`,
		strings.TrimSpace(barcode),
	)
	return scanIngredient(row.Scan)
}

// CreateOFF inserts an ingredient imported from OpenFoodFacts. It sets
// `last_verified_at = CURRENT_TIMESTAMP` because the data was just taken
// from OFF. When the barcode already exists it returns ErrBarcodeAlreadyUsed.
// servingG (the OFF serving in g/ml, nil when unknown) becomes the quantity
// proposed in the picker (#136). createdBy is whoever imported it (nil for the
// reconciler): they become its author, the only one who edits it (#143).
func (r *Ingredients) CreateOFF(
	ctx context.Context,
	barcode, name string,
	brand *string,
	unit domain.IngredientUnit,
	kcal, protein, carbs, fat float64,
	createdBy *int64,
	unitWeightG, servingG *float64,
	fiber, sugars, saturatedFat, sodium *float64,
) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ingredients (
			source, off_barcode, name, brand, unit,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			created_by, unit_weight_g, default_quantity, last_verified_at,
			fiber_per_100, sugars_per_100, saturated_fat_per_100, sodium_per_100
		) VALUES ('openfoodfacts', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?, ?)
	`,
		strings.TrimSpace(barcode), strings.TrimSpace(name), brand, string(unit),
		kcal, protein, carbs, fat, createdBy, unitWeightG, servingG,
		fiber, sugars, saturatedFat, sodium,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrBarcodeAlreadyUsed
		}
		return 0, fmt.Errorf("insert OFF ingredient: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// SearchActive finds the non-superseded ingredients in viewerID's catalog
// with q as a substring of the name or brand (case-insensitive). Limit
// hard-coded to 20 for HTMX.
func (r *Ingredients) SearchActive(ctx context.Context, viewerID int64, q string, limit int) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q = strings.TrimSpace(q)
	pattern := "%" + strings.ToLower(q) + "%"
	scope, args := inCatalog(r.db, "created_by", viewerID)
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE superseded_by IS NULL
		  AND (source = 'openfoodfacts' OR `+scope+`)
		  AND (lower(name) LIKE ? OR lower(coalesce(brand, '')) LIKE ?)
		ORDER BY (source = 'openfoodfacts') DESC, name COLLATE NOCASE
		LIMIT ?`,
		append(args, pattern, pattern, limit)...,
	)
	if err != nil {
		return nil, fmt.Errorf("search ingredients: %w", err)
	}
	defer rows.Close()
	return collectIngredients(rows)
}

// ListActive returns the non-superseded ingredients in viewerID's catalog
// ordered by name, paged.
func (r *Ingredients) ListActive(ctx context.Context, viewerID int64, offset, limit int) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	scope, args := inCatalog(r.db, "created_by", viewerID)
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE superseded_by IS NULL
		  AND (source = 'openfoodfacts' OR `+scope+`)
		ORDER BY name COLLATE NOCASE
		LIMIT ? OFFSET ?`,
		append(args, limit, offset)...,
	)
	if err != nil {
		return nil, fmt.Errorf("list ingredients: %w", err)
	}
	defer rows.Close()
	return collectIngredients(rows)
}

// LocalsToReconcile returns up to `limit` non-superseded `source='local'`
// ingredients whose `last_verified_at` is older than `since` (or NULL).
// Used by the OFF reconciliation job.
//
// Implementation note: SQLite stores CURRENT_TIMESTAMP as
// "YYYY-MM-DD HH:MM:SS" (UTC). To avoid format differences between the Go
// param (time.Time) and the stored value, `since` is formatted explicitly in
// the same layout, so the lexicographic comparison matches the
// chronological one.
func (r *Ingredients) LocalsToReconcile(ctx context.Context, limit int, since time.Time) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	threshold := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE source = 'local'
		  AND superseded_by IS NULL
		  AND off_barcode IS NULL -- con codice e' locale apposta (SetLocalBarcode)
		  AND (last_verified_at IS NULL OR last_verified_at < ?)
		ORDER BY last_verified_at IS NOT NULL, last_verified_at, id
		LIMIT ?`,
		threshold, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("locals to reconcile: %w", err)
	}
	defer rows.Close()
	return collectIngredients(rows)
}

// MarkSuperseded marks `localID` as superseded by `byID`. Safe: it updates
// only when the row was source='local' (an OFF row cannot be superseded).
func (r *Ingredients) MarkSuperseded(ctx context.Context, localID, byID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE ingredients
		 SET superseded_by = ?, last_verified_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND source = 'local'`,
		byID, localID,
	)
	if err != nil {
		return fmt.Errorf("mark superseded: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIngredientNotFound
	}
	return nil
}

// TouchVerified sets last_verified_at to CURRENT_TIMESTAMP so a local is
// skipped by reconciliation for N days even when no match was found.
func (r *Ingredients) TouchVerified(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE ingredients SET last_verified_at = CURRENT_TIMESTAMP WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("touch verified: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIngredientNotFound
	}
	return nil
}

const selectIngredientCols = `SELECT
	id, source, off_barcode, name, brand, unit,
	kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
	created_by, created_at, last_verified_at, superseded_by,
	unit_weight_g, default_quantity,
	fiber_per_100, sugars_per_100, saturated_fat_per_100, sodium_per_100`

func collectIngredients(rows *sql.Rows) ([]*domain.Ingredient, error) {
	out := make([]*domain.Ingredient, 0, 16)
	for rows.Next() {
		ing, err := scanIngredient(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, ing)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ingredients: %w", err)
	}
	return out, nil
}

// scanFn abstracts the Row.Scan / Rows.Scan signature to share the body.
type scanFn func(dest ...any) error

func scanIngredient(scan scanFn) (*domain.Ingredient, error) {
	var ing domain.Ingredient
	var srcStr, unitStr string
	err := scan(
		&ing.ID, &srcStr, &ing.OFFBarcode, &ing.Name, &ing.Brand, &unitStr,
		&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100,
		&ing.CreatedBy, &ing.CreatedAt, &ing.LastVerifiedAt, &ing.SupersededBy,
		&ing.UnitWeightG, &ing.DefaultQuantity,
		&ing.FiberPer100, &ing.SugarsPer100, &ing.SaturatedFatPer100, &ing.SodiumPer100,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIngredientNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan ingredient: %w", err)
	}
	ing.Source = domain.IngredientSource(srcStr)
	ing.Unit = domain.IngredientUnit(unitStr)
	return &ing, nil
}
