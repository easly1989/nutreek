-- +goose Up
-- How the user wants the shopping list grouped (#232): 'none' shows one list
-- sorted by name (the default), 'category' groups the rows by the ingredient's
-- category.
ALTER TABLE users ADD COLUMN shopping_group TEXT NOT NULL DEFAULT 'none' CHECK (shopping_group IN ('none', 'category'));

-- +goose Down
ALTER TABLE users DROP COLUMN shopping_group;
