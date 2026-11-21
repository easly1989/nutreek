-- +goose Up
-- An item the user deleted from a shopping list (#230) stays deleted when the
-- list is regenerated: the ingredient is remembered per list, and Generate
-- leaves it out. Adding it by hand is always possible.
CREATE TABLE shopping_list_deleted (
    shopping_list_id INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    ingredient_id    INTEGER NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    PRIMARY KEY (shopping_list_id, ingredient_id)
);

-- +goose Down
DROP TABLE shopping_list_deleted;
