package off

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

// A line of the dump in its current shape (#204), trimmed from a real one:
// nutrients in nutrition.aggregated_set, nutriments empty, no image URL but
// images.selected.front. Energy only in kJ, sodium in mg.
const newShapeLine = `{"code":"8001234567895","product_name":"Banana","brands":"Chiquita,Other","quantity":"1 kg",
"countries_tags":["en:italy"],"nutriments":{},"image_url":null,
"nutrition":{"aggregated_set":{"per":"100g","preparation":"as_sold","nutrients":{
  "energy-kj":{"value":372,"unit":"kJ","source_per":"100g"},
  "energy":{"value":372,"unit":"kJ","source_per":"100g"},
  "proteins":{"value":1.1,"unit":"g","source_per":"100g"},
  "carbohydrates":{"value":"20","unit":"g","source_per":"100g"},
  "fat":{"value":0.3,"unit":"g","source_per":"100g"},
  "sugars":{"value":17,"unit":"g","source_per":"100g"},
  "fiber":{"value":2.6,"unit":"g","source_per":"serving"},
  "sodium":{"value":1,"unit":"mg","source_per":"100g"},
  "cocoa":{"value":10,"unit":"%","source_per":"100g"}}},
 "input_sets":[]},
"images":{"uploaded":{"1":{}},"selected":{"front":{
  "fr":{"rev":"4","imgid":"1","sizes":{"200":{"w":150,"h":200}}},
  "it":{"rev":"7","imgid":"2","sizes":{"100":{},"200":{"w":150,"h":200},"full":{}}}}}}}`

// A line in the old shape: nutriments with bare keys and the image URL.
const oldShapeLine = `{"code":"8000000000017","product_name":"Grana Padano DOP","brands":"Consorzio",
"countries_tags":["en:italy"],"nutriments":{"energy-kcal":384,"proteins":33,"carbohydrates":0,"fat":28,"sodium":0.6},
"image_front_small_url":"https://images.openfoodfacts.org/images/products/800/000/000/0017/front_it.3.200.jpg"}`

func parseLine(t *testing.T, line string) Product {
	t.Helper()
	var d dumpProduct
	if err := json.Unmarshal([]byte(line), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d.toProduct()
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestToProductDumpShapes(t *testing.T) {
	p := parseLine(t, newShapeLine)
	if !p.HasFullNutrients || !near(p.Kcal, 372/4.184) || p.Protein != 1.1 || p.Carbs != 20 || p.Fat != 0.3 {
		t.Errorf("new shape macros: %+v", p)
	}
	if p.Sodium == nil || !near(*p.Sodium, 0.001) || p.Sugars == nil || *p.Sugars != 17 || p.Fiber == nil || *p.Fiber != 2.6 {
		t.Errorf("new shape micronutrients: sodium %v sugars %v fiber %v", p.Sodium, p.Sugars, p.Fiber)
	}
	if p.Brand != "Chiquita" || p.Quantity != "1 kg" {
		t.Errorf("new shape brand/quantity: %q %q", p.Brand, p.Quantity)
	}
	if want := "https://images.openfoodfacts.org/images/products/800/123/456/7895/front_it.7.200.jpg"; p.ImageURL != want {
		t.Errorf("new shape image: %q, want %q", p.ImageURL, want)
	}

	p = parseLine(t, oldShapeLine)
	if !p.HasFullNutrients || p.Kcal != 384 || p.Protein != 33 || p.Carbs != 0 || p.Fat != 28 {
		t.Errorf("old shape macros: %+v", p)
	}
	if want := "https://images.openfoodfacts.org/images/products/800/000/000/0017/front_it.3.200.jpg"; p.ImageURL != want {
		t.Errorf("old shape image: %q", p.ImageURL)
	}
}

// Drinks come per 100 ml: read like the old nutriments, 1 ml as 1 g.
func TestToProductPer100ml(t *testing.T) {
	p := parseLine(t, `{"code":"1","product_name":"Latte","nutrition":{"aggregated_set":{"per":"100ml","nutrients":{"energy-kcal":{"value":46,"unit":"kcal"},"proteins":{"value":3.3,"unit":"g"},"carbohydrates":{"value":4.8,"unit":"g"},"fat":{"value":1.5,"unit":"g"}}}}}`)
	if !p.HasFullNutrients || p.Kcal != 46 || p.Protein != 3.3 || p.Carbs != 4.8 || p.Fat != 1.5 {
		t.Errorf("per 100 ml: %+v", p)
	}
}

// Entered per serving, the *_100g keys still count; the bare ones do not.
func TestToProductPerServing100gKeys(t *testing.T) {
	p := parseLine(t, `{"code":"1","product_name":"Crackers","nutrition_data_per":"serving","nutriments":{"energy-kcal":120,"energy-kcal_100g":430,"proteins_100g":10,"carbohydrates_100g":70,"fat_100g":11,"proteins":3}}`)
	if !p.HasFullNutrients || p.Kcal != 430 || p.Protein != 10 {
		t.Errorf("per serving with *_100g keys: %+v", p)
	}
}

// The plausibility bounds are inclusive: pure oil is still importable.
func TestToProductPlausibleEdge(t *testing.T) {
	p := parseLine(t, `{"code":"1","product_name":"Olio","nutriments":{"energy-kcal_100g":900,"proteins_100g":0,"carbohydrates_100g":5,"fat_100g":100}}`)
	if !p.HasFullNutrients {
		t.Errorf("900 kcal and 105 g of macros per 100 g must be importable: %+v", p)
	}
}

// A nutrient missing from the aggregated set, or null there, still comes from
// nutriments; the set wins where it has a value.
func TestToProductPartialSet(t *testing.T) {
	p := parseLine(t, `{"code":"1","product_name":"Yogurt",
"nutriments":{"energy-kcal_100g":61,"proteins_100g":9,"fat_100g":3.1,"sugars_100g":4},
"nutrition":{"aggregated_set":{"per":"100g","nutrients":{
  "proteins":{"value":3.5,"unit":"g"},"carbohydrates":{"value":4.7,"unit":"g"},"fat":{"value":null,"unit":"g"}}}}}`)
	if !p.HasFullNutrients || p.Kcal != 61 || p.Protein != 3.5 || p.Carbs != 4.7 || p.Fat != 3.1 {
		t.Errorf("partial set: %+v", p)
	}
	if p.Sugars == nil || *p.Sugars != 4 {
		t.Errorf("sugars from nutriments: %v", p.Sugars)
	}
}

// Only values per 100 g/ml count: a set per serving or with a macro missing
// does not make the product importable, and a malformed nutrition block does
// not discard the rest of the line.
func TestToProductIncomplete(t *testing.T) {
	cases := map[string]string{
		"per serving":  `{"code":"1","product_name":"Latte","nutrition":{"aggregated_set":{"per":"serving","nutrients":{"energy-kcal":{"value":46,"unit":"kcal"},"proteins":{"value":3.3,"unit":"g"},"carbohydrates":{"value":4.8,"unit":"g"},"fat":{"value":1.5,"unit":"g"}}}}}`,
		"fat missing":  `{"code":"1","product_name":"Pane","nutrition":{"aggregated_set":{"per":"100g","nutrients":{"energy-kcal":{"value":250,"unit":"kcal"},"proteins":{"value":8,"unit":"g"},"carbohydrates":{"value":50,"unit":"g"}}}}}`,
		"unknown unit": `{"code":"1","product_name":"Pane","nutrition":{"aggregated_set":{"per":"100g","nutrients":{"energy-kcal":{"value":250,"unit":"kcal"},"proteins":{"value":8,"unit":"g"},"carbohydrates":{"value":50,"unit":"g"},"fat":{"value":1,"unit":"%"}}}}}`,
		// 0000105000196: bare values per a 1.5 g serving, no *_100g keys.
		"bare per serving":   `{"code":"0000105000196","product_name":"Apple & Cinnamon Tea","nutrition_data_per":"serving","serving_size":"1.5 g","nutriments":{"energy-kcal":240,"energy-kcal_serving":3.6,"proteins":1,"carbohydrates":55,"fat":0.5}}`,
		"kJ entered as kcal": `{"code":"1","product_name":"Biscotti","nutriments":{"energy-kcal_100g":1320,"proteins_100g":7,"carbohydrates_100g":70,"fat_100g":15}}`,
		"macros above 100 g": `{"code":"1","product_name":"Biscotti","nutrition":{"aggregated_set":{"per":"100g","nutrients":{"energy-kcal":{"value":450,"unit":"kcal"},"proteins":{"value":20,"unit":"g"},"carbohydrates":{"value":70,"unit":"g"},"fat":{"value":16,"unit":"g"}}}}}`,
		"null in nutriments": `{"code":"1","product_name":"Pane","nutriments":{"energy-kcal":250,"proteins":null,"carbohydrates":50,"fat":1}}`,
		"null in the set":    `{"code":"1","product_name":"Pane","nutrition":{"aggregated_set":{"per":"100g","nutrients":{"energy-kcal":{"value":null,"unit":"kcal"},"proteins":{"value":8,"unit":"g"},"carbohydrates":{"value":50,"unit":"g"},"fat":{"value":1,"unit":"g"}}}}}`,
		"malformed set":      `{"code":"1","product_name":"Pane","nutrition":{"aggregated_set":"oops"},"images":[1]}`,
	}
	for name, line := range cases {
		p := parseLine(t, line)
		if p.HasFullNutrients || p.Name == "" {
			t.Errorf("%s: %+v", name, p)
		}
	}
}

func TestFrontImageLanguage(t *testing.T) {
	front := func(langs string) json.RawMessage {
		return json.RawMessage(`{"selected":{"front":{` + langs + `}}}`)
	}
	const s = `{"rev":"2","sizes":{"200":{}}}`
	cases := []struct {
		name   string
		images json.RawMessage
		want   string // the file name, "" for no image
	}{
		{"Italian first", front(`"fr":` + s + `,"en":` + s + `,"it":` + s), "front_it.2.200.jpg"},
		{"then English", front(`"fr":` + s + `,"en":` + s), "front_en.2.200.jpg"},
		{"then the first other language", front(`"fr":` + s + `,"de":` + s), "front_de.2.200.jpg"},
		{"numeric rev", front(`"it":{"rev":5,"sizes":{"200":{}}}`), "front_it.5.200.jpg"},
		{"no 200 px size", front(`"it":{"rev":"2","sizes":{"full":{}}},"en":` + s), "front_en.2.200.jpg"},
		{"a language that is not a code", front(`"../x":` + s), ""},
		{"no front", json.RawMessage(`{"uploaded":{"1":{}}}`), ""},
		{"no images", nil, ""},
	}
	for _, tc := range cases {
		got := frontImageURL("8001234567895", tc.images)
		want := ""
		if tc.want != "" {
			want = "https://images.openfoodfacts.org/images/products/800/123/456/7895/" + tc.want
		}
		if got != want {
			t.Errorf("%s: %q, want %q", tc.name, got, want)
		}
	}
}

// Search offers only importable products, and exact duplicates once (#204).
func TestSearchByNameImportableAndDistinct(t *testing.T) {
	sqldb := openTestDB(t)
	seedOFFProducts(t, sqldb, []Product{
		{Code: "1000000000001", Name: "Banana", Brand: "Chiquita", Kcal: 89, Protein: 1.1, Carbs: 20, Fat: 0.3, HasFullNutrients: true},
		{Code: "1000000000002", Name: "BANANA", Brand: "Chiquita", Kcal: 89, Protein: 1.1, Carbs: 20, Fat: 0.3, HasFullNutrients: true},
		{Code: "1000000000003", Name: "Banana", Brand: "Chiquita", Kcal: 95, Protein: 1.2, Carbs: 22, Fat: 0.3, HasFullNutrients: true},
		{Code: "1000000000004", Name: "Banana", Brand: "Dole", Kcal: 89, Protein: 1.1, Carbs: 20, Fat: 0.3, HasFullNutrients: true},
		{Code: "1000000000005", Name: "Banana", Brand: "Chiquita", HasFullNutrients: false},
	})
	got, err := NewClient(sqldb, "", nil).SearchByName(context.Background(), "banana")
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, p := range got {
		if !p.HasFullNutrients {
			t.Errorf("a product without macros is offered: %+v", p)
		}
		codes[p.Code] = true
	}
	// 0001 and 0002 are the same product: one of them stays.
	if len(got) != 3 || !codes["1000000000003"] || !codes["1000000000004"] || codes["1000000000005"] ||
		codes["1000000000001"] == codes["1000000000002"] {
		t.Errorf("results: %+v", got)
	}
}
