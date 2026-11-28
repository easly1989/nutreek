package db

import (
	"context"
	"strings"
	"testing"
)

// Migration 0057: the lookups it was written for read an index and not the whole table.
func TestPerfIndexesAreUsed(t *testing.T) {
	store := newTestStore(t)
	for _, c := range []struct{ name, query, want string }{
		{"OFF status count", `SELECT COUNT(*) FROM off_products WHERE has_full_nutrients = 1`, "idx_off_products_full_nutrients"},
		{"meals using a recipe", `SELECT COUNT(*) FROM meal_items WHERE recipe_id = 1`, "idx_meal_items_recipe"},
		{"meals using an ingredient", `SELECT COUNT(*) FROM meal_items WHERE ingredient_id = 1`, "idx_meal_items_ingredient"},
		{"recipes using an ingredient", `SELECT recipe_id FROM recipe_ingredients WHERE ingredient_id = 1`, "idx_recipe_ingredients_ingredient"},
		{"cheat items by recipe", `SELECT id FROM meal_cheat_items WHERE recipe_id = 1`, "idx_meal_cheat_items_recipe"},
		{"cheat items by ingredient", `SELECT id FROM meal_cheat_items WHERE ingredient_id = 1`, "idx_meal_cheat_items_ingredient"},
		{"plan items by recipe", `SELECT id FROM nutritionist_meal_items WHERE recipe_id = 1`, "idx_nutr_items_recipe"},
		{"plan items by ingredient", `SELECT id FROM nutritionist_meal_items WHERE ingredient_id = 1`, "idx_nutr_items_ingredient"},
		{"photo ETags without the pictures", `SELECT recipe_id, etag FROM recipe_photos`, "COVERING INDEX idx_recipe_photos_etag"},
	} {
		rows, err := store.DB.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+c.query)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		rows.Close()
		if got := strings.Join(plan, " | "); !strings.Contains(got, c.want) {
			t.Errorf("%s: plan %q does not use %s", c.name, got, c.want)
		}
	}
}
