package domain

import "time"

// IngredientSource enumerates where an ingredient comes from.
// `local` means entered by hand by a user; `openfoodfacts` means
// imported from the OFF API (we treat its macros as read-only).
type IngredientSource string

const (
	IngredientSourceOFF   IngredientSource = "openfoodfacts"
	IngredientSourceLocal IngredientSource = "local"
)

// IngredientUnit limits the unit of measure to the two values the schema
// allows (CHECK(unit IN ('g','ml'))).
type IngredientUnit string

const (
	UnitGram       IngredientUnit = "g"
	UnitMilliliter IngredientUnit = "ml"
)

// Ingredient is a row of the `ingredients` table decoded into Go types.
// Optional fields are *T to tell NULL from the zero value.
type Ingredient struct {
	ID             int64
	Source         IngredientSource
	OFFBarcode     *string
	Name           string
	Brand          *string
	Unit           IngredientUnit
	KcalPer100     float64
	ProteinPer100  float64
	CarbsPer100    float64
	FatPer100      float64
	CreatedBy      *int64
	CreatedAt      time.Time
	LastVerifiedAt *time.Time
	SupersededBy   *int64
	// UnitWeightG is the weight in grams of a single unit (e.g. 150 for a
	// 150g banana). NULL = an ingredient without "pieces" support.
	UnitWeightG *float64
	// DefaultQuantity is the default quantity to prefill in the picker.
	DefaultQuantity *float64
	// Micronutrients (per 100 g/ml) — all nullable: they can be missing for
	// local ingredients entered without OFF data.
	FiberPer100        *float64
	SugarsPer100       *float64
	SaturatedFatPer100 *float64
	SodiumPer100       *float64
}

// PieceWeight is UnitWeightG, 0 when unknown: above 0 the ingredient can be
// counted in pieces.
func (i Ingredient) PieceWeight() float64 {
	if i.UnitWeightG == nil || *i.UnitWeightG <= 0 {
		return 0
	}
	return *i.UnitWeightG
}
