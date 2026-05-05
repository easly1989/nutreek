-- +goose Up
-- +goose StatementBegin

-- Sgarri ("cheat") associati a un meal: stessa struttura di meal_items
-- (recipe_id XOR ingredient_id, quantity > 0). Quando un meal ha almeno
-- una riga in questa tabella, il calcolo macro del pasto sostituisce gli
-- items "ufficiali" con questi cheat (vedi macros.ForMealEffective).
CREATE TABLE meal_cheat_items (
    id            INTEGER PRIMARY KEY,
    meal_id       INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    recipe_id     INTEGER REFERENCES recipes(id),
    ingredient_id INTEGER REFERENCES ingredients(id),
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    CHECK ((recipe_id IS NOT NULL) <> (ingredient_id IS NOT NULL))
);
CREATE INDEX idx_meal_cheat_items_meal_sort ON meal_cheat_items(meal_id, sort_order);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_meal_cheat_items_meal_sort;
DROP TABLE IF EXISTS meal_cheat_items;
-- +goose StatementEnd
