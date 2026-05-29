-- +goose Up
CREATE TABLE nutritionist_plans (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL DEFAULT 'Piano Nutrizionista',
    notes      TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE nutritionist_meal_types (
    id         INTEGER PRIMARY KEY,
    plan_id    INTEGER NOT NULL REFERENCES nutritionist_plans(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE nutritionist_meal_variations (
    id           INTEGER PRIMARY KEY,
    meal_type_id INTEGER NOT NULL REFERENCES nutritionist_meal_types(id) ON DELETE CASCADE,
    name         TEXT,
    sort_order   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE nutritionist_meal_items (
    id            INTEGER PRIMARY KEY,
    variation_id  INTEGER NOT NULL REFERENCES nutritionist_meal_variations(id) ON DELETE CASCADE,
    recipe_id     INTEGER REFERENCES recipes(id) ON DELETE SET NULL,
    ingredient_id INTEGER REFERENCES ingredients(id) ON DELETE SET NULL,
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    quantity_unit TEXT    NOT NULL DEFAULT 'g' CHECK(quantity_unit IN ('g','ml','pz')),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    CHECK ((recipe_id IS NOT NULL) <> (ingredient_id IS NOT NULL))
);

CREATE INDEX idx_nutr_items_variation ON nutritionist_meal_items(variation_id);
CREATE INDEX idx_nutr_variations_type ON nutritionist_meal_variations(meal_type_id);
CREATE INDEX idx_nutr_types_plan      ON nutritionist_meal_types(plan_id);

-- +goose Down
DROP TABLE nutritionist_meal_items;
DROP TABLE nutritionist_meal_variations;
DROP TABLE nutritionist_meal_types;
DROP TABLE nutritionist_plans;
