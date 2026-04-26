package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
)

// ErrIngredientNotFound viene restituito dalle Get quando la riga non esiste.
var ErrIngredientNotFound = errors.New("ingrediente non trovato")

// ErrBarcodeAlreadyUsed viene restituito da CreateOFF se il barcode esiste gia'.
// Il chiamante puo' recuperare la riga esistente con GetByBarcode.
var ErrBarcodeAlreadyUsed = errors.New("barcode gia' presente")

// Ingredients e' il repository degli ingredienti.
type Ingredients struct {
	db *sql.DB
}

func NewIngredients(db *sql.DB) *Ingredients {
	return &Ingredients{db: db}
}

// CreateLocal inserisce un nuovo ingrediente con source='local'. brand/createdBy
// possono essere nil. Ritorna l'id assegnato dal DB.
func (r *Ingredients) CreateLocal(
	ctx context.Context,
	name string,
	brand *string,
	unit domain.IngredientUnit,
	kcal, protein, carbs, fat float64,
	createdBy *int64,
) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ingredients (
			source, name, brand, unit,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			created_by
		) VALUES ('local', ?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(name), brand, string(unit), kcal, protein, carbs, fat, createdBy)
	if err != nil {
		return 0, fmt.Errorf("insert ingredient: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// GetByID ritorna un ingrediente o ErrIngredientNotFound.
func (r *Ingredients) GetByID(ctx context.Context, id int64) (*domain.Ingredient, error) {
	row := r.db.QueryRowContext(ctx, selectIngredientCols+` FROM ingredients WHERE id = ?`, id)
	return scanIngredient(row.Scan)
}

// GetByBarcode ritorna l'ingrediente OFF con quel barcode o ErrIngredientNotFound.
func (r *Ingredients) GetByBarcode(ctx context.Context, barcode string) (*domain.Ingredient, error) {
	row := r.db.QueryRowContext(ctx,
		selectIngredientCols+` FROM ingredients WHERE off_barcode = ?`,
		strings.TrimSpace(barcode),
	)
	return scanIngredient(row.Scan)
}

// CreateOFF inserisce un ingrediente importato da OpenFoodFacts. Setta
// `last_verified_at = CURRENT_TIMESTAMP` perche' il dato e' appena stato
// preso da OFF. Se il barcode esiste gia' ritorna ErrBarcodeAlreadyUsed.
func (r *Ingredients) CreateOFF(
	ctx context.Context,
	barcode, name string,
	brand *string,
	unit domain.IngredientUnit,
	kcal, protein, carbs, fat float64,
) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ingredients (
			source, off_barcode, name, brand, unit,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			last_verified_at
		) VALUES ('openfoodfacts', ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`,
		strings.TrimSpace(barcode), strings.TrimSpace(name), brand, string(unit),
		kcal, protein, carbs, fat,
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

// SearchActive cerca ingredienti non superseded che hanno q come substring
// nel nome (case-insensitive). Limit hard-coded a 20 per HTMX search-as-you-type.
func (r *Ingredients) SearchActive(ctx context.Context, q string, limit int) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q = strings.TrimSpace(q)
	pattern := "%" + strings.ToLower(q) + "%"
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE superseded_by IS NULL AND lower(name) LIKE ?
		ORDER BY (source = 'openfoodfacts') DESC, name COLLATE NOCASE
		LIMIT ?`,
		pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search ingredients: %w", err)
	}
	defer rows.Close()
	return collectIngredients(rows)
}

// ListActive ritorna gli ingredienti non superseded ordinati per nome,
// paginati. offset/limit non controllati negativi: il chiamante li clamp-a.
func (r *Ingredients) ListActive(ctx context.Context, offset, limit int) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE superseded_by IS NULL
		ORDER BY name COLLATE NOCASE
		LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list ingredients: %w", err)
	}
	defer rows.Close()
	return collectIngredients(rows)
}

const selectIngredientCols = `SELECT
	id, source, off_barcode, name, brand, unit,
	kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
	created_by, created_at, last_verified_at, superseded_by`

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

// scanFn astrae la firma di Row.Scan / Rows.Scan per condividere il body.
type scanFn func(dest ...any) error

func scanIngredient(scan scanFn) (*domain.Ingredient, error) {
	var ing domain.Ingredient
	var srcStr, unitStr string
	err := scan(
		&ing.ID, &srcStr, &ing.OFFBarcode, &ing.Name, &ing.Brand, &unitStr,
		&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100,
		&ing.CreatedBy, &ing.CreatedAt, &ing.LastVerifiedAt, &ing.SupersededBy,
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
