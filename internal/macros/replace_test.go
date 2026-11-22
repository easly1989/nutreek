package macros

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func ing(id int64, name string, kcal, p, c, f float64) *domain.Ingredient {
	return &domain.Ingredient{ID: id, Name: name, Unit: domain.UnitGram, KcalPer100: kcal, ProteinPer100: p, CarbsPer100: c, FatPer100: f}
}

// The row keeps its weight: pieces turn into grams, ml stay ml.
func TestReplacementQuantity(t *testing.T) {
	banana := &domain.Ingredient{Unit: domain.UnitGram}
	w := 120.0
	banana.UnitWeightG = &w
	if q, u := ReplacementQuantity(2, "pz", banana); q != 240 || u != "g" {
		t.Errorf("2 pz of 120 g = %v %s, want 240 g", q, u)
	}
	if q, u := ReplacementQuantity(150, "g", banana); q != 150 || u != "g" {
		t.Errorf("150 g = %v %s, want 150 g", q, u)
	}
	if q, u := ReplacementQuantity(200, "ml", banana); q != 200 || u != "ml" {
		t.Errorf("200 ml = %v %s, want 200 ml", q, u)
	}
}

// The same category comes first, then the nearest in energy and macros; the
// current ingredient never shows and limit cuts the list.
func TestRankReplacements(t *testing.T) {
	current := ing(1, "Yogurt greco 0%", 58, 10, 4, 0.3)
	grams := 150.0
	cur := FromIngredient(*current, grams)
	cands := []*domain.Ingredient{
		current,
		ing(2, "Ricotta", 140, 8, 2.6, 11),        // far, other category
		ing(3, "Skyr", 62, 11, 3.5, 0.2),          // near, same category
		ing(4, "Yogurt greco 2%", 69, 9, 4, 2),    // same category, farther than Skyr
		ing(5, "Yogurt magro", 57, 4.9, 7.2, 0.4), // other category but nearer than Ricotta
	}
	same := map[int64]bool{3: true, 4: true}

	got := RankReplacements(cur, grams, current.ID, cands, same, 0)
	var order []int64
	for _, r := range got {
		order = append(order, r.Ingredient.ID)
	}
	want := []int64{3, 4, 5, 2}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	if !got[0].SameCategory || got[2].SameCategory {
		t.Error("SameCategory must follow the set")
	}

	// The difference is the new row minus the current one, at the same weight.
	wantKcal := 62*1.5 - 58*1.5
	if !almostEqual(got[0].Diff.Kcal, wantKcal) || !almostEqual(got[0].Macros.Kcal, 62*1.5) {
		t.Errorf("skyr diff kcal = %v (macros %v), want %v", got[0].Diff.Kcal, got[0].Macros.Kcal, wantKcal)
	}

	if cut := RankReplacements(cur, grams, current.ID, cands, same, 2); len(cut) != 2 || cut[1].Ingredient.ID != 4 {
		t.Errorf("limit 2 = %d rows, want the first two", len(cut))
	}

	// Equal distance and category: by name.
	tie := RankReplacements(cur, grams, 0, []*domain.Ingredient{ing(8, "Zeta", 60, 10, 4, 0.3), ing(9, "Alfa", 60, 10, 4, 0.3)}, nil, 0)
	if tie[0].Ingredient.Name != "Alfa" {
		t.Errorf("tie order = %s first, want Alfa", tie[0].Ingredient.Name)
	}
}
