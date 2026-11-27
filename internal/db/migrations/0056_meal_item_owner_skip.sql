-- +goose Up
-- owner_skip: the account's owner does not eat this row (#353). The quantity stays: it is
-- the base the other people's automatic quantities are scaled from.
ALTER TABLE meal_items ADD COLUMN owner_skip INTEGER NOT NULL DEFAULT 0
    CHECK(owner_skip IN (0,1));

-- +goose Down
ALTER TABLE meal_items DROP COLUMN owner_skip;
