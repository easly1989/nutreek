package off

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// NormalizeBarcode riconosce un codice a barre alimentare (EAN-13, EAN-8 o
// UPC-A) scritto o letto dalla fotocamera: toglie gli spazi, accetta solo
// cifre e verifica la cifra di controllo. L'UPC-A torna a 13 cifre con lo
// zero davanti, come lo salva OpenFoodFacts. ok=false per tutto il resto,
// cosi' una ricerca per nome non viene scambiata per un codice.
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
		// Pesi 3 e 1 alternati, contando da destra a partire dalla cifra
		// che precede quella di controllo.
		if (len(code)-1-i)%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return code, true
}

// FindByBarcode cerca un prodotto per codice: prima nella copia locale del
// dump (off_products, nessuna rete), poi, se live e' true e la copia manca o
// non ha i macro completi, sull'API di OFF. Con live=false un prodotto locale
// incompleto viene restituito comunque; ErrNotFound se non c'e' da nessuna parte.
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

// ErrUnavailable: l'API di OFF non ha risposto (rete, timeout, 5xx). Distinto
// da un errore della copia locale, che e' un guasto nostro.
var ErrUnavailable = errors.New("OpenFoodFacts non raggiungibile")

// localByBarcode legge off_products; nil se il codice non c'e' o il DB non
// e' configurato.
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
