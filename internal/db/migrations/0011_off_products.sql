-- +goose Up
-- +goose StatementBegin
-- off_products contiene il dump locale di OpenFoodFacts filtrato ai prodotti
-- venduti in Italia (countries_tags include en:italy). Aggiornato dal Syncer
-- in modo idempotente: il dump completo (~3.5M prodotti) viene streammato
-- riga per riga e solo i ~250k italiani finiscono qui.
-- Questa tabella sostituisce le chiamate live a it.openfoodfacts.org/cgi/search.pl
-- per la ricerca testuale: ricerche locali via FTS5, <50ms, zero 503.
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
-- FTS5 con tokenizer unicode61 + remove_diacritics=2: "muesli" matcha
-- "Müesli" senza diacriticFold custom lato Go. content='off_products' usa
-- la tabella sorgente come storage (external content table), evitando di
-- duplicare i dati e mantenendoli sincronizzati via trigger sotto.
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
-- Trigger sync FTS5: pattern raccomandato da SQLite per external content
-- tables. Sull'INSERT propaghiamo rowid+colonne indicizzate; sulla DELETE
-- e UPDATE usiamo il comando speciale 'delete' (riga in fts ha solo il
-- riferimento al rowid sorgente).
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
-- off_sync_state singleton (CHECK id=1): traccia stato sync per UI admin e
-- per il check "sync dovuta?" dello scheduler. dump_etag permette di
-- skippare il download se il server non ha aggiornato (If-None-Match).
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
