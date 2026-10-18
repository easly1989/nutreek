package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

// catalogScopes holds, per database, the query listing the users whose
// hand-made ingredients and public recipes a viewer sees, with one ? for the
// viewer's id. A database without one shares its catalog with everyone, as
// normal installs do; the demo build scopes it to the visitor's group (#137).
// Keyed by database so one call reaches every repository on it.
var catalogScopes sync.Map // *sql.DB → string

// ScopeCatalog limits the shared catalog on d to the users peers lists.
func ScopeCatalog(d *sql.DB, peers string) { catalogScopes.Store(d, peers) }

// hiddenProducts holds, per database, the query listing the ingredients a
// viewer does not see in the ingredient lists, with one ? for the viewer's id:
// the demo hides its OpenFoodFacts demo products in another language (#189).
var hiddenProducts sync.Map // *sql.DB → string

// HideProducts keeps the ingredients hidden lists out of the lists on d.
func HideProducts(d *sql.DB, hidden string) { hiddenProducts.Store(d, hidden) }

// notHidden returns the SQL condition "the ingredient is not hidden from the
// viewer" and its arguments: always true without HideProducts.
func notHidden(d *sql.DB, viewerID int64) (string, []any) {
	hidden, ok := hiddenProducts.Load(d)
	if !ok {
		return "1", nil
	}
	return "id NOT IN (" + hidden.(string) + ")", []any{viewerID}
}

// HiddenBarcodes returns the barcodes of the products hidden from viewerID's
// lists (see HideProducts), so the OpenFoodFacts search leaves them out too;
// nil without HideProducts.
func (r *Ingredients) HiddenBarcodes(ctx context.Context, viewerID int64) (map[string]bool, error) {
	hidden, ok := hiddenProducts.Load(r.db)
	if !ok {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT off_barcode FROM ingredients WHERE off_barcode IS NOT NULL AND id IN (`+hidden.(string)+`)`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("hidden barcodes: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out[code] = true
	}
	return out, rows.Err()
}

// inCatalog returns the SQL condition "the user in col shares the catalog
// with the viewer" and its arguments: always true on an unscoped database.
func inCatalog(d *sql.DB, col string, viewerID int64) (string, []any) {
	peers, ok := catalogScopes.Load(d)
	if !ok {
		return "1", nil
	}
	return col + " IN (" + peers.(string) + ")", []any{viewerID}
}

// ingredientVisibleTo checks that the viewer may use the ingredient: an
// OpenFoodFacts product, or one made inside their catalog. Otherwise
// ErrIngredientNotFound. On an unscoped database every ingredient is shared,
// so it checks nothing.
func ingredientVisibleTo(ctx context.Context, d *sql.DB, id, viewerID int64) error {
	if _, ok := catalogScopes.Load(d); !ok {
		return nil
	}
	cond, args := inCatalog(d, "created_by", viewerID)
	var one int
	err := d.QueryRowContext(ctx,
		`SELECT 1 FROM ingredients WHERE id = ? AND (source = 'openfoodfacts' OR `+cond+`)`,
		append([]any{id}, args...)...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrIngredientNotFound
	}
	if err != nil {
		return fmt.Errorf("ingredient visibility: %w", err)
	}
	return nil
}
