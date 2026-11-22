-- +goose Up
-- The package size ("500 g", "2 x 125 g") joins the full-text index, so a query
-- like "yogurt greco 125" finds a product by name and size together (#224).
-- FTS5 cannot add a column in place: the index and its triggers are made again
-- and filled from off_products (external content), which reads each row once.

-- +goose StatementBegin
DROP TRIGGER IF EXISTS off_products_ai;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER IF EXISTS off_products_ad;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER IF EXISTS off_products_au;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS off_products_fts;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE VIRTUAL TABLE off_products_fts USING fts5(
    barcode UNINDEXED,
    product_name,
    brands,
    quantity,
    content='off_products',
    content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER off_products_ai AFTER INSERT ON off_products BEGIN
    INSERT INTO off_products_fts(rowid, barcode, product_name, brands, quantity)
    VALUES (new.rowid, new.barcode, new.product_name, new.brands, new.quantity);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER off_products_ad AFTER DELETE ON off_products BEGIN
    INSERT INTO off_products_fts(off_products_fts, rowid, barcode, product_name, brands, quantity)
    VALUES('delete', old.rowid, old.barcode, old.product_name, old.brands, old.quantity);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER off_products_au AFTER UPDATE ON off_products BEGIN
    INSERT INTO off_products_fts(off_products_fts, rowid, barcode, product_name, brands, quantity)
    VALUES('delete', old.rowid, old.barcode, old.product_name, old.brands, old.quantity);
    INSERT INTO off_products_fts(rowid, barcode, product_name, brands, quantity)
    VALUES (new.rowid, new.barcode, new.product_name, new.brands, new.quantity);
END;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO off_products_fts(off_products_fts) VALUES('rebuild');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS off_products_ai;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER IF EXISTS off_products_ad;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER IF EXISTS off_products_au;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS off_products_fts;
-- +goose StatementEnd

-- +goose StatementBegin
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
INSERT INTO off_products_fts(off_products_fts) VALUES('rebuild');
-- +goose StatementEnd
