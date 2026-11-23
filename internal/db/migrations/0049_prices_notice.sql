-- +goose Up
-- The notice on the shopping list that store prices are indicative (#277): a user can
-- close it for good.
ALTER TABLE users ADD COLUMN prices_notice_hidden INTEGER NOT NULL DEFAULT 0 CHECK (prices_notice_hidden IN (0, 1));

-- An integrated store is connected once for a user, also when two clicks race.
CREATE UNIQUE INDEX idx_store_plugins_builtin ON store_plugins (user_id, base_url) WHERE base_url LIKE 'builtin://%';

-- +goose Down
DROP INDEX idx_store_plugins_builtin;
ALTER TABLE users DROP COLUMN prices_notice_hidden;
