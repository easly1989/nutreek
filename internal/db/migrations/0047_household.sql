-- +goose Up
-- Household (#238): other people who share the account's week. A person has no login
-- and a calorie goal of their own; their quantities are the account owner's scaled by
-- the ratio of the goals, unless set by hand for one item (household_item_quantities,
-- an absolute quantity in the item's own unit, 0 = not eaten) or skipped for a whole
-- meal (household_meal_skips).
CREATE TABLE household_people (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    target_kcal REAL    NOT NULL CHECK (target_kcal > 0),
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_household_people_user ON household_people (user_id);

CREATE TABLE household_item_quantities (
    person_id    INTEGER NOT NULL REFERENCES household_people (id) ON DELETE CASCADE,
    meal_item_id INTEGER NOT NULL REFERENCES meal_items (id) ON DELETE CASCADE,
    quantity     REAL    NOT NULL CHECK (quantity >= 0),
    PRIMARY KEY (person_id, meal_item_id)
);
CREATE INDEX idx_household_item_quantities_item ON household_item_quantities (meal_item_id);

CREATE TABLE household_meal_skips (
    person_id INTEGER NOT NULL REFERENCES household_people (id) ON DELETE CASCADE,
    meal_id   INTEGER NOT NULL REFERENCES meals (id) ON DELETE CASCADE,
    PRIMARY KEY (person_id, meal_id)
);
CREATE INDEX idx_household_meal_skips_meal ON household_meal_skips (meal_id);

-- What each person needs of an ingredient on a shopping list (person_id 0 = the
-- account owner): the detail that opens on a row. It is rewritten with the list.
CREATE TABLE shopping_list_shares (
    shopping_list_id INTEGER NOT NULL REFERENCES shopping_lists (id) ON DELETE CASCADE,
    ingredient_id    INTEGER NOT NULL REFERENCES ingredients (id) ON DELETE CASCADE,
    person_id        INTEGER NOT NULL,
    quantity         REAL    NOT NULL,
    PRIMARY KEY (shopping_list_id, ingredient_id, person_id)
);

-- +goose Down
DROP TABLE shopping_list_shares;
DROP TABLE household_meal_skips;
DROP TABLE household_item_quantities;
DROP TABLE household_people;
