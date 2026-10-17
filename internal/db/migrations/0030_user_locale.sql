-- +goose Up
-- The interface language each user picked (#159): a code from i18n.Languages,
-- or '' to follow the browser. Users who signed up before this migration
-- keep the Italian they always had.
ALTER TABLE users ADD COLUMN locale TEXT NOT NULL DEFAULT '';
UPDATE users SET locale = 'it';

-- +goose Down
ALTER TABLE users DROP COLUMN locale;
