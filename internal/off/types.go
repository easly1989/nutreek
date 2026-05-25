package off

import (
	"encoding/json"
	"strconv"
)

// Product e' la rappresentazione "pulita" che esponiamo all'app: solo i
// campi che usiamo davvero, con tipi prevedibili.
type Product struct {
	Code     string  // barcode
	Name     string  // product_name
	Brand    string  // primo elemento di brands (CSV su OFF)
	Quantity string  // testuale ("100 g", "1 L"), opzionale
	Kcal     float64 // per 100g/ml
	Protein  float64
	Carbs    float64
	Fat      float64
	// HasFullNutrients e' true sse tutti e 4 i macro sono presenti su OFF.
	// Lo usiamo per decidere se permettere l'import o suggerire un local.
	HasFullNutrients bool
}

// rawProduct riflette la forma JSON degli endpoint OFF "product"
// (api/v2/product/{code}.json). I nutriments arrivano in una mappa
// free-form con chiavi tipo "energy-kcal_100g" che possono essere
// number o stringa a seconda del prodotto. Lo parsiamo a mano in toProduct.
type rawProduct struct {
	Code        string                     `json:"code"`
	ProductName string                     `json:"product_name"`
	Brands      string                     `json:"brands"`
	Quantity    string                     `json:"quantity"`
	Nutriments  map[string]json.RawMessage `json:"nutriments"`
}

// cgiSearchResponse e' la risposta di it.openfoodfacts.org/cgi/search.pl.
// Usa lo stesso schema di rawProduct: brands come stringa CSV, nutriments identici.
type cgiSearchResponse struct {
	Products []rawProduct `json:"products"`
}

type productResponse struct {
	Status  int        `json:"status"` // 1 ok, 0 not found
	Code    string     `json:"code"`
	Product rawProduct `json:"product"`
}

func (r rawProduct) toProduct() Product {
	p := Product{
		Code:     r.Code,
		Name:     r.ProductName,
		Brand:    firstBrand(r.Brands),
		Quantity: r.Quantity,
	}
	kcal, kOK := lookupNutrient(r.Nutriments, "energy-kcal_100g")
	if !kOK {
		// fallback: alcuni prodotti hanno solo "energy_100g" (kJ).
		if kj, ok := lookupNutrient(r.Nutriments, "energy_100g"); ok {
			kcal = kj / 4.184
			kOK = true
		}
	}
	prot, pOK := lookupNutrient(r.Nutriments, "proteins_100g")
	carbs, cOK := lookupNutrient(r.Nutriments, "carbohydrates_100g")
	fat, fOK := lookupNutrient(r.Nutriments, "fat_100g")
	p.Kcal, p.Protein, p.Carbs, p.Fat = kcal, prot, carbs, fat
	p.HasFullNutrients = kOK && pOK && cOK && fOK
	return p
}

// lookupNutrient legge un campo che OFF puo' restituire come numero o stringa.
func lookupNutrient(m map[string]json.RawMessage, key string) (float64, bool) {
	raw, ok := m[key]
	if !ok || len(raw) == 0 {
		return 0, false
	}
	// Tenta number (la maggior parte dei casi).
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	// Fallback: stringa (qualche produttore inserisce "12.3" tra apici).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		v, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return v, true
		}
	}
	return 0, false
}

func firstBrand(csv string) string {
	for i := 0; i < len(csv); i++ {
		if csv[i] == ',' {
			return trimSpace(csv[:i])
		}
	}
	return trimSpace(csv)
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
