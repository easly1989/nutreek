-- +goose Up
-- An «ignored» shopping item (#230) stays in the list, greyed out, and does not
-- count in the progress: the user does not need it this time.
ALTER TABLE shopping_list_items ADD COLUMN ignored INTEGER NOT NULL DEFAULT 0 CHECK (ignored IN (0, 1));
ALTER TABLE shopping_user_items ADD COLUMN ignored INTEGER NOT NULL DEFAULT 0 CHECK (ignored IN (0, 1));

-- +goose Down
ALTER TABLE shopping_user_items DROP COLUMN ignored;
ALTER TABLE shopping_list_items DROP COLUMN ignored;
