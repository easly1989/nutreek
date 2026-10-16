package app

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/off"
)

func TestSplitSuggested(t *testing.T) {
	mela := &domain.Ingredient{ID: 1, Name: "Mela"}
	riso := &domain.Ingredient{ID: 2, Name: "Riso basmati"}
	melone := &domain.Ingredient{ID: 3, Name: "Melone"}
	rows := []ingredientRow{{Local: mela}, {Local: melone}, {OFF: &off.Product{Code: "1", Name: "Mela Golden"}}}

	sugg, rest := splitSuggested([]*domain.Ingredient{mela, riso}, rows, "MEL")
	if len(sugg) != 1 || sugg[0].Name() != "Mela" {
		t.Errorf("suggeriti = %v, atteso solo Mela", sugg)
	}
	if len(rest) != 2 || rest[0].Name() != "Melone" || rest[1].Name() != "Mela Golden" {
		t.Errorf("altre righe = %v, attese Melone e Mela Golden", rest)
	}

	sugg, rest = splitSuggested([]*domain.Ingredient{mela, riso}, nil, "")
	if len(sugg) != 2 || len(rest) != 0 {
		t.Errorf("senza ricerca: %d suggeriti, %d altre righe", len(sugg), len(rest))
	}
}
