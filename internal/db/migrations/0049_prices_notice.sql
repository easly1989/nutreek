-- +goose Up
-- The notice on the shopping list that store prices are indicative (#277): a user can
-- close it for good.
ALTER TABLE users ADD COLUMN prices_notice_hidden INTEGER NOT NULL DEFAULT 0 CHECK (prices_notice_hidden IN (0, 1));

-- +goose Down
ALTER TABLE users DROP COLUMN prices_notice_hidden;
