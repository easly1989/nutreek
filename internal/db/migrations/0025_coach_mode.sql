-- +goose Up
-- +goose StatementBegin

-- 1) Removes the UNIQUE constraint on nutritionist_plans.user_id and adds
--    is_active: a user can have N plans (a library), but only one active.
--
--    SQLite has no DROP CONSTRAINT -> rebuild the table. Note well:
--    PRAGMA foreign_keys is a no-op inside the goose transaction, so with
--    FKs on, dropping the parent would cascade-delete the children. That is
--    why the whole chain is rebuilt (plans -> meal_types -> variations ->
--    items): copy into the _new tables (parents before children), drop the
--    old ones (children before parents), rename _new -> the original name
--    (parents before children: each RENAME rewrites the FK references in the
--    other tables).

CREATE TABLE nutritionist_plans_new (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL DEFAULT 'Piano Nutrizionista',
    notes      TEXT,
    is_active  INTEGER NOT NULL DEFAULT 1 CHECK(is_active IN (0,1)),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- The existing plans (one per user) all stay active.
INSERT INTO nutritionist_plans_new (id, user_id, name, notes, is_active, created_at, updated_at)
  SELECT id, user_id, name, notes, 1, created_at, updated_at FROM nutritionist_plans;

CREATE TABLE nutritionist_meal_types_new (
    id         INTEGER PRIMARY KEY,
    plan_id    INTEGER NOT NULL REFERENCES nutritionist_plans_new(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);
INSERT INTO nutritionist_meal_types_new (id, plan_id, name, sort_order)
  SELECT id, plan_id, name, sort_order FROM nutritionist_meal_types;

CREATE TABLE nutritionist_meal_variations_new (
    id           INTEGER PRIMARY KEY,
    meal_type_id INTEGER NOT NULL REFERENCES nutritionist_meal_types_new(id) ON DELETE CASCADE,
    name         TEXT,
    sort_order   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO nutritionist_meal_variations_new (id, meal_type_id, name, sort_order)
  SELECT id, meal_type_id, name, sort_order FROM nutritionist_meal_variations;

CREATE TABLE nutritionist_meal_items_new (
    id            INTEGER PRIMARY KEY,
    variation_id  INTEGER NOT NULL REFERENCES nutritionist_meal_variations_new(id) ON DELETE CASCADE,
    recipe_id     INTEGER REFERENCES recipes(id) ON DELETE SET NULL,
    ingredient_id INTEGER REFERENCES ingredients(id) ON DELETE SET NULL,
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    quantity_unit TEXT    NOT NULL DEFAULT 'g' CHECK(quantity_unit IN ('g','ml','pz')),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    CHECK ((recipe_id IS NOT NULL) <> (ingredient_id IS NOT NULL))
);
INSERT INTO nutritionist_meal_items_new (id, variation_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order)
  SELECT id, variation_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order FROM nutritionist_meal_items;

-- Drop the old tables: children before parents (so the implicit DELETE of
-- the DROP never finds children to cascade to).
DROP TABLE nutritionist_meal_items;
DROP TABLE nutritionist_meal_variations;
DROP TABLE nutritionist_meal_types;
DROP TABLE nutritionist_plans;

-- Rename: parents before children. Each rename updates the FK references of
-- the tables pointing to the renamed name.
ALTER TABLE nutritionist_plans_new           RENAME TO nutritionist_plans;
ALTER TABLE nutritionist_meal_types_new      RENAME TO nutritionist_meal_types;
ALTER TABLE nutritionist_meal_variations_new RENAME TO nutritionist_meal_variations;
ALTER TABLE nutritionist_meal_items_new      RENAME TO nutritionist_meal_items;

CREATE INDEX idx_nutr_items_variation ON nutritionist_meal_items(variation_id);
CREATE INDEX idx_nutr_variations_type ON nutritionist_meal_variations(meal_type_id);
CREATE INDEX idx_nutr_types_plan      ON nutritionist_meal_types(plan_id);
CREATE INDEX idx_nutritionist_plans_user ON nutritionist_plans(user_id);
-- At most one active plan per user.
CREATE UNIQUE INDEX uniq_nutritionist_plans_active
  ON nutritionist_plans(user_id) WHERE is_active = 1;

-- 2) User role: 'user' (the default) or 'coach'.
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'
    CHECK(role IN ('user','coach'));

-- 3) Coach <-> client relationship with an assigned plan.
--    UNIQUE(client_id): a client has at most one coach.
--    assigned_plan_id: the plan from the coach's library assigned to the
--    client (NULL = no plan assigned, the client uses their own).
CREATE TABLE coach_clients (
    id               INTEGER PRIMARY KEY,
    coach_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id        INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    assigned_plan_id INTEGER REFERENCES nutritionist_plans(id) ON DELETE SET NULL,
    assigned_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(coach_id, client_id),
    CHECK(coach_id <> client_id)
);
CREATE INDEX idx_coach_clients_coach ON coach_clients(coach_id);

-- 4) Historical freeze: which nutrition plan was in use when the week was
--    generated/applied. Traceability only, nullable.
ALTER TABLE weekly_plans ADD COLUMN source_plan_id INTEGER
    REFERENCES nutritionist_plans(id) ON DELETE SET NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- The tables/columns referencing nutritionist_plans must be removed before
-- the chain is rebuilt.
ALTER TABLE weekly_plans DROP COLUMN source_plan_id;
DROP TABLE IF EXISTS coach_clients;
ALTER TABLE users DROP COLUMN role;

-- Restore the UNIQUE constraint on user_id keeping only the active plan
-- and its contents (inactive library plans are lost).
CREATE TABLE nutritionist_plans_old (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL DEFAULT 'Piano Nutrizionista',
    notes      TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO nutritionist_plans_old (id, user_id, name, notes, created_at, updated_at)
  SELECT id, user_id, name, notes, created_at, updated_at
  FROM nutritionist_plans WHERE is_active = 1;

CREATE TABLE nutritionist_meal_types_old (
    id         INTEGER PRIMARY KEY,
    plan_id    INTEGER NOT NULL REFERENCES nutritionist_plans_old(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);
INSERT INTO nutritionist_meal_types_old (id, plan_id, name, sort_order)
  SELECT t.id, t.plan_id, t.name, t.sort_order
  FROM nutritionist_meal_types t
  JOIN nutritionist_plans p ON p.id = t.plan_id AND p.is_active = 1;

CREATE TABLE nutritionist_meal_variations_old (
    id           INTEGER PRIMARY KEY,
    meal_type_id INTEGER NOT NULL REFERENCES nutritionist_meal_types_old(id) ON DELETE CASCADE,
    name         TEXT,
    sort_order   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO nutritionist_meal_variations_old (id, meal_type_id, name, sort_order)
  SELECT v.id, v.meal_type_id, v.name, v.sort_order
  FROM nutritionist_meal_variations v
  JOIN nutritionist_meal_types_old t ON t.id = v.meal_type_id;

CREATE TABLE nutritionist_meal_items_old (
    id            INTEGER PRIMARY KEY,
    variation_id  INTEGER NOT NULL REFERENCES nutritionist_meal_variations_old(id) ON DELETE CASCADE,
    recipe_id     INTEGER REFERENCES recipes(id) ON DELETE SET NULL,
    ingredient_id INTEGER REFERENCES ingredients(id) ON DELETE SET NULL,
    quantity      REAL    NOT NULL CHECK(quantity > 0),
    quantity_unit TEXT    NOT NULL DEFAULT 'g' CHECK(quantity_unit IN ('g','ml','pz')),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    CHECK ((recipe_id IS NOT NULL) <> (ingredient_id IS NOT NULL))
);
INSERT INTO nutritionist_meal_items_old (id, variation_id, recipe_id, ingredient_id, quantity, quantity_unit, sort_order)
  SELECT i.id, i.variation_id, i.recipe_id, i.ingredient_id, i.quantity, i.quantity_unit, i.sort_order
  FROM nutritionist_meal_items i
  JOIN nutritionist_meal_variations_old v ON v.id = i.variation_id;

DROP TABLE nutritionist_meal_items;
DROP TABLE nutritionist_meal_variations;
DROP TABLE nutritionist_meal_types;
DROP TABLE nutritionist_plans;

ALTER TABLE nutritionist_plans_old           RENAME TO nutritionist_plans;
ALTER TABLE nutritionist_meal_types_old      RENAME TO nutritionist_meal_types;
ALTER TABLE nutritionist_meal_variations_old RENAME TO nutritionist_meal_variations;
ALTER TABLE nutritionist_meal_items_old      RENAME TO nutritionist_meal_items;

CREATE INDEX idx_nutr_items_variation ON nutritionist_meal_items(variation_id);
CREATE INDEX idx_nutr_variations_type ON nutritionist_meal_variations(meal_type_id);
CREATE INDEX idx_nutr_types_plan      ON nutritionist_meal_types(plan_id);

-- +goose StatementEnd
