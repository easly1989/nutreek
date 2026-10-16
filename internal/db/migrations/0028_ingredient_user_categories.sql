-- +goose Up
-- An ingredient's category is per user (#143): the ingredients.category_id
-- column sat on the shared ingredient, and one user's assignment overwrote
-- everyone else's. Now every user puts any ingredient in their own
-- categories.
CREATE TABLE ingredient_user_categories (
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    category_id   INTEGER NOT NULL REFERENCES ingredient_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, ingredient_id)
);
CREATE INDEX idx_ingredient_user_categories_category ON ingredient_user_categories(category_id);

-- Existing assignments stay with whoever owns the category. The old column
-- stays orphaned (as in 0012).
INSERT OR IGNORE INTO ingredient_user_categories (user_id, ingredient_id, category_id)
SELECT ic.user_id, i.id, ic.id
  FROM ingredients i
  JOIN ingredient_categories ic ON ic.id = i.category_id;

-- +goose Down
DROP TABLE ingredient_user_categories;
