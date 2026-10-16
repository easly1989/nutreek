-- +goose Up
-- +goose StatementBegin
-- is_draft = 1 marks a recipe as an "unconfirmed draft", typically created
-- by Duplicate. Drafts are:
--   - hidden from the recipe list
--   - not selectable in weekly plans
--   - not savable (handleRecipeUpdate blocks the save while is_draft = 1)
-- They are promoted to is_draft = 0 on the first ingredient add/remove that
-- makes the content differ from every other recipe of the user.
ALTER TABLE recipes ADD COLUMN is_draft INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_recipes_is_draft ON recipes(is_draft);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_recipes_is_draft;
ALTER TABLE recipes DROP COLUMN is_draft;
-- +goose StatementEnd
