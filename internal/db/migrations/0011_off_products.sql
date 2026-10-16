-- +goose Up
-- +goose StatementBegin
-- off_products holds the local OpenFoodFacts dump filtered to products sold
-- in Italy (countries_tags includes en:italy). The Syncer updates it
-- idempotently: the full dump (~3.5M products) is streamed line by line and
-- only the ~250k Italian ones land here.
-- This table replaces the live calls to it.openfoodfacts.org/cgi/search.pl
-- for text search: local searches via FTS5, <50ms, zero 503s.
CREATE TABLE off_products (
    barcode             TEXT PRIMARY KEY,
    product_name        TEXT NOT NULL,
    brands              TEXT NOT NULL DEFAULT '',
    quantity            TEXT NOT NULL DEFAULT '',
    kcal_per_100        REAL NOT NULL DEFAULT 0,
    protein_per_100     REAL NOT NULL DEFAULT 0,
    carbs_per_100       REAL NOT NULL DEFAULT 0,
    fat_per_100         REAL NOT NULL DEFAULT 0,
    has_full_nutrients  INTEGER NOT NULL DEFAULT 0,
    updated_at          INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
-- FTS5 with the unicode61 tokenizer + remove_diacritics=2: "muesli" matches
-- "Müesli" without a custom diacritic fold in Go. content='off_products' uses
-- the source table as storage (external content table), so the data is not
-- duplicated and stays in sync through the triggers below.
CREATE VIRTUAL TABLE off_products_fts USING fts5(
    barcode UNINDEXED,
    product_name,
    brands,
    content='off_products',
    content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'
);
-- +goose StatementEnd

-- +goose StatementBegin
-- FTS5 sync triggers: the pattern SQLite recommends for external content
-- tables. On INSERT we propagate rowid+indexed columns; on DELETE and
-- UPDATE we use the special 'delete' command (the fts row only holds the
-- reference to the source rowid).
CREATE TRIGGER off_products_ai AFTER INSERT ON off_products BEGIN
    INSERT INTO off_products_fts(rowid, barcode, product_name, brands)
    VALUES (new.rowid, new.barcode, new.product_name, new.brands);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER off_products_ad AFTER DELETE ON off_products BEGIN
    INSERT INTO off_products_fts(off_products_fts, rowid, barcode, product_name, brands)
    VALUES('delete', old.rowid, old.barcode, old.product_name, old.brands);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER off_products_au AFTER UPDATE ON off_products BEGIN
    INSERT INTO off_products_fts(off_products_fts, rowid, barcode, product_name, brands)
    VALUES('delete', old.rowid, old.barcode, old.product_name, old.brands);
    INSERT INTO off_products_fts(rowid, barcode, product_name, brands)
    VALUES (new.rowid, new.barcode, new.product_name, new.brands);
END;
-- +goose StatementEnd

-- +goose StatementBegin
-- off_sync_state singleton (CHECK id=1): tracks the sync state for the admin
-- UI and for the scheduler's "is a sync due?" check. dump_etag lets the
-- download be skipped when the server has not updated (If-None-Match).
CREATE TABLE off_sync_state (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    last_synced_at      INTEGER,
    last_attempt_at     INTEGER,
    last_error          TEXT,
    products_count      INTEGER NOT NULL DEFAULT 0,
    dump_etag           TEXT
);
INSERT INTO off_sync_state (id) VALUES (1);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS off_sync_state;
DROP TRIGGER IF EXISTS off_products_au;
DROP TRIGGER IF EXISTS off_products_ad;
DROP TRIGGER IF EXISTS off_products_ai;
DROP TABLE IF EXISTS off_products_fts;
DROP TABLE IF EXISTS off_products;
-- +goose StatementEnd
