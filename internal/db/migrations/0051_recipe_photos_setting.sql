-- +goose Up
-- Whether the recipe list shows the photos of the recipes (#251). On by default: a person who
-- finds them in the way turns them off in Settings.
ALTER TABLE users ADD COLUMN show_recipe_photos INTEGER NOT NULL DEFAULT 1 CHECK (show_recipe_photos IN (0, 1));

-- +goose Down
ALTER TABLE users DROP COLUMN show_recipe_photos;
