-- +goose Up
-- The owner's OFF status page counts the products with and without complete nutrients. Without
-- an index each count read the whole table (about 250,000 rows with their text columns):
-- 3 s once in production. With this index each count reads only the index.
CREATE INDEX IF NOT EXISTS idx_off_products_full_nutrients ON off_products(has_full_nutrients);

-- +goose Down
DROP INDEX IF EXISTS idx_off_products_full_nutrients;
