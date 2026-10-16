package off

import (
	"encoding/json"
	"testing"
)

func TestParseServingG(t *testing.T) {
	cases := []struct {
		name     string
		quantity string // JSON grezzo di serving_quantity ("" = assente)
		unit     string
		size     string
		want     float64 // 0 = nil
	}{
		{"numero in grammi", `30`, "g", "30 g", 30},
		{"numero senza unita'", `40`, "", "", 40},
		{"stringa", `"25.5"`, "g", "", 25.5},
		{"millilitri", `250`, "ml", "", 250},
		{"unita' diversa, ripiega su serving_size", `1`, "piece", "1 biscotto (12,5 g)", 12.5},
		{"solo serving_size", ``, "", "30g", 30},
		{"null", `null`, "", "", 0},
		{"zero", `0`, "g", "", 0},
		{"assurda", `5000`, "g", "", 0},
		{"niente in g/ml", ``, "", "1 pezzo", 0},
		{"niente", ``, "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseServingG(json.RawMessage(c.quantity), c.unit, c.size)
			switch {
			case c.want == 0 && got != nil:
				t.Fatalf("atteso nil, avuto %v", *got)
			case c.want != 0 && (got == nil || *got != c.want):
				t.Fatalf("atteso %v, avuto %v", c.want, got)
			}
		})
	}
}

// La risposta dell'API per codice porta la porzione nel prodotto.
func TestToProductServing(t *testing.T) {
	var resp productResponse
	body := `{"status":1,"code":"8012345678907","product":{"code":"8012345678907","product_name":"Fiocchi d'avena","quantity":"500 g",` +
		`"serving_quantity":"40","serving_quantity_unit":"g","serving_size":"40 g",` +
		`"nutriments":{"energy-kcal_100g":372,"proteins_100g":13.5,"carbohydrates_100g":58.7,"fat_100g":7}}}`
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	p := resp.Product.toProduct()
	if p.ServingG == nil || *p.ServingG != 40 {
		t.Fatalf("ServingG = %v, atteso 40", p.ServingG)
	}
	if p.UnitWeightG == nil || *p.UnitWeightG != 500 {
		t.Fatalf("UnitWeightG = %v, atteso 500 (la confezione)", p.UnitWeightG)
	}
}
