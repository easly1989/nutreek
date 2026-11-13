package app

import (
	"fmt"
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestPaginateRowsReachesEveryIngredient(t *testing.T) {
	var rows []ingredientRow
	for i := 0; i < 120; i++ {
		rows = append(rows, ingredientRow{Local: &domain.Ingredient{ID: int64(i), Name: fmt.Sprintf("Item %03d", i)}})
	}
	f := ingredientFilter{Src: "mine"}
	seen := 0
	for p := 1; p <= 3; p++ {
		page, pg := paginateRows(rows, fmt.Sprint(p), f)
		seen += len(page)
		if pg.Pages != 3 || pg.Total != 120 || pg.Page != p {
			t.Errorf("page %d: %+v", p, pg)
		}
	}
	if seen != 120 {
		t.Errorf("pages cover %d rows, want 120", seen)
	}
	last, pg := paginateRows(rows, "99", f) // out of range clamps to the last page
	if pg.Page != 3 || len(last) != 20 || pg.NextHref != "" || pg.PrevHref != "/ingredients?page=2&src=mine" {
		t.Errorf("clamp: %d rows, %+v", len(last), pg)
	}
}
