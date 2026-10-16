package app

import (
	"net/http/httptest"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

func TestIngredientFilter(t *testing.T) {
	latticini := &domain.IngredientCategory{ID: 3, Name: "Latticini"}
	latte := domain.AllergenTag{ID: 7, Name: "Latte"}
	rows := []ingredientRow{
		{Local: &domain.Ingredient{ID: 1, Name: "Yogurt"}, Category: latticini, Allergens: []domain.AllergenTag{latte}},
		{Local: &domain.Ingredient{ID: 2, Name: "Mela"}},
		{OFF: &off.Product{Code: "800", Name: "Yogurt OFF"}},
	}
	names := func(rs []ingredientRow) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Name())
		}
		return out
	}
	cases := []struct {
		query string
		want  []string
		off   bool
	}{
		{"", []string{"Yogurt", "Mela", "Yogurt OFF"}, true},
		{"src=mine", []string{"Yogurt", "Mela"}, false},
		{"src=off", []string{"Yogurt OFF"}, true},
		{"src=boh", []string{"Yogurt", "Mela", "Yogurt OFF"}, true},
		{"category=3", []string{"Yogurt"}, false},
		{"allergen=7", []string{"Yogurt"}, false},
		{"allergen=9", nil, false},
	}
	for _, c := range cases {
		f := parseIngredientFilter(httptest.NewRequest("GET", "/ingredients?"+c.query, nil))
		got := names(f.apply(rows))
		if len(got) != len(c.want) {
			t.Errorf("%q: %v, want %v", c.query, got, c.want)
		} else {
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("%q: %v, want %v", c.query, got, c.want)
					break
				}
			}
		}
		if f.WantsOFF() != c.off {
			t.Errorf("%q: wantsOFF = %v, want %v", c.query, f.WantsOFF(), c.off)
		}
	}
	f := ingredientFilter{Src: "mine", CategoryID: 3, AllergenID: 7}
	if got := f.Query(); got != "allergen=7&category=3&src=mine" {
		t.Errorf("Query() = %q", got)
	}
}
