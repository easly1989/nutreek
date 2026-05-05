-- +goose Up
-- +goose StatementBegin

-- Lista della spesa: una per (utente, settimana). week_count permette
-- di coprire piu' settimane consecutive in una sola spesa.
CREATE TABLE shopping_lists (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start  DATE    NOT NULL,
    week_count  INTEGER NOT NULL DEFAULT 1 CHECK(week_count >= 1 AND week_count <= 4),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, week_start)
);

-- Item generati automaticamente dall'aggregazione del piano. UNIQUE
-- (shopping_list_id, ingredient_id) abilita preservare lo stato checked
-- su rigenerazione (DELETE-then-INSERT con merge).
CREATE TABLE shopping_list_items (
    id                INTEGER PRIMARY KEY,
    shopping_list_id  INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    ingredient_id     INTEGER NOT NULL REFERENCES ingredients(id),
    total_quantity    REAL    NOT NULL CHECK(total_quantity > 0),
    checked           INTEGER NOT NULL DEFAULT 0 CHECK(checked IN (0,1)),
    sort_order        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(shopping_list_id, ingredient_id)
);

-- Item aggiunti a mano dall'utente: testo libero (description + quantity).
-- Persistono fra rigenerazioni della lista (l'algoritmo Generate non
-- tocca questa tabella).
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
