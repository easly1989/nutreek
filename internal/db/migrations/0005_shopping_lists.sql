-- +goose Up
-- +goose StatementBegin

-- Shopping list: one per (user, week). week_count lets one shopping trip
-- cover several consecutive weeks.
CREATE TABLE shopping_lists (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start  DATE    NOT NULL,
    week_count  INTEGER NOT NULL DEFAULT 1 CHECK(week_count >= 1 AND week_count <= 4),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, week_start)
);

-- Items generated automatically by aggregating the plan. UNIQUE
-- (shopping_list_id, ingredient_id) makes it possible to keep the checked
-- state on regeneration (DELETE-then-INSERT with a merge).
CREATE TABLE shopping_list_items (
    id                INTEGER PRIMARY KEY,
    shopping_list_id  INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    ingredient_id     INTEGER NOT NULL REFERENCES ingredients(id),
    total_quantity    REAL    NOT NULL CHECK(total_quantity > 0),
    checked           INTEGER NOT NULL DEFAULT 0 CHECK(checked IN (0,1)),
    sort_order        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(shopping_list_id, ingredient_id)
);

-- Items the user adds by hand: free text (description + quantity).
-- They persist across list regenerations (the Generate algorithm does not
-- touch this table).
CREATE TABLE shopping_user_items (
    id                INTEGER PRIMARY KEY,
    shopping_list_id  INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    description       TEXT    NOT NULL,
    quantity_text     TEXT,
    checked           INTEGER NOT NULL DEFAULT 0 CHECK(checked IN (0,1)),
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS shopping_user_items;
DROP TABLE IF EXISTS shopping_list_items;
DROP TABLE IF EXISTS shopping_lists;
-- +goose StatementEnd
