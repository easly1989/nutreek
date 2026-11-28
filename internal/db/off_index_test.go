package db

import (
	"context"
	"strings"
	"testing"
)

// The owner's OFF status counts products by has_full_nutrients: the query must read the index
// of migration 0057, not the whole table.
func TestOFFFullNutrientsCountUsesIndex(t *testing.T) {
	store := newTestStore(t)
	rows, err := store.DB.QueryContext(context.Background(), `EXPLAIN QUERY PLAN SELECT COUNT(*) FROM off_products WHERE has_full_nutrients = 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if got := strings.Join(plan, " | "); !strings.Contains(got, "idx_off_products_full_nutrients") {
		t.Errorf("plan = %q, want a search on idx_off_products_full_nutrients", got)
	}
}
