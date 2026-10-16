-- +goose Up
-- Adds the OFF product's thumbnail URL (image_front_small_url from the JSONL
-- dump or from the live API). Used to show thumbnails in the ingredient search.
-- DEFAULT '' for compatibility with existing rows (later syncs fill it).
ALTER TABLE off_products ADD COLUMN image_url TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite <3.35 does not support DROP COLUMN; the column is left orphaned.
-- On SQLite >=3.35 this would work: ALTER TABLE off_products DROP COLUMN image_url;
