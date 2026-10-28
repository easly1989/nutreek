-- +goose Up
-- quantity_unit: the unit a recipe row was entered in (#206), as meal_items
-- has since 0013. 'pz' = pieces, counted as quantity × ingredient.unit_weight_g.
-- Existing rows were converted to grams on insert: they stay 'g'.
ALTER TABLE recipe_ingredients ADD COLUMN quantity_unit TEXT NOT NULL DEFAULT 'g'
    CHECK(quantity_unit IN ('g','ml','pz'));

-- +goose Down
-- As in 0013: Down leaves the column orphaned.
