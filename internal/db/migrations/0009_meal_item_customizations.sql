-- +goose Up
-- +goose StatementBegin
-- meal_item_customizations: ingredient overrides for a single occurrence of
-- a recipe in a meal. When rows exist for a meal_item, the macros are
-- computed on them instead of on the recipe's base ingredients. The base
-- recipe stays unchanged in the DB.
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
