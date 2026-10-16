-- +goose Up
-- unit_weight_g: grams per single unit of the ingredient (e.g. 150 for a
-- 150g banana). NULL = an ingredient without "pieces" support (g/ml only).
ALTER TABLE ingredients ADD COLUMN unit_weight_g REAL;

-- quantity_unit: the unit the user gave the quantity in, in the plan.
-- 'g' = grams/ml (the historical behaviour, a DEFAULT compatible with existing rows).
-- 'pz' = pieces; the macro total uses quantity × ingredient.unit_weight_g.
ALTER TABLE meal_items ADD COLUMN quantity_unit TEXT NOT NULL DEFAULT 'g'
    CHECK(quantity_unit IN ('g','ml','pz'));

-- +goose Down
-- SQLite <3.35 does not support an inline DROP COLUMN. On SQLite >=3.35
-- ALTER TABLE ... DROP COLUMN would work. Down leaves the columns orphaned.
