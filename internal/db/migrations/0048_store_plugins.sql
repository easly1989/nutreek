-- +goose Up
-- Store plugins (#239): HTTP services a user connects to search a store's catalogue.
-- The token is sealed with NUTREEK_SECRET_KEY (AES-GCM) before it is stored.
CREATE TABLE store_plugins (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT    NOT NULL,
    base_url     TEXT    NOT NULL,
    token_sealed TEXT    NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    position     INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_store_plugins_user ON store_plugins (user_id, position);

-- Where OpenFoodFacts stands among the user's search sources: off_rank is how many
-- plugins come before it (0 = first, as before plugins existed); it can be turned off.
ALTER TABLE users ADD COLUMN off_rank INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN off_enabled INTEGER NOT NULL DEFAULT 1 CHECK (off_enabled IN (0, 1));

-- The store product an ingredient was made from (picked in a search), with the price
-- and availability it had when last checked. One product for each ingredient and the
-- other way round, for a user.
CREATE TABLE ingredient_store_products (
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    plugin_id     INTEGER NOT NULL REFERENCES store_plugins(id) ON DELETE CASCADE,
    product_id    TEXT    NOT NULL,
    product_name  TEXT    NOT NULL,
    size_qty      REAL    NOT NULL DEFAULT 0,
    size_unit     TEXT    NOT NULL DEFAULT '',
    price_cents   INTEGER,
    currency      TEXT    NOT NULL DEFAULT 'EUR',
    available     INTEGER NOT NULL DEFAULT 1,
    checked_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, ingredient_id),
    UNIQUE (user_id, plugin_id, product_id)
);

-- +goose Down
DROP TABLE ingredient_store_products;
ALTER TABLE users DROP COLUMN off_enabled;
ALTER TABLE users DROP COLUMN off_rank;
DROP TABLE store_plugins;
