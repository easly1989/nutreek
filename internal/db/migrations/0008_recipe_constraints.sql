-- +goose Up
-- +goose StatementBegin

-- 1) Resolve any duplicate names of the same user (case-insensitive).
--    Keep the oldest (by created_at+id); the others get a " (n)" suffix.
--    Note: the ROW_NUMBER() window function needs SQLite >= 3.25.
WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY user_id, LOWER(name)
           ORDER BY created_at, id
         ) AS rn
  FROM recipes
)
UPDATE recipes
SET name = name || ' (' || (SELECT rn FROM ranked WHERE ranked.id = recipes.id) || ')'
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

-- 2) Unique recipe name per user (case-insensitive).
CREATE UNIQUE INDEX uniq_recipes_user_name
  ON recipes(user_id, name COLLATE NOCASE);

-- 3) Change the FK meal_items.recipe_id: from RESTRICT (the default) to ON DELETE CASCADE.
--    SQLite has no ALTER for FKs → rebuild the table.
--    Keep the current schema (sort_order from 0003 included).
PRAGMA foreign_keys = OFF;

CREATE TABLE meal_items_new (
    id            INTEGER PRIMARY KEY,
    meal_id       INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    recipe_id     INTEGER REFERENCES recipes(id) ON DELETE CASCADE,
    ingredient_id INTEGER REFERENCES ingredients(id),
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    CHECK ((recipe_id IS NOT NULL) <> (ingredient_id IS NOT NULL))
);

INSERT INTO meal_items_new (id, meal_id, recipe_id, ingredient_id, quantity, sort_order)
  SELECT id, meal_id, recipe_id, ingredient_id, quantity, sort_order FROM meal_items;

DROP TABLE meal_items;
ALTER TABLE meal_items_new RENAME TO meal_items;

CREATE INDEX idx_meal_items_meal      ON meal_items(meal_id);
CREATE INDEX idx_meal_items_meal_sort ON meal_items(meal_id, sort_order);

PRAGMA foreign_keys = ON;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS uniq_recipes_user_name;

-- Restore the RESTRICT FK on meal_items.recipe_id.
PRAGMA foreign_keys = OFF;

CREATE TABLE meal_items_old (
    id            INTEGER PRIMARY KEY,
    meal_id       INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    recipe_id     INTEGER REFERENCES recipes(id),
    ingredient_id INTEGER REFERENCES ingredients(id),
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    CHECK ((recipe_id IS NOT NULL) <> (ingredient_id IS NOT NULL))
);

INSERT INTO meal_items_old (id, meal_id, recipe_id, ingredient_id, quantity, sort_order)
  SELECT id, meal_id, recipe_id, ingredient_id, quantity, sort_order FROM meal_items;

DROP TABLE meal_items;
ALTER TABLE meal_items_old RENAME TO meal_items;

CREATE INDEX idx_meal_items_meal      ON meal_items(meal_id);
CREATE INDEX idx_meal_items_meal_sort ON meal_items(meal_id, sort_order);

PRAGMA foreign_keys = ON;

-- +goose StatementEnd
