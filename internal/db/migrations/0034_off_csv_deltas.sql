-- +goose Up
-- The Syncer bootstraps from the CSV export, then applies the daily deltas
-- (#211). high_water is the newest last_modified_t imported (unix seconds):
-- the deltas that end after it are still to apply. The dump's ETag is gone.
ALTER TABLE off_sync_state ADD COLUMN high_water INTEGER;
ALTER TABLE off_sync_state ADD COLUMN last_bootstrap_at INTEGER;
ALTER TABLE off_sync_state ADD COLUMN last_delta_at INTEGER;
ALTER TABLE off_sync_state DROP COLUMN dump_etag;

-- A bootstrap fills this table first and swaps it into off_products only
-- once it is whole, so searches keep the old products while it downloads.
CREATE TABLE off_products_staging (
    barcode             TEXT PRIMARY KEY,
    product_name        TEXT NOT NULL,
    brands              TEXT NOT NULL DEFAULT '',
    quantity            TEXT NOT NULL DEFAULT '',
    kcal_per_100        REAL NOT NULL DEFAULT 0,
    protein_per_100     REAL NOT NULL DEFAULT 0,
    carbs_per_100       REAL NOT NULL DEFAULT 0,
    fat_per_100         REAL NOT NULL DEFAULT 0,
    has_full_nutrients  INTEGER NOT NULL DEFAULT 0,
    image_url           TEXT NOT NULL DEFAULT '',
    serving_g           REAL,
    updated_at          INTEGER NOT NULL
);

-- +goose Down
DROP TABLE off_products_staging;
ALTER TABLE off_sync_state ADD COLUMN dump_etag TEXT;
ALTER TABLE off_sync_state DROP COLUMN last_delta_at;
ALTER TABLE off_sync_state DROP COLUMN last_bootstrap_at;
ALTER TABLE off_sync_state DROP COLUMN high_water;
