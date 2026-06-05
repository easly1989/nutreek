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
// e unitWeightG possono essere nil. Ritorna l'id assegnato dal DB.
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

// Update aggiorna tutti i campi modificabili di un ingrediente.
// Valido sia per ingredienti locali che per ingredienti OFF importati.
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
	unitWeightG *float64,
	fiber, sugars, saturatedFat, sodium *float64,
) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ingredients (
			source, off_barcode, name, brand, unit,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			unit_weight_g, last_verified_at,
			fiber_per_100, sugars_per_100, saturated_fat_per_100, sodium_per_100
		) VALUES ('openfoodfacts', ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?, ?)
	`,
		strings.TrimSpace(barcode), strings.TrimSpace(name), brand, string(unit),
		kcal, protein, carbs, fat, unitWeightG,
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

// SearchActive cerca ingredienti non superseded che hanno q come substring
// nel nome o nel brand (case-insensitive). Limit hard-coded a 20 per HTMX.
func (r *Ingredients) SearchActive(ctx context.Context, q string, limit int) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q = strings.TrimSpace(q)
	pattern := "%" + strings.ToLower(q) + "%"
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE superseded_by IS NULL
		  AND (lower(name) LIKE ? OR lower(coalesce(brand, '')) LIKE ?)
		ORDER BY (source = 'openfoodfacts') DESC, name COLLATE NOCASE
		LIMIT ?`,
		pattern, pattern, limit,
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

// LocalsToReconcile ritorna fino a `limit` ingredienti `source='local'`
// non superseded la cui `last_verified_at` e' anteriore a `since` (o NULL).
// Usato dal job di reconciliation OFF.
//
// Note implementative: SQLite memorizza CURRENT_TIMESTAMP nel formato
// "YYYY-MM-DD HH:MM:SS" (UTC). Per evitare differenze di formato fra il
// param Go (time.Time) e quello stored, formattiamo `since` esplicitamente
// nello stesso layout, cosi' il confronto lessicografico coincide con
// quello cronologico.
func (r *Ingredients) LocalsToReconcile(ctx context.Context, limit int, since time.Time) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	threshold := since.UTC().Format("2006-01-02 15:04:05")
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE source = 'local'
		  AND superseded_by IS NULL
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

// MarkSuperseded segna `localID` come soppiantato da `byID`. Sicuro: aggiorna
// solo se la riga era source='local' (un OFF non puo' essere superseded).
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

// TouchVerified aggiorna last_verified_at a CURRENT_TIMESTAMP per skip-pare
// la riconciliazione di un local in N giorni anche se non ha trovato match.
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

// scanFn astrae la firma di Row.Scan / Rows.Scan per condividere il body.
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
