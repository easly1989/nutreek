-- +goose Up
-- The product's serving in grams (or ml), from OFF's serving_quantity (#136).
-- The picker proposes it as the quantity instead of the whole package.
-- NULL = serving unknown; later syncs fill it.
ALTER TABLE off_products ADD COLUMN serving_g REAL;

-- +goose Down
-- As in 0012: SQLite <3.35 does not support DROP COLUMN, the column stays orphaned.
