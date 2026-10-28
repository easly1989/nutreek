package off

import (
	"encoding/json"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Product is the "clean" representation we expose to the app: only the
// fields we really use, with predictable types.
type Product struct {
	Code     string  // barcode
	Name     string  // product_name
	Brand    string  // first element of brands (CSV on OFF)
	Quantity string  // text ("100 g", "1 L"), optional
	Kcal     float64 // per 100g/ml
	Protein  float64
	Carbs    float64
	Fat      float64
	// HasFullNutrients is true iff all 4 macros are present on OFF.
	// We use it to decide whether to allow the import or suggest a local one.
	HasFullNutrients bool
	// ImageURL is the URL of the product's front thumbnail (OFF's
	// image_front_small_url, or built from images.selected.front in the
	// current dump). It can be empty for products without a photo.
	ImageURL string
	// UnitWeightG is the weight in grams per single unit, extracted from
	// product_quantity (e.g. "150 g" → 150). nil when it cannot be parsed.
	UnitWeightG *float64
	// ServingG is the serving in grams or ml (OFF's serving_quantity, or a
	// serving_size like "30 g"). nil when OFF does not know it.
	ServingG *float64
	// Micronutrients — nil when missing from the OFF dataset.
	Fiber        *float64
	Sugars       *float64
	SaturatedFat *float64
	Sodium       *float64
}

// rawProduct mirrors the JSON shape of the OFF "product" endpoints
// (api/v2/product/{code}.json) and of a line of the public JSONL dump
// (reused by dumpProduct.toProduct). The nutriments arrive in a free-form
// map with keys like "energy-kcal_100g" that can be a number or a string
// depending on the product. We parse it by hand in toProduct.
type rawProduct struct {
	Code               string                     `json:"code"`
	ProductName        string                     `json:"product_name"`
	Brands             string                     `json:"brands"`
	Quantity           string                     `json:"quantity"`
	Nutriments         map[string]json.RawMessage `json:"nutriments"`
	ImageFrontSmallURL string                     `json:"image_front_small_url"`
	ImageURL           string                     `json:"image_url"`
	// serving_quantity arrives as a number or as a string, depending on the product.
	ServingQuantity     json.RawMessage `json:"serving_quantity"`
	ServingQuantityUnit string          `json:"serving_quantity_unit"`
	ServingSize         string          `json:"serving_size"`
	// The dump's current shape (#204): nutrients in nutrition.aggregated_set,
	// no image URLs but images.selected.front. Kept raw and parsed leniently,
	// so an unexpected shape there does not discard the whole product.
	Nutrition json.RawMessage `json:"nutrition"`
	Images    json.RawMessage `json:"images"`
}

// aggregatedSet is nutrition.aggregated_set of the dump's current shape:
// every nutrient with its value and unit, per the quantity in Per.
type aggregatedSet struct {
	Per       string `json:"per"`
	Nutrients map[string]struct {
		Value json.RawMessage `json:"value"`
		Unit  string          `json:"unit"`
	} `json:"nutrients"`
}

// aggregatedPer100 returns the aggregated set when it is per 100 g or per
// 100 ml, nil otherwise (missing, malformed or per serving). Drinks come per
// 100 ml: like the old nutriments' *_100g values, the app reads 1 ml as 1 g.
func aggregatedPer100(nutrition json.RawMessage) *aggregatedSet {
	var n struct {
		AggregatedSet *aggregatedSet `json:"aggregated_set"`
	}
	if len(nutrition) == 0 || json.Unmarshal(nutrition, &n) != nil {
		return nil
	}
	if s := n.AggregatedSet; s != nil && (s.Per == "100g" || s.Per == "100ml") && len(s.Nutrients) > 0 {
		return s
	}
	return nil
}

// grams reads a nutrient of the set in grams.
func (s *aggregatedSet) grams(name string) (float64, bool) {
	n, ok := s.Nutrients[name]
	if !ok {
		return 0, false
	}
	v, ok := flexFloat(n.Value)
	if !ok {
		return 0, false
	}
	switch strings.ToLower(n.Unit) {
	case "g":
		return v, true
	case "mg":
		return v / 1000, true
	case "µg", "mcg":
		return v / 1e6, true
	}
	return 0, false
}

// kcal reads the energy in kcal: energy-kcal when there, else kJ converted.
func (s *aggregatedSet) kcal() (float64, bool) {
	for _, name := range []string{"energy-kcal", "energy-kj", "energy"} {
		n, ok := s.Nutrients[name]
		if !ok {
			continue
		}
		v, ok := flexFloat(n.Value)
		if !ok {
			continue
		}
		switch strings.ToLower(n.Unit) {
		case "kcal":
			return v, true
		case "kj":
			return v / 4.184, true
		}
	}
	return 0, false
}

// frontImageURL builds the 200 px front thumbnail from images.selected.front
// of the dump's current shape: Italian first, then English, then the first
// other language in alphabetical order. "" when there is no front image.
func frontImageURL(code string, images json.RawMessage) string {
	var im struct {
		Selected struct {
			Front map[string]struct {
				Rev   json.Number                `json:"rev"` // "4" or 4
				Sizes map[string]json.RawMessage `json:"sizes"`
			} `json:"front"`
		} `json:"selected"`
	}
	if len(images) == 0 || json.Unmarshal(images, &im) != nil {
		return ""
	}
	front := im.Selected.Front
	for _, lang := range append([]string{"it", "en"}, slices.Sorted(maps.Keys(front))...) {
		f, ok := front[lang]
		if !ok || !reImageLang.MatchString(lang) {
			continue
		}
		if _, ok := f.Sizes["200"]; !ok {
			continue
		}
		if rev, err := strconv.Atoi(f.Rev.String()); err == nil && rev > 0 {
			return FrontImageURL(code, lang, strconv.Itoa(rev))
		}
	}
	return ""
}

// reImageLang keeps the language of an image to a plain code ("it", "en"):
// it ends up in the URL path.
var reImageLang = regexp.MustCompile(`^[a-z]{2,3}$`)

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
	if imgURL == "" {
		imgURL = frontImageURL(r.Code, r.Images)
	}
	p := Product{
		Code:        r.Code,
		Name:        r.ProductName,
		Brand:       firstBrand(r.Brands),
		Quantity:    r.Quantity,
		ImageURL:    imgURL,
		UnitWeightG: ParseUnitWeightG(r.Quantity),
		ServingG:    ParseServingG(r.ServingQuantity, r.ServingQuantityUnit, r.ServingSize),
	}
	// The JSONL dump uses bare keys ("proteins") while the API uses "_100g"
	// ("proteins_100g"). Try both to support the two sources.
	grams := func(name string) (float64, bool) {
		return lookupNutrientAny(r.Nutriments, name+"_100g", name)
	}
	kcal, kOK := lookupNutrientAny(r.Nutriments, "energy-kcal_100g", "energy-kcal")
	if !kOK {
		if kj, ok := lookupNutrientAny(r.Nutriments, "energy_100g", "energy"); ok {
			kcal = kj / 4.184
			kOK = true
		}
	}
	// The dump's current shape wins when it carries values per 100 g/ml;
	// a nutrient missing from it still comes from nutriments.
	if set := aggregatedPer100(r.Nutrition); set != nil {
		fromNutriments := grams
		grams = func(name string) (float64, bool) {
			if v, ok := set.grams(name); ok {
				return v, true
			}
			return fromNutriments(name)
		}
		if v, ok := set.kcal(); ok {
			kcal, kOK = v, true
		}
	}
	prot, pOK := grams("proteins")
	carbs, cOK := grams("carbohydrates")
	fat, fOK := grams("fat")
	p.Kcal, p.Protein, p.Carbs, p.Fat = kcal, prot, carbs, fat
	// Implausible values (kJ entered as kcal, grams per serving) are not importable.
	plausible := kcal <= maxKcalPer100 && prot+carbs+fat <= maxMacrosPer100
	p.HasFullNutrients = kOK && pOK && cOK && fOK && plausible

	if v, ok := grams("fiber"); ok {
		p.Fiber = &v
	}
	if v, ok := grams("sugars"); ok {
		p.Sugars = &v
	}
	if v, ok := grams("saturated-fat"); ok {
		p.SaturatedFat = &v
	}
	if v, ok := grams("sodium"); ok {
		p.Sodium = &v
	}
	return p
}

// Upper bounds of plausible values per 100 g: pure fat is ~900 kcal, and the
// macros cannot weigh much more than the 100 g they are in (5 g of rounding).
const (
	maxKcalPer100   = 900
	maxMacrosPer100 = 105
)

// lookupNutrientAny tries the keys in order and returns the first match.
// Used to handle both the API format (_100g) and the JSONL dump (bare).
func lookupNutrientAny(m map[string]json.RawMessage, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := lookupNutrient(m, k); ok {
			return v, ok
		}
	}
	return 0, false
}

// lookupNutrient reads a field OFF can return as a number or a string.
func lookupNutrient(m map[string]json.RawMessage, key string) (float64, bool) {
	return flexFloat(m[key])
}

// flexFloat reads a value OFF can write as a number or a string; null is
// missing, not 0.
func flexFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	// Try a number (most cases).
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	// Fallback: a string (some producers enter "12.3" in quotes).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		v, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return v, true
		}
	}
	return 0, false
}

// reSingleGrams matches strings like "150 g" or "200g" (a single quantity in grams).
var reSingleGrams = regexp.MustCompile(`(?i)^\s*(\d+(?:[.,]\d+)?)\s*g\s*$`)

// reMultiGrams matches "6 × 25 g" or "6x 30g" (N pieces of M grams each).
var reMultiGrams = regexp.MustCompile(`(?i)^\s*\d+\s*[x×]\s*(\d+(?:[.,]\d+)?)\s*g\s*$`)

// ParseUnitWeightG tries to extract the weight per piece in grams from OFF's
// product_quantity (e.g. "150 g" → 150.0, "6 × 25 g" → 25.0). Returns nil when it cannot be parsed.
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

// maxServingG discards absurd servings (data entry errors on OFF): a serving
// proposed above one kilo would do more harm than an empty field.
const maxServingG = 1000

// reServingSize matches the g/ml part of serving_size: "30 g", "1 biscotto
// (12,5 g)", "250ml".
var reServingSize = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(g|ml)\b`)

// ParseServingG derives the serving in grams (or ml) from the OFF fields:
// serving_quantity (number or string) when its unit is g/ml or missing,
// otherwise the g/ml quantity written in serving_size. nil when neither
// gives a value in (0, 1000].
func ParseServingG(quantity json.RawMessage, unit, size string) *float64 {
	valid := func(v float64) *float64 {
		if v > 0 && v <= maxServingG {
			return &v
		}
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "g", "ml":
		if v, ok := lookupNutrient(map[string]json.RawMessage{"q": quantity}, "q"); ok {
			if p := valid(v); p != nil {
				return p
			}
		}
	}
	if m := reServingSize.FindStringSubmatch(size); m != nil {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
			return valid(v)
		}
	}
	return nil
}

// FrontImageURL builds the URL of the 200 px front image of a product in the
// given language and revision, following OFF's rule: a numeric barcode
// shorter than 13 digits is left-padded with zeros, then split as
// NNN/NNN/NNN/rest. "" for a barcode that is not numeric.
//
//	https://images.openfoodfacts.org/images/products/343/566/076/8163/front_it.4.200.jpg
func FrontImageURL(barcode, lang, rev string) string {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" || strings.Trim(barcode, "0123456789") != "" {
		return ""
	}
	if len(barcode) < 13 {
		barcode = strings.Repeat("0", 13-len(barcode)) + barcode
	}
	return "https://" + ThumbHost + "/images/products/" +
		barcode[:3] + "/" + barcode[3:6] + "/" + barcode[6:9] + "/" + barcode[9:] +
		"/front_" + lang + "." + rev + ".200.jpg"
}

// ThumbHost is the only host the page loads OFF thumbnails from: the same
// name is in the CSP (img-src, internal/app/router.go).
const ThumbHost = "images.openfoodfacts.org"

// ThumbURL makes an OFF thumbnail URL displayable: old dumps still use
// static.openfoodfacts.org/images/, which becomes ThumbHost (same paths).
// Any other URL (another host, not https) returns "": the template then
// shows the barcode icon.
func ThumbURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	switch {
	case u.Host == ThumbHost && strings.HasPrefix(u.Path, "/images/"):
	case u.Host == "static.openfoodfacts.org" && strings.HasPrefix(u.Path, "/images/"):
		u.Host = ThumbHost
	default:
		return ""
	}
	return u.String()
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
