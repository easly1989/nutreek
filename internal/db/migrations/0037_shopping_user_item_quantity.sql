-- +goose Up
-- Manual shopping items get a numeric quantity and a unit (g, ml or pz), so
-- they can be merged into the matching ingredient row. Both stay NULL for
-- free-text items (quantity_text keeps the display string).
ALTER TABLE shopping_user_items ADD COLUMN quantity REAL;
ALTER TABLE shopping_user_items ADD COLUMN unit TEXT CHECK(unit IS NULL OR unit IN ('g','ml','pz'));

-- +goose Down
-- As in 0013: Down leaves the columns orphaned.
