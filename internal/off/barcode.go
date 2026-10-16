package off

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// NormalizeBarcode recognizes a food barcode (EAN-13, EAN-8 or UPC-A) typed
// or read by the camera: it removes the spaces, accepts only digits and
// checks the check digit. UPC-A goes back to 13 digits with a leading zero,
// as OpenFoodFacts stores it. ok=false for everything else, so a search by
// name is not mistaken for a code.
func NormalizeBarcode(s string) (string, bool) {
	code := strings.Join(strings.Fields(s), "")
	switch len(code) {
	case 8, 13:
	case 12:
		code = "0" + code
	default:
		return "", false
	}
	sum := 0
	for i, r := range code {
		if r < '0' || r > '9' {
			return "", false
		}
		d := int(r - '0')
		if i == len(code)-1 {
			if (10-sum%10)%10 != d {
				return "", false
			}
			break
		}
		// Weights 3 and 1 alternating, counting from the right starting at the
		// digit before the check digit.
		if (len(code)-1-i)%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return code, true
}

// FindByBarcode looks a product up by code: first in the local copy of the
// dump (off_products, no network), then, when live is true and the copy is
// missing or lacks complete macros, on the OFF API. With live=false an
// incomplete local product is returned anyway; ErrNotFound when it is nowhere.
func (c *Client) FindByBarcode(ctx context.Context, code string, live bool) (*Product, error) {
	local, err := c.localByBarcode(ctx, code)
	if err != nil {
		return nil, err
	}
	if local != nil && (local.HasFullNutrients || !live) {
		return local, nil
	}
	if !live {
		return nil, ErrNotFound
	}
	p, err := c.GetByBarcode(ctx, code)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return p, err
}

// ErrUnavailable: the OFF API did not answer (network, timeout, 5xx). Distinct
// from an error of the local copy, which is our own failure.
var ErrUnavailable = errors.New("OpenFoodFacts non raggiungibile")

// localByBarcode reads off_products; nil when the code is not there or the DB
// is not configured.
func (c *Client) localByBarcode(ctx context.Context, code string) (*Product, error) {
	if c.DB == nil {
		return nil, nil
	}
	var p Product
	var hasFull int
	err := c.DB.QueryRowContext(ctx, `
		SELECT barcode, product_name, brands, quantity,
		       kcal_per_100, protein_per_100, carbs_per_100, fat_per_100,
		       has_full_nutrients, image_url, serving_g
		FROM off_products WHERE barcode = ?`, code).Scan(
		&p.Code, &p.Name, &p.Brand, &p.Quantity,
		&p.Kcal, &p.Protein, &p.Carbs, &p.Fat, &hasFull, &p.ImageURL, &p.ServingG)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("off_products per codice: %w", err)
	}
	p.HasFullNutrients = hasFull == 1
	p.UnitWeightG = ParseUnitWeightG(p.Quantity)
	return &p, nil
}
