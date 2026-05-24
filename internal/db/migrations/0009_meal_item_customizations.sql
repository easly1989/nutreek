-- +goose Up
-- +goose StatementBegin
-- meal_item_customizations: sovrascrittura ingredienti per una singola
-- occorrenza di una ricetta in un pasto. Se esistono righe per un meal_item,
-- i macro vengono calcolati su queste invece che sugli ingredienti base della
-- ricetta. La ricetta di base rimane invariata nel DB.
CREATE TABLE meal_item_customizations (
    id            INTEGER PRIMARY KEY,
    meal_item_id  INTEGER NOT NULL REFERENCES meal_items(id) ON DELETE CASCADE,
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id),
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    UNIQUE(meal_item_id, ingredient_id)
);
CREATE INDEX idx_mic_meal_item ON meal_item_customizations(meal_item_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS meal_item_customizations;
-- +goose StatementEnd
