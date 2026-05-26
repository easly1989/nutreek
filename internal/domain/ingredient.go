package domain

import "time"

// IngredientSource enumera la provenienza di un ingrediente.
// `local` significa inserito a mano da un utente; `openfoodfacts`
// importato dall'API OFF (macro consideriamo non modificabili).
type IngredientSource string

const (
	IngredientSourceOFF   IngredientSource = "openfoodfacts"
	IngredientSourceLocal IngredientSource = "local"
)

// IngredientUnit limita l'unita' di misura ai due valori previsti dallo
// schema (CHECK(unit IN ('g','ml'))).
type IngredientUnit string

const (
	UnitGram       IngredientUnit = "g"
	UnitMilliliter IngredientUnit = "ml"
)

// Ingredient rappresenta una riga della tabella `ingredients` deserializzata
// in tipi Go. Gli optional sono *T per distinguere NULL da zero-value.
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
	// UnitWeightG e' il peso in grammi di una singola unita' (es. 150 per una
	// banana da 150g). NULL = ingrediente senza supporto "pezzi".
	UnitWeightG *float64
}
