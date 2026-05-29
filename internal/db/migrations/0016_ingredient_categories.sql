-- +goose Up
CREATE TABLE ingredient_categories (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, name)
);

CREATE TABLE allergen_tags (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO allergen_tags (name) VALUES
    ('glutine'),
    ('lattosio'),
    ('uova'),
    ('frutta a guscio'),
    ('arachidi'),
    ('soia'),
    ('pesce'),
    ('crostacei'),
    ('sedano'),
    ('senape'),
    ('sesamo'),
    ('molluschi'),
    ('lupino'),
    ('anidride solforosa');

CREATE TABLE ingredient_allergens (
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    allergen_id   INTEGER NOT NULL REFERENCES allergen_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (ingredient_id, allergen_id)
);

ALTER TABLE ingredients ADD COLUMN category_id INTEGER REFERENCES ingredient_categories(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE ingredients DROP COLUMN category_id;
DROP TABLE ingredient_allergens;
DROP TABLE allergen_tags;
DROP TABLE ingredient_categories;
