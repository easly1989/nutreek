-- +goose Up
-- La categoria di un ingrediente e' di ogni utente (#143): la colonna
-- ingredients.category_id stava sull'ingrediente condiviso, e l'assegnazione
-- di un utente sovrascriveva quella degli altri. Ora ogni utente mette
-- qualsiasi ingrediente nelle proprie categorie.
CREATE TABLE ingredient_user_categories (
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ingredient_id INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    category_id   INTEGER NOT NULL REFERENCES ingredient_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, ingredient_id)
);
CREATE INDEX idx_ingredient_user_categories_category ON ingredient_user_categories(category_id);

-- Le assegnazioni esistenti restano a chi possiede la categoria. La colonna
-- vecchia resta orfana (come in 0012).
INSERT OR IGNORE INTO ingredient_user_categories (user_id, ingredient_id, category_id)
SELECT ic.user_id, i.id, ic.id
  FROM ingredients i
  JOIN ingredient_categories ic ON ic.id = i.category_id;

-- +goose Down
DROP TABLE ingredient_user_categories;
