package off

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
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
	// ImageURL e' la URL del thumbnail fronte prodotto (image_front_small_url
	// di OFF). Puo' essere vuota per prodotti senza foto.
	ImageURL string
	// UnitWeightG e' il peso in grammi per singola unita', estratto da
	// product_quantity (es. "150 g" → 150). nil se non parsabile.
	UnitWeightG *float64
}

// rawProduct riflette la forma JSON degli endpoint OFF "product"
// (api/v2/product/{code}.json) e di una riga del dump JSONL pubblico
// (riusato da dumpProduct.toProduct). I nutriments arrivano in una mappa
// free-form con chiavi tipo "energy-kcal_100g" che possono essere
// number o stringa a seconda del prodotto. Lo parsiamo a mano in toProduct.
type rawProduct struct {
	Code               string                     `json:"code"`
	ProductName        string                     `json:"product_name"`
	Brands             string                     `json:"brands"`
	Quantity           string                     `json:"quantity"`
	Nutriments         map[string]json.RawMessage `json:"nutriments"`
	ImageFrontSmallURL string                     `json:"image_front_small_url"`
	ImageURL           string                     `json:"image_url"`
}

type productResponse struct {
	Status  int        `json:"status"` // 1 ok, 0 not found
	Code    string     `json:"code"`
	Product rawProduct `json:"product"`
}

func (r rawProduct) toProduct() Product {
	imgURL := r.ImageFrontSmallURL
	if imgURL == "" {
		imgURL = r.ImageURL
	}
	p := Product{
		Code:        r.Code,
		Name:        r.ProductName,
		Brand:       firstBrand(r.Brands),
		Quantity:    r.Quantity,
		ImageURL:    imgURL,
		UnitWeightG: ParseUnitWeightG(r.Quantity),
	}
	// Il dump JSONL usa chiavi bare ("proteins") mentre l'API usa "_100g"
	// ("proteins_100g"). Proviamo entrambe per supportare le due sorgenti.
	kcal, kOK := lookupNutrientAny(r.Nutriments, "energy-kcal_100g", "energy-kcal")
	if !kOK {
		if kj, ok := lookupNutrientAny(r.Nutriments, "energy_100g", "energy"); ok {
			kcal = kj / 4.184
			kOK = true
		}
	}
	prot, pOK := lookupNutrientAny(r.Nutriments, "proteins_100g", "proteins")
	carbs, cOK := lookupNutrientAny(r.Nutriments, "carbohydrates_100g", "carbohydrates")
	fat, fOK := lookupNutrientAny(r.Nutriments, "fat_100g", "fat")
	p.Kcal, p.Protein, p.Carbs, p.Fat = kcal, prot, carbs, fat
	p.HasFullNutrients = kOK && pOK && cOK && fOK
	return p
}

// lookupNutrientAny prova le chiavi in ordine e ritorna il primo match.
// Usato per gestire sia il formato API (_100g) sia il dump JSONL (bare).
func lookupNutrientAny(m map[string]json.RawMessage, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := lookupNutrient(m, k); ok {
			return v, ok
		}
	}
	return 0, false
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

// reSingleGrams matcha stringhe tipo "150 g" o "200g" (singola quantità in grammi).
var reSingleGrams = regexp.MustCompile(`(?i)^\s*(\d+(?:[.,]\d+)?)\s*g\s*$`)

// reMultiGrams matcha "6 × 25 g" o "6x 30g" (N pezzi da M grammi ciascuno).
var reMultiGrams = regexp.MustCompile(`(?i)^\s*\d+\s*[x×]\s*(\d+(?:[.,]\d+)?)\s*g\s*$`)

// ParseUnitWeightG prova a estrarre il peso per pezzo in grammi da product_quantity
// di OFF (es. "150 g" → 150.0, "6 × 25 g" → 25.0). Ritorna nil se non parsabile.
func ParseUnitWeightG(productQuantity string) *float64 {
	q := strings.TrimSpace(productQuantity)
	if q == "" {
		return nil
	}
	for _, re := range []*regexp.Regexp{reMultiGrams, reSingleGrams} {
		if m := re.FindStringSubmatch(q); m != nil {
			s := strings.ReplaceAll(m[1], ",", ".")
			v, err := strconv.ParseFloat(s, 64)
			if err == nil && v > 0 {
				return &v
			}
		}
	}
	return nil
}

// OFFCDNImageURL costruisce la URL best-effort del thumbnail fronte di un
// prodotto OFF dal barcode. Usata come fallback quando image_url e' vuota nel
// dump locale (prodotti importati prima della migrazione 0012). Non garantisce
// che l'immagine esista: gestire con onerror nel template HTML.
//
// Pattern CDN OFF (EAN-13):  NNN/NNN/NNN/NNNN/front_fr.200.jpg
// Pattern CDN OFF (altri):   {barcode}/front_fr.200.jpg
func OFFCDNImageURL(barcode string) string {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return ""
	}
	base := "https://images.openfoodfacts.org/images/products/"
	if len(barcode) == 13 {
		return base + barcode[:3] + "/" + barcode[3:6] + "/" + barcode[6:9] + "/" + barcode[9:] + "/front_fr.200.jpg"
	}
	return base + barcode + "/front_fr.200.jpg"
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
