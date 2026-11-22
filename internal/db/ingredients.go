package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/amon2126/nutreek/internal/domain"
)

// likeEscaper makes text literal in a LIKE pattern that uses ESCAPE '\'.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

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

// piecesTables hold quantities in pieces («pz»), counted with the
// ingredient's unit weight.
var piecesTables = []string{"recipe_ingredients", "meal_items", "nutritionist_meal_items"}

// Update updates every editable field of an ingredient.
// Valid for local ingredients and for imported OFF ingredients alike.
// When the unit weight goes away, the rows in pieces of that ingredient are
// turned into grams with the old weight first, so they keep their amount.
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
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if unitWeightG == nil || *unitWeightG <= 0 {
		for _, table := range piecesTables {
			if _, err := tx.ExecContext(ctx, `
				UPDATE `+table+` SET quantity_unit = ?,
					quantity = quantity * (SELECT unit_weight_g FROM ingredients WHERE id = ?)
				WHERE ingredient_id = ? AND quantity_unit = 'pz'
				  AND (SELECT unit_weight_g FROM ingredients WHERE id = ?) > 0`,
				string(unit), id, id, id); err != nil {
				return fmt.Errorf("pieces to grams in %s: %w", table, err)
			}
		}
	}
	// off_aligned stays on only when nothing of what OpenFoodFacts gave changes
	// (the right-hand sides read the row as it was). The numbers compare within
	// a tolerance, so saving a form that shows rounded values changes nothing:
	// 0.01 for kcal, macros and weight, 0.0005 for the micronutrients, which
	// are small (sodium is in grams).
	same := func(col string, tol string) string {
		return "(" + col + " IS NULL AND ? IS NULL OR abs(" + col + " - ?) < " + tol + ")"
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE ingredients SET
			off_aligned = CASE WHEN off_aligned = 1 AND name = ? AND coalesce(brand, '') = coalesce(?, '') AND unit = ?
				AND `+same("kcal_per_100", "0.01")+` AND `+same("protein_per_100", "0.01")+` AND `+same("carbs_per_100", "0.01")+` AND `+same("fat_per_100", "0.01")+`
				AND `+same("unit_weight_g", "0.01")+`
				AND `+same("fiber_per_100", "0.0005")+` AND `+same("sugars_per_100", "0.0005")+` AND `+same("saturated_fat_per_100", "0.0005")+` AND `+same("sodium_per_100", "0.0005")+`
				THEN 1 ELSE 0 END,
			name=?, brand=?, unit=?,
			kcal_per_100=?, protein_per_100=?, carbs_per_100=?, fat_per_100=?,
			unit_weight_g=?, default_quantity=?,
			fiber_per_100=?, sugars_per_100=?, saturated_fat_per_100=?, sodium_per_100=?
		WHERE id=?`,
		strings.TrimSpace(name), brand, string(unit),
		kcal, kcal, protein, protein, carbs, carbs, fat, fat,
		unitWeightG, unitWeightG,
		fiber, fiber, sugars, sugars, saturatedFat, saturatedFat, sodium, sodium,
		strings.TrimSpace(name), brand, string(unit),
		kcal, protein, carbs, fat,
		unitWeightG, defaultQuantity,
		fiber, sugars, saturatedFat, sodium,
		id,
	)
	if err != nil {
		return fmt.Errorf("update ingredient: %w", err)
	}
	return tx.Commit()
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

// CreateOFF inserts an ingredient imported from OpenFoodFacts, saving it
// locally so it is reusable without OFF. name and brand are normalised with
// NormalizeOFFName / NormalizeOFFBrand first. It sets
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
	name = NormalizeOFFName(name)
	brand = NormalizeOFFBrand(brand)
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ingredients (
			source, off_barcode, name, brand, unit,
			kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
			created_by, unit_weight_g, default_quantity, last_verified_at,
			fiber_per_100, sugars_per_100, saturated_fat_per_100, sodium_per_100,
			off_aligned
		) VALUES ('openfoodfacts', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?, ?, 1)
	`,
		strings.TrimSpace(barcode), name, brand, string(unit),
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

// maxSearchTokens bounds the words of a query that count, so a pasted text
// cannot make a query with dozens of conditions.
const maxSearchTokens = 8

// searchTokens splits a query into lowercase words (letters and digits, as the
// OFF search does). A query with none, such as «%» or «...», is kept whole so it
// still matches as text.
func searchTokens(q string) []string {
	tokens := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(tokens) == 0 {
		if t := strings.ToLower(strings.TrimSpace(q)); t != "" {
			tokens = []string{t}
		}
	}
	if len(tokens) > maxSearchTokens {
		tokens = tokens[:maxSearchTokens]
	}
	return tokens
}

// SearchActive finds the non-superseded ingredients in viewerID's catalog that
// match every word of q (#224): each word is a substring, case-insensitive, of
// the name, the brand or the name of the category the viewer put the ingredient
// in. So «yogurt fage» finds Fage yogurt, and «fage yogurt» too. An ingredient
// whose name is exactly q comes first, so a caller that looks for one by name
// finds it among the first rows. Then what the viewer used before (#225): the
// ingredients in their weekly plans and recipes, the most used first and, among
// those as much used, the most recent; then the other ingredients they made by
// hand; then the ones imported from OpenFoodFacts; each group by name.
func (r *Ingredients) SearchActive(ctx context.Context, viewerID int64, q string, limit int) ([]*domain.Ingredient, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	// Uses are counted in the viewer's plans (one per row of a meal) and in
	// their recipes; the date of the last use is the week of the plan or the
	// day the recipe was last saved.
	args := []any{viewerID, viewerID}
	scope, scopeArgs := inCatalog(r.db, "created_by", viewerID)
	shown, hideArgs := notHidden(r.db, viewerID)
	args = append(args, scopeArgs...)
	args = append(args, hideArgs...)
	var words strings.Builder
	for _, t := range searchTokens(q) {
		pattern := "%" + likeEscaper.Replace(t) + "%"
		words.WriteString(` AND (lower(name) LIKE ? ESCAPE '\' OR lower(coalesce(brand, '')) LIKE ? ESCAPE '\'
		    OR EXISTS (SELECT 1 FROM ingredient_user_categories uc JOIN ingredient_categories c ON c.id = uc.category_id
		               WHERE uc.ingredient_id = ingredients.id AND uc.user_id = ? AND lower(c.name) LIKE ? ESCAPE '\'))`)
		args = append(args, pattern, pattern, viewerID, pattern)
	}
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		LEFT JOIN (
			SELECT ingredient_id AS used_id, COUNT(*) AS uses, MAX(used_at) AS last_used FROM (
				SELECT mi.ingredient_id, p.week_start AS used_at
				FROM meal_items mi
				JOIN meals m ON m.id = mi.meal_id
				JOIN weekly_plans p ON p.id = m.weekly_plan_id
				WHERE p.user_id = ? AND mi.ingredient_id IS NOT NULL
				UNION ALL
				SELECT ri.ingredient_id, date(rc.updated_at)
				FROM recipe_ingredients ri
				JOIN recipes rc ON rc.id = ri.recipe_id
				WHERE rc.user_id = ?
			) GROUP BY ingredient_id
		) u ON u.used_id = ingredients.id
		WHERE superseded_by IS NULL
		  AND (source = 'openfoodfacts' OR `+scope+`) AND `+shown+words.String()+`
		ORDER BY (lower(name) = ?) DESC, (u.uses IS NOT NULL) DESC, u.uses DESC, u.last_used DESC,
		         (source = 'openfoodfacts') ASC, name COLLATE NOCASE
		LIMIT ?`,
		append(args, strings.ToLower(strings.TrimSpace(q)), limit)...,
	)
	if err != nil {
		return nil, fmt.Errorf("search ingredients: %w", err)
	}
	defer rows.Close()
	return collectIngredients(rows)
}

// CatalogBarcodes returns which of the barcodes belong to an ingredient in
// viewerID's catalog (an OpenFoodFacts import, or one made by hand and linked to
// the code): the OpenFoodFacts search leaves those products out, so a product
// and the ingredient made from it appear once (#225).
func (r *Ingredients) CatalogBarcodes(ctx context.Context, viewerID int64, codes []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(codes) == 0 {
		return out, nil
	}
	scope, scopeArgs := inCatalog(r.db, "created_by", viewerID)
	args := make([]any, 0, len(codes)+len(scopeArgs))
	for _, c := range codes {
		args = append(args, c)
	}
	args = append(args, scopeArgs...)
	rows, err := r.db.QueryContext(ctx, `
		SELECT off_barcode FROM ingredients
		WHERE off_barcode IN (`+strings.TrimSuffix(strings.Repeat("?,", len(codes)), ",")+`)
		  AND superseded_by IS NULL AND (source = 'openfoodfacts' OR `+scope+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("catalog barcodes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan barcode: %w", err)
		}
		out[code] = true
	}
	return out, rows.Err()
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
	shown, hideArgs := notHidden(r.db, viewerID)
	args = append(args, hideArgs...)
	rows, err := r.db.QueryContext(ctx, selectIngredientCols+`
		FROM ingredients
		WHERE superseded_by IS NULL
		  AND (source = 'openfoodfacts' OR `+scope+`) AND `+shown+`
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

// ListAllActive returns every non-superseded ingredient in viewerID's catalog
// ordered by name, reading ListActive pages until the catalog is exhausted.
// The ingredients page uses it so that no row is cut off by a page size.
func (r *Ingredients) ListAllActive(ctx context.Context, viewerID int64) ([]*domain.Ingredient, error) {
	const page = 200
	var all []*domain.Ingredient
	for offset := 0; ; offset += page {
		got, err := r.ListActive(ctx, viewerID, offset, page)
		if err != nil {
			return nil, err
		}
		all = append(all, got...)
		if len(got) < page {
			return all, nil
		}
	}
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
	fiber_per_100, sugars_per_100, saturated_fat_per_100, sodium_per_100,
	off_aligned`

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
	var aligned int
	err := scan(
		&ing.ID, &srcStr, &ing.OFFBarcode, &ing.Name, &ing.Brand, &unitStr,
		&ing.KcalPer100, &ing.ProteinPer100, &ing.CarbsPer100, &ing.FatPer100,
		&ing.CreatedBy, &ing.CreatedAt, &ing.LastVerifiedAt, &ing.SupersededBy,
		&ing.UnitWeightG, &ing.DefaultQuantity,
		&ing.FiberPer100, &ing.SugarsPer100, &ing.SaturatedFatPer100, &ing.SodiumPer100,
		&aligned,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIngredientNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan ingredient: %w", err)
	}
	ing.Source = domain.IngredientSource(srcStr)
	ing.Unit = domain.IngredientUnit(unitStr)
	ing.OFFAligned = aligned == 1
	return &ing, nil
}
