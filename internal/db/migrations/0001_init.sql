-- +goose Up
-- +goose StatementBegin

-- Utenti
CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    email         TEXT    UNIQUE NOT NULL,
    password_hash TEXT    NOT NULL,
    display_name  TEXT    NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Ingredienti (condivisi fra tutti gli utenti).
-- `source='openfoodfacts'` => provenienza da OFF (macro sola lettura).
-- `source='local'` => inserito a mano da un utente, candidato alla
-- riconciliazione automatica contro OFF (vedi `last_verified_at`).
CREATE TABLE ingredients (
    id               INTEGER PRIMARY KEY,
    source           TEXT    NOT NULL CHECK(source IN ('openfoodfacts','local')),
    off_barcode      TEXT    UNIQUE,
    name             TEXT    NOT NULL,
    brand            TEXT,
    unit             TEXT    NOT NULL DEFAULT 'g' CHECK(unit IN ('g','ml')),
    kcal_per_100     REAL    NOT NULL,
    protein_per_100  REAL    NOT NULL,
    carbs_per_100    REAL    NOT NULL,
    fat_per_100      REAL    NOT NULL,
    created_by       INTEGER REFERENCES users(id),
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_verified_at DATETIME,
    superseded_by    INTEGER REFERENCES ingredients(id)
);
CREATE INDEX idx_ingredients_name       ON ingredients(name);
CREATE INDEX idx_ingredients_superseded ON ingredients(superseded_by);

-- Ricette
CREATE TABLE recipes (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    description TEXT,
    servings    INTEGER NOT NULL DEFAULT 1 CHECK(servings > 0),
    is_public   INTEGER NOT NULL DEFAULT 0 CHECK(is_public IN (0,1)),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_recipes_user   ON recipes(user_id);
CREATE INDEX idx_recipes_public ON recipes(is_public) WHERE is_public = 1;

CREATE TABLE recipe_ingredients (
    id            INTEGER PRIMARY KEY,
    recipe_id     INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id),
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    sort_order    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_recipe_ingredients_recipe ON recipe_ingredients(recipe_id);

-- Template pasti riutilizzabili
CREATE TABLE meal_templates (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    is_active  INTEGER NOT NULL DEFAULT 0 CHECK(is_active IN (0,1)),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE meal_template_slots (
    id           INTEGER PRIMARY KEY,
    template_id  INTEGER NOT NULL REFERENCES meal_templates(id) ON DELETE CASCADE,
    day_of_week  INTEGER NOT NULL CHECK(day_of_week BETWEEN -1 AND 6), -- -1 = ogni giorno
    name         TEXT    NOT NULL,
    sort_order   INTEGER NOT NULL DEFAULT 0
);

-- Piano settimanale concreto (una riga per utente+settimana)
CREATE TABLE weekly_plans (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start  DATE    NOT NULL,
    template_id INTEGER REFERENCES meal_templates(id),
    UNIQUE(user_id, week_start)
);

CREATE TABLE meals (
    id              INTEGER PRIMARY KEY,
    weekly_plan_id  INTEGER NOT NULL REFERENCES weekly_plans(id) ON DELETE CASCADE,
    day_of_week     INTEGER NOT NULL CHECK(day_of_week BETWEEN 0 AND 6),
    name            TEXT    NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_meals_plan_day ON meals(weekly_plan_id, day_of_week);

-- Righe di un pasto: o un ingrediente ad hoc (quantita in grammi/ml)
-- o una ricetta (quantita in "porzioni"). Esattamente uno dei due.
CREATE TABLE meal_items (
    id            INTEGER PRIMARY KEY,
    meal_id       INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    recipe_id     INTEGER REFERENCES recipes(id),
    ingredient_id INTEGER REFERENCES ingredients(id),
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    CHECK ((recipe_id IS NOT NULL) <> (ingredient_id IS NOT NULL))
);
CREATE INDEX idx_meal_items_meal ON meal_items(meal_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS meal_items;
DROP TABLE IF EXISTS meals;
DROP TABLE IF EXISTS weekly_plans;
DROP TABLE IF EXISTS meal_template_slots;
DROP TABLE IF EXISTS meal_templates;
DROP TABLE IF EXISTS recipe_ingredients;
DROP TABLE IF EXISTS recipes;
DROP TABLE IF EXISTS ingredients;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
