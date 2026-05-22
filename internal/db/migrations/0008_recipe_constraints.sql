-- +goose Up
-- +goose StatementBegin

-- 1) Risolvi eventuali nomi duplicati per stesso utente (case-insensitive).
--    Mantieni il più vecchio (per created_at+id), gli altri prendono suffisso " (n)".
--    Nota: window function ROW_NUMBER() supportata da SQLite >= 3.25.
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

-- 2) Vincolo unicità nome ricetta per utente (case-insensitive).
CREATE UNIQUE INDEX uniq_recipes_user_name
  ON recipes(user_id, name COLLATE NOCASE);

-- 3) Cambia FK meal_items.recipe_id: da RESTRICT (default) a ON DELETE CASCADE.
--    SQLite non supporta ALTER per FK → ricreazione tabella.
--    Manteniamo lo schema attuale (sort_order da 0003 incluso).
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

-- Ripristina FK RESTRICT su meal_items.recipe_id.
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
